package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"testing"

	"pricereminder/server/internal/binance"
	"pricereminder/server/internal/pricebuffer"
	"pricereminder/server/internal/stream"
)

func TestHealthSubscriptionStatus(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s := &Server{catalog: binance.NewCatalog(), hub: stream.NewHub(pricebuffer.New()), logger: slog.New(slog.NewTextHandler(io.Discard, nil)), activeSymbols: func(context.Context) ([]string, error) {
			if fail {
				return nil, errors.New("db unavailable")
			}
			return []string{"BTCUSDT", "ETHUSDT"}, nil
		}}
		w := httptest.NewRecorder()
		s.health(w, httptest.NewRequest("GET", "/healthz", nil))
		if fail {
			if w.Code != 503 {
				t.Fatal(w.Code)
			}
			continue
		}
		var v map[string]any
		json.Unmarshal(w.Body.Bytes(), &v)
		if w.Code != 200 || v["activeSubscriptions"] != float64(2) || v["status"] != "ok" {
			t.Fatal(w.Code, v)
		}
		for _, key := range []string{"contracts", "priceSymbols", "latestEventTime"} {
			if _, ok := v[key]; !ok {
				t.Fatal("missing legacy field", key)
			}
		}
	}
}
