package gold

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSnapshotStates(t *testing.T) {
	at := time.Date(2026, 11, 3, 11, 0, 0, 0, beijing)
	now := at.UnixMilli()
	for _, tc := range []struct {
		name            string
		age, successAge int64
		problem         string
		status          int
		state           string
	}{
		{"fresh", 25000, 1000, "", 200, "quoted"},
		{"stale_quote", 130000, 1000, "", 200, "stale"},
		{"stopped_source", 1000, 16000, "", 200, "stale"},
		{"source_error", 1000, 1000, "unavailable", 200, "stale"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := Service{quote: &Quote{Price: "847", PreviousClose: "836.08", EventTime: now - tc.age, ReceivedAt: now - tc.successAge}, lastSuccess: now - tc.successAge, lastError: tc.problem}
			w := httptest.NewRecorder()
			s.serveAt(w, httptest.NewRequest("GET", "/v1/gold/quote", nil), at)
			var result map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if w.Code != tc.status || result["state"] != tc.state || result["symbol"] != Symbol || result["eventTime"] != float64(now-tc.age) {
				t.Fatal(w.Code, result)
			}
		})
	}
	s := Service{}
	w := httptest.NewRecorder()
	s.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 503 {
		t.Fatal(w.Code)
	}
}
