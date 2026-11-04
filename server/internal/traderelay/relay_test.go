package traderelay

import (
	"context"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"net/http"
	"net/http/httptest"
	"pricereminder/server/internal/binance"
	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/rules"
	"reflect"
	"strings"
	"testing"
	"time"
)

type source []string

func (s source) ActiveSymbols(context.Context) ([]string, error) { return s, nil }
func fixture(t *testing.T) (*Relay, *session, Config) {
	t.Helper()
	c := binance.NewCatalog()
	c.Replace([]binance.Contract{{Symbol: "BTCUSDT", QuoteAsset: "USDT"}, {Symbol: "ETHUSDC", QuoteAsset: "USDC"}})
	h := New(source{"BTCUSDT"}, c)
	s := &session{id: "session", device: "device", scanners: map[string]*rules.MarketScanner{}, messages: make(chan Envelope, 64), cancel: func() {}}
	h.sessions[s.device] = s
	r, e := rules.NewMarketRule("00000000-0000-0000-0000-000000000001", 1, "1")
	if e != nil {
		t.Fatal(e)
	}
	cfg := Config{Type: "configure", Protocol: 2, Version: 1, Symbols: []string{"BTCUSDT"}, Rules: []rules.MarketRule{r}}
	if e = h.configure(s, cfg); e != nil {
		t.Fatal(e)
	}
	<-s.messages
	return h, s, cfg
}
func point(t *testing.T, s, p string, ts int64) domain.PricePoint {
	t.Helper()
	v, e := domain.NewPricePoint(s, p, ts)
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func TestSharedUpstreamAndIndependentWatermarks(t *testing.T) {
	h, s, _ := fixture(t)
	symbols, e := h.ActiveSymbols(context.Background())
	if e != nil || !reflect.DeepEqual(symbols, []string{"BTCUSDT", "ETHUSDC"}) {
		t.Fatalf("%v %v", symbols, e)
	}
	if h.LegacyInterested("ETHUSDC") {
		t.Fatal("full market leaked into legacy replay")
	}
	h.Publish(point(t, "BTCUSDT", "100", 2000))
	h.Publish(point(t, "ETHUSDC", "100", 1000))
	h.flush(time.Unix(1, 0))
	<-s.messages
	h.Publish(point(t, "ETHUSDC", "102", 1500))
	h.flush(time.Unix(2, 0))
	out := <-s.messages
	if len(out.Triggers) != 1 || out.Triggers[0].Symbol != "ETHUSDC" {
		t.Fatalf("independent symbol lost: %+v", out)
	}
	h.Publish(point(t, "ETHUSDC", "120", 1500))
	h.Publish(point(t, "ETHUSDC", "130", 1400))
	h.flush(time.Unix(3, 0))
	select {
	case out := <-s.messages:
		t.Fatalf("duplicate/late emitted: %+v", out)
	default:
	}
}
func TestConfigIdempotencyAndRuleReset(t *testing.T) {
	h, s, cfg := fixture(t)
	h.Publish(point(t, "BTCUSDT", "100", 1000))
	h.flush(time.Unix(1, 0))
	<-s.messages
	old := s.scanners[cfg.Rules[0].ID]
	if e := h.configure(s, cfg); e != nil {
		t.Fatal(e)
	}
	<-s.messages
	if s.scanners[cfg.Rules[0].ID] != old {
		t.Fatal("identical configuration reset window")
	}
	cfg.Version++
	cfg.Symbols = append(cfg.Symbols, "ETHUSDC")
	if e := h.configure(s, cfg); e != nil {
		t.Fatal(e)
	}
	<-s.messages
	if s.scanners[cfg.Rules[0].ID] != old {
		t.Fatal("normal subscription reset scanner")
	}
	h.Publish(point(t, "BTCUSDT", "102", 2000))
	h.flush(time.Unix(2, 0))
	if out := <-s.messages; len(out.Triggers) != 1 {
		t.Fatalf("lost preserved baseline: %+v", out)
	}
	cfg.Version++
	cfg.Rules[0].ThresholdText = "2"
	if e := h.configure(s, cfg); e != nil {
		t.Fatal(e)
	}
	<-s.messages
	if s.scanners[cfg.Rules[0].ID] == old {
		t.Fatal("modified rule retained window")
	}
	cfg.Symbols = nil
	if e := h.configure(s, cfg); e == nil {
		t.Fatal("conflicting same version accepted")
	}
	cfg.Version = 1
	if e := h.configure(s, cfg); e == nil {
		t.Fatal("obsolete version accepted")
	}
}
func TestLastTradePerFlushAndRemoval(t *testing.T) {
	h, s, cfg := fixture(t)
	h.Publish(point(t, "BTCUSDT", "100", 1000))
	h.Publish(point(t, "BTCUSDT", "101", 1100))
	h.Publish(point(t, "BTCUSDT", "99", 1050))
	h.flush(time.Unix(1, 0))
	out := <-s.messages
	if len(out.Points) != 1 || out.Points[0].PriceText != "101" {
		t.Fatalf("bad aggregate: %+v", out)
	}
	cfg.Version++
	cfg.Rules = nil
	cfg.Symbols = nil
	if e := h.configure(s, cfg); e != nil {
		t.Fatal(e)
	}
	<-s.messages
	symbols, _ := h.ActiveSymbols(context.Background())
	if !reflect.DeepEqual(symbols, []string{"BTCUSDT"}) {
		t.Fatal(symbols)
	}
	h.Publish(point(t, "BTCUSDT", "120", 2000))
	h.flush(time.Unix(2, 0))
	select {
	case out := <-s.messages:
		t.Fatal(out)
	default:
	}
}
func TestResetCancelsSessionsAndDropsPending(t *testing.T) {
	h, s, _ := fixture(t)
	cancelled := false
	s.cancel = func() { cancelled = true }
	h.Publish(point(t, "BTCUSDT", "100", 1000))
	h.Reset()
	if !cancelled || len(h.pending) != 0 || h.latest != 0 || len(h.emitted) != 0 {
		t.Fatal("reset did not fence old stream")
	}
}

func TestWebSocketConfigAndReplacement(t *testing.T) {
	h, _, cfg := fixture(t)
	h.sessions = map[string]*session{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.Serve(w, r, "same-device", "") }))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	first, _, e := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer first.CloseNow()
	if e = wsjson.Write(ctx, first, cfg); e != nil {
		t.Fatal(e)
	}
	var ack Envelope
	if e = wsjson.Read(ctx, first, &ack); e != nil || ack.Type != "configured" {
		t.Fatalf("%+v %v", ack, e)
	}
	second, _, e := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer second.CloseNow()
	if e = wsjson.Write(ctx, second, cfg); e != nil {
		t.Fatal(e)
	}
	if e = wsjson.Read(ctx, second, &ack); e != nil {
		t.Fatal(e)
	}
	if e = wsjson.Read(ctx, first, &ack); e == nil {
		t.Fatal("replaced session remained readable")
	}
	h.Publish(point(t, "BTCUSDT", "100", 1000))
	h.flush(time.Unix(1, 0))
	if e = wsjson.Read(ctx, second, &ack); e != nil || len(ack.Points) != 1 {
		t.Fatalf("new session lost subscription: %+v %v", ack, e)
	}
	h.Reset()
	if e = wsjson.Read(ctx, second, &ack); e == nil {
		t.Fatal("upstream failure left session alive")
	}
}
