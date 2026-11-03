package gold

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// Service caches a single upstream subscription. Client requests never trigger
// upstream requests, and this feed is independent of the Binance watchdog.
type Service struct {
	mu          sync.RWMutex
	quote       *Quote
	lastError   string
	lastSuccess int64
}

func (s *Service) Run(ctx context.Context, logger *slog.Logger) {
	// Domestic quotes must not inherit the Binance proxy environment.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 8 * time.Second, Transport: transport}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		quote, err := Fetch(ctx, client)
		if ctx.Err() != nil {
			return
		}
		s.mu.Lock()
		if err != nil {
			s.lastError = "黄金数据源暂不可用"
			logger.Warn("gold quotation fetch failed", "error", err)
		} else if s.quote != nil && quote.EventTime < s.quote.EventTime {
			s.lastError = "黄金报价时间倒退"
			logger.Warn("gold quotation timestamp regressed")
		} else {
			s.quote = &quote
			s.lastError = ""
			s.lastSuccess = quote.ReceivedAt
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.serveAt(w, r, time.Now()) }

func (s *Service) serveAt(w http.ResponseWriter, r *http.Request, at time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if s.quote == nil {
		http.Error(w, `{"error":"黄金报价暂不可用"}`, http.StatusServiceUnavailable)
		return
	}
	now := at.UnixMilli()
	// Source timestamps may identify the last trade rather than a heartbeat.
	// Never claim a holiday or delayed transport based solely on an unchanged price.
	state := "quoted"
	if now-s.quote.EventTime > 120000 || now-s.lastSuccess > 15000 || s.lastError != "" {
		state = "stale"
	}
	if session := sessionState(at); session != "open" {
		state = session
	}
	_ = json.NewEncoder(w).Encode(struct {
		Quote
		Symbol string `json:"symbol"`
		Source string `json:"source"`
		Unit   string `json:"unit"`
		State  string `json:"state"`
		Error  string `json:"error,omitempty"`
	}{*s.quote, Symbol, "新浪财经", "元/克", state, s.lastError})
}
