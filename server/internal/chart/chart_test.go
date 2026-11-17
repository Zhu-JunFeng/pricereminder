package chart

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGoldCutoffAndOvernightTradingDay(t *testing.T) {
	timeline := goldTimeline{
		Instrument: "Au99.99", SourceTime: "2026年11月14日 00:01:20",
		Times: []string{"20:00", "23:59", "00:01", "09:00", "11:30", "11:31", "12:00", "13:30"},
		Prices: []json.RawMessage{json.RawMessage(`"830"`), json.RawMessage(`"835"`), json.RawMessage(`"837"`),
			json.RawMessage(`"837"`), json.RawMessage(`"837"`), json.RawMessage(`"837"`),
			json.RawMessage(`"837"`), json.RawMessage(`"837"`)},
	}
	now := time.Date(2026, 11, 14, 0, 5, 0, 0, beijing)
	out, err := parseGold(timeline, now)
	if err != nil {
		t.Fatal(err)
	}
	if out.Date != "2026-11-16" || len(out.Points) != 3 ||
		out.Points[0].Time != time.Date(2026, 11, 13, 20, 0, 0, 0, beijing).UnixMilli() ||
		out.Points[2].Time != time.Date(2026, 11, 14, 0, 1, 0, 0, beijing).UnixMilli() {
		t.Fatalf("future slots or wrong overnight dates: %+v", out)
	}
	timeline.SourceTime = "2026年11月16日 12:00:00"
	out, err = parseGold(timeline, time.Date(2026, 11, 16, 12, 0, 0, 0, beijing))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Points) != 5 || out.Points[4].Time != time.Date(2026, 11, 16, 11, 30, 0, 0, beijing).UnixMilli() {
		t.Fatalf("lunch break should retain morning only: %+v", out)
	}
}

func TestGoldCloseMatchesTradingDate(t *testing.T) {
	var rows [][]json.RawMessage
	if err := json.Unmarshal([]byte(`[["2026-11-16",841,829.49,823,841],["2026-11-17",829,831.42,825,834]]`), &rows); err != nil {
		t.Fatal(err)
	}
	if got := goldClose(rows, "2026-11-17"); got != "831.42" {
		t.Fatalf("wrong official close: %q", got)
	}
	if got := goldClose(rows, "2026-11-18"); got != "" {
		t.Fatalf("unexpected close on unpublished date: %q", got)
	}
}

func TestBinanceBeijingBoundaryAndConfirmedClose(t *testing.T) {
	start := dayStart(time.Date(2026, 11, 16, 16, 29, 0, 0, time.UTC))
	first := start.UnixMilli()
	last := first + 95*candleInterval
	raw := fmt.Sprintf(`[[%d,"100","110","99","105","1",%d],[%d,"120","123","119","121","1",%d]]`,
		first, first+candleInterval-1, last, last+candleInterval-1)
	var rows [][]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		t.Fatal(err)
	}
	before := start.Add(23*time.Hour + 58*time.Minute)
	openDay, err := parseBinance(rows, "BTCUSDT", start, before)
	if err != nil || openDay.Date != "2026-11-17" || openDay.Open != "100" || openDay.FinalClose != "" {
		t.Fatalf("open day: %+v, %v", openDay, err)
	}
	closedDay, err := parseBinance(rows, "BTCUSDT", start, before.Add(2*time.Minute))
	if err != nil || closedDay.FinalClose != "121" {
		t.Fatalf("closed day: %+v, %v", closedDay, err)
	}
	if _, err := parseBinance(rows, "BTCUSDT", start.AddDate(0, 0, 1), before.Add(2*time.Minute)); err == nil {
		t.Fatal("accepted old candles after midnight")
	}
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCacheCoalescesAndKeepsSameDayDataOnFailure(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	started, release := make(chan struct{}), make(chan struct{})
	client := &http.Client{Transport: transportFunc(func(r *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		if fail.Load() {
			return nil, errors.New("upstream offline")
		}
		q := r.URL.Query()
		if q.Get("interval") != "15m" || q.Get("symbol") != "BTCUSDT" {
			return nil, errors.New("unexpected query")
		}
		start := dayStart(time.Now()).UnixMilli()
		data := fmt.Sprintf(`[[%d,"100","110","99","105","1",%d]]`, start, start+candleInterval-1)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(data))}, nil
	})}
	s := New(client, "https://fixture.invalid")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := s.Get(ctx, "BTCUSDT")
			if err != nil || out.Open != "100" || out.Stale {
				t.Errorf("unexpected cached series: %+v, %v", out, err)
			}
		}()
	}
	select {
	case <-started:
		close(release)
	case <-ctx.Done():
		t.Fatal("fetch never started")
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("got %d upstream fetches", calls.Load())
	}
	fail.Store(true)
	s.mu.Lock()
	cached := s.cache["BTCUSDT"]
	cached.until = time.Now().Add(-time.Second)
	s.cache["BTCUSDT"] = cached
	s.mu.Unlock()
	out, err := s.Get(ctx, "BTCUSDT")
	if err != nil || !out.Stale || out.UpdatedAt != cached.data.UpdatedAt {
		t.Fatalf("stale fallback did not preserve source time: %+v, %v", out, err)
	}
}
