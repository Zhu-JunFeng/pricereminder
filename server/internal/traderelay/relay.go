// Package traderelay shares the existing trade upstream with versioned online scanners.
package traderelay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"pricereminder/server/internal/binance"
	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/rules"
)

type Config struct {
	Type     string             `json:"type"`
	Version  int64              `json:"version"`
	Protocol int                `json:"protocol"`
	Symbols  []string           `json:"symbols"`
	Rules    []rules.MarketRule `json:"marketRules"`
}
type Envelope struct {
	Type            string              `json:"type"`
	Protocol        int                 `json:"protocol"`
	Version         int64               `json:"version"`
	Session         string              `json:"session"`
	State           string              `json:"state,omitempty"`
	Points          []domain.PricePoint `json:"prices,omitempty"`
	Triggers        []rules.Trigger     `json:"triggers,omitempty"`
	EventID         string              `json:"eventId,omitempty"`
	ServerTime      int64               `json:"serverTime"`
	LatestEventTime int64               `json:"latestEventTime"`
}
type session struct {
	id, device string
	config     Config
	scanners   map[string]*rules.MarketScanner
	messages   chan Envelope
	cancel     context.CancelFunc
}
type Relay struct {
	mu       sync.Mutex
	source   binance.SymbolSource
	catalog  *binance.Catalog
	sessions map[string]*session
	pending  map[string]domain.PricePoint
	latest   int64
	emitted  map[string]int64
	legacy   map[string]bool
}

func New(source binance.SymbolSource, catalog *binance.Catalog) *Relay {
	return &Relay{source: source, catalog: catalog, sessions: map[string]*session{}, pending: map[string]domain.PricePoint{}, emitted: map[string]int64{}, legacy: map[string]bool{}}
}
func (h *Relay) ActiveSymbols(ctx context.Context) ([]string, error) {
	result, err := h.source.ActiveSymbols(ctx)
	if err != nil {
		return nil, err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	set := map[string]bool{}
	h.legacy = map[string]bool{}
	for _, s := range result {
		h.legacy[s] = true
		set[s] = true
	}
	all := false
	for _, s := range h.sessions {
		for _, v := range s.config.Symbols {
			set[v] = true
		}
		all = all || len(s.config.Rules) > 0
	}
	if all {
		for _, c := range h.catalog.All() {
			if c.QuoteAsset == "USDT" || c.QuoteAsset == "USDC" {
				set[c.Symbol] = true
			}
		}
	}
	result = nil
	for s := range set {
		result = append(result, s)
	}
	sort.Strings(result)
	return result, nil
}
func (h *Relay) Publish(p domain.PricePoint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if p.EventTime <= h.emitted[p.Symbol] {
		return
	}
	if old, ok := h.pending[p.Symbol]; !ok || p.EventTime > old.EventTime {
		h.pending[p.Symbol] = p
	}
	if p.EventTime > h.latest {
		h.latest = p.EventTime
	}
}
func (h *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.flush(now)
		}
	}
}
func (h *Relay) flush(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	points := make([]domain.PricePoint, 0, len(h.pending))
	for _, p := range h.pending {
		points = append(points, p)
		h.emitted[p.Symbol] = p.EventTime
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Symbol < points[j].Symbol })
	h.pending = map[string]domain.PricePoint{}
	for _, s := range h.sessions {
		if s.config.Version == 0 {
			continue
		}
		out := Envelope{Type: "batch", Protocol: 2, Version: s.config.Version, Session: s.id, ServerTime: now.UnixMilli(), LatestEventTime: h.latest}
		for _, p := range points {
			for _, symbol := range s.config.Symbols {
				if symbol == p.Symbol {
					out.Points = append(out.Points, p)
					break
				}
			}
			for _, rule := range s.config.Rules {
				out.Triggers = append(out.Triggers, s.scanners[rule.ID].Evaluate(rule, p)...)
			}
		}
		if len(out.Triggers) > 0 {
			out.EventID = uuid.NewString()
		}
		if len(out.Points) > 0 || len(out.Triggers) > 0 || now.Unix()%10 == 0 {
			h.send(s, out)
		}
	}
}
func (h *Relay) send(s *session, out Envelope) {
	select {
	case s.messages <- out:
	default:
		s.cancel()
	}
}

// Reset closes active v2 sessions on an upstream discontinuity. Clients warm up afresh.
func (h *Relay) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pending = map[string]domain.PricePoint{}
	h.latest = 0
	h.emitted = map[string]int64{}
	for _, s := range h.sessions {
		s.cancel()
	}
}
func (h *Relay) normalize(c Config) (Config, error) {
	if c.Protocol != 2 || c.Type != "configure" || c.Version < 1 || len(c.Symbols) > 50 || len(c.Rules) > 50 {
		return c, errors.New("unsupported protocol or invalid configuration")
	}
	set := map[string]bool{}
	for _, v := range c.Symbols {
		if !h.catalog.ContainsSupportedQuote(v) {
			return c, fmt.Errorf("unsupported symbol %s", v)
		}
		set[v] = true
	}
	c.Symbols = nil
	for v := range set {
		c.Symbols = append(c.Symbols, v)
	}
	sort.Strings(c.Symbols)
	ids := map[string]bool{}
	valid := []rules.MarketRule{}
	for _, r := range c.Rules {
		if _, e := uuid.Parse(r.ID); e != nil || ids[r.ID] {
			return c, errors.New("invalid or duplicate rule id")
		}
		ids[r.ID] = true
		if !r.Enabled {
			continue
		}
		n, e := rules.NewMarketRule(r.ID, r.WindowMinutes, r.ThresholdText)
		if e != nil {
			return c, e
		}
		valid = append(valid, n)
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].ID < valid[j].ID })
	c.Rules = valid
	return c, nil
}
func (h *Relay) configure(s *session, c Config) error {
	normalized, err := h.normalize(c)
	if err != nil {
		return err
	}
	c = normalized
	h.mu.Lock()
	defer h.mu.Unlock()
	if c.Version < s.config.Version {
		return errors.New("obsolete config version")
	}
	if c.Version == s.config.Version && !reflect.DeepEqual(c, s.config) {
		return errors.New("conflicting config version")
	}
	next := map[string]*rules.MarketScanner{}
	for _, r := range c.Rules {
		for _, old := range s.config.Rules {
			if reflect.DeepEqual(old, r) {
				next[r.ID] = s.scanners[r.ID]
			}
		}
		if next[r.ID] == nil {
			next[r.ID] = rules.NewMarketScanner()
		}
	}
	s.config = c
	s.scanners = next
	h.send(s, Envelope{Type: "configured", Protocol: 2, Version: c.Version, Session: s.id, State: "warming_up", ServerTime: time.Now().UnixMilli(), LatestEventTime: h.latest})
	return nil
}
func (h *Relay) Serve(w http.ResponseWriter, r *http.Request, device, origin string) {
	options := &websocket.AcceptOptions{}
	if origin != "" {
		options.OriginPatterns = []string{origin}
	}
	conn, err := websocket.Accept(w, r, options)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	s := &session{id: uuid.NewString(), device: device, scanners: map[string]*rules.MarketScanner{}, messages: make(chan Envelope, 64), cancel: cancel}
	h.mu.Lock()
	if old := h.sessions[device]; old != nil {
		old.cancel()
	}
	h.sessions[device] = s
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		if h.sessions[device] == s {
			delete(h.sessions, device)
		}
		h.mu.Unlock()
	}()
	go func() {
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-s.messages:
				writeCtx, done := context.WithTimeout(ctx, 8*time.Second)
				e := wsjson.Write(writeCtx, conn, msg)
				done()
				if e != nil {
					return
				}
			}
		}
	}()
	readCtx, done := context.WithTimeout(ctx, 10*time.Second)
	var config Config
	err = wsjson.Read(readCtx, conn, &config)
	done()
	if err == nil {
		err = h.configure(s, config)
	}
	for err == nil {
		err = wsjson.Read(ctx, conn, &config)
		if err == nil {
			err = h.configure(s, config)
		}
	}
	if ctx.Err() == nil {
		payload, _ := json.Marshal(map[string]string{"error": err.Error()})
		_ = conn.Close(websocket.StatusPolicyViolation, string(payload))
	}
}

// LegacyInterested avoids retaining a full-market replay buffer for v1 clients.
func (h *Relay) LegacyInterested(symbol string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.legacy[symbol]
}
