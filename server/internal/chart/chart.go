// Package chart serves display-only history. It never feeds the alert engines.
package chart

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"pricereminder/server/internal/gold"
)

const minute = int64(60_000)
const candleInterval = 15 * minute
const cacheDuration = time.Minute

var beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

type Point struct {
	Time  int64  `json:"time"`
	Open  string `json:"open,omitempty"`
	High  string `json:"high,omitempty"`
	Low   string `json:"low,omitempty"`
	Close string `json:"close"`
}

type Series struct {
	Symbol     string  `json:"symbol"`
	Date       string  `json:"date"`
	Source     string  `json:"source"`
	UpdatedAt  int64   `json:"updatedAt"`
	IntervalMS int64   `json:"intervalMS"`
	Open       string  `json:"open,omitempty"`
	FinalClose string  `json:"finalClose,omitempty"`
	Stale      bool    `json:"stale"`
	Points     []Point `json:"points"`
}

type entry struct {
	window string
	until  time.Time
	data   Series
	err    error
}

type Service struct {
	mu         sync.Mutex
	cache      map[string]entry
	inflight   map[string]chan struct{}
	slots      chan struct{}
	binance    *http.Client
	gold       *http.Client
	binanceURL string
}

func New(binance *http.Client, binanceURL string) *Service {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Service{
		cache: make(map[string]entry), inflight: make(map[string]chan struct{}), slots: make(chan struct{}, 6),
		binance: binance, binanceURL: strings.TrimRight(binanceURL, "/"),
		gold: &http.Client{Transport: transport, Timeout: 6 * time.Second},
	}
}

func dayStart(at time.Time) time.Time {
	t := at.In(beijing)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, beijing)
}

func windowKey(symbol string, at time.Time) string {
	t := at.In(beijing)
	if symbol == gold.Symbol {
		t = t.Add(-20 * time.Hour)
	}
	return t.Format(time.DateOnly)
}

func (s *Service) Get(ctx context.Context, symbol string) (Series, error) {
	for {
		if err := ctx.Err(); err != nil {
			return Series{}, err
		}
		now := time.Now()
		window := windowKey(symbol, now)
		s.mu.Lock()
		previous := s.cache[symbol]
		if previous.window == window && now.Before(previous.until) {
			s.mu.Unlock()
			return previous.data, previous.err
		}
		pending := s.inflight[symbol]
		if pending == nil {
			pending = make(chan struct{})
			s.inflight[symbol] = pending
			go s.refresh(symbol, window, now, previous, pending)
		}
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return Series{}, ctx.Err()
		case <-pending:
		}
	}
}

func (s *Service) refresh(symbol, window string, now time.Time, previous entry, done chan struct{}) {
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Second)
	defer cancel()
	var data Series
	var err error
	select {
	case <-ctx.Done():
		err = ctx.Err()
	case s.slots <- struct{}{}:
		if symbol == gold.Symbol {
			data, err = s.fetchGold(ctx, now, previous.data)
		} else {
			data, err = s.fetchBinance(ctx, symbol, now)
		}
		<-s.slots
	}
	if err != nil && previous.window == window && len(previous.data.Points) > 0 {
		data, err = previous.data, nil
		data.Stale = true
	}
	s.mu.Lock()
	s.cache[symbol] = entry{window: window, until: time.Now().Add(cacheDuration), data: data, err: err}
	delete(s.inflight, symbol)
	close(done)
	s.mu.Unlock()
}

func (s *Service) fetchBinance(ctx context.Context, symbol string, now time.Time) (Series, error) {
	start := dayStart(now)
	query := url.Values{
		"symbol": {symbol}, "interval": {"15m"}, "limit": {"96"},
		"startTime": {strconv.FormatInt(start.UnixMilli(), 10)},
		"endTime":   {strconv.FormatInt(start.AddDate(0, 0, 1).UnixMilli()-1, 10)},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.binanceURL+"/fapi/v1/klines?"+query.Encode(), nil)
	if err != nil {
		return Series{}, err
	}
	var rows [][]json.RawMessage
	if err := readJSON(s.binance, req, &rows); err != nil {
		return Series{}, err
	}
	return parseBinance(rows, symbol, start, now)
}

func parseBinance(rows [][]json.RawMessage, symbol string, start, now time.Time) (Series, error) {
	end := start.AddDate(0, 0, 1).UnixMilli()
	out := Series{Symbol: symbol, Date: start.Format(time.DateOnly), Source: "币安", IntervalMS: candleInterval}
	for _, row := range rows {
		if len(row) < 7 {
			return Series{}, fmt.Errorf("invalid Binance candle")
		}
		var ms, endMS int64
		if json.Unmarshal(row[0], &ms) != nil || json.Unmarshal(row[6], &endMS) != nil ||
			ms < start.UnixMilli() || ms >= end || ms > now.UnixMilli() ||
			(ms-start.UnixMilli())%candleInterval != 0 || endMS != ms+candleInterval-1 ||
			(len(out.Points) > 0 && ms <= out.Points[len(out.Points)-1].Time) {
			return Series{}, fmt.Errorf("invalid Binance candle time")
		}
		open, high, low, closePrice := price(row[1]), price(row[2]), price(row[3]), price(row[4])
		if open == "" || high == "" || low == "" || closePrice == "" {
			return Series{}, fmt.Errorf("invalid Binance candle price")
		}
		o := decimal.RequireFromString(open)
		h := decimal.RequireFromString(high)
		l := decimal.RequireFromString(low)
		c := decimal.RequireFromString(closePrice)
		if h.LessThan(o) || h.LessThan(c) || l.GreaterThan(o) || l.GreaterThan(c) {
			return Series{}, fmt.Errorf("invalid Binance candle range")
		}
		if ms == start.UnixMilli() {
			out.Open = open
		}
		out.Points = append(out.Points, Point{Time: ms, Open: open, High: high, Low: low, Close: closePrice})
		out.UpdatedAt = min(endMS, now.UnixMilli())
		if endMS == end-1 && !now.Before(time.UnixMilli(end)) {
			out.FinalClose = closePrice
		}
	}
	if len(out.Points) == 0 {
		return Series{}, fmt.Errorf("no Binance candles for day")
	}
	return out, nil
}

func readJSON(client *http.Client, request *http.Request, dest any) error {
	resp, err := client.Do(request)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("chart source HTTP %d", resp.StatusCode)
	}
	const maxBody = 512 * 1024
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return err
	}
	if len(body) > maxBody {
		return fmt.Errorf("chart response too large")
	}
	return json.Unmarshal(body, dest)
}

func price(raw json.RawMessage) string {
	text := strings.Trim(string(raw), "\"")
	if len(text) > 32 {
		return ""
	}
	value, err := decimal.NewFromString(text)
	if err != nil || !value.IsPositive() || value.Exponent() < -16 || value.Exponent() > 16 {
		return ""
	}
	return value.String()
}
