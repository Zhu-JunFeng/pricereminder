package watchdog

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestControllerCandidatesAndSelection(t *testing.T) {
	selected := "old"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/providers/proxies/pqjc":
			io.WriteString(w, `{"proxies":[{"name":"DIRECT","type":"Direct"},{"name":"group","type":"Selector"},{"name":"fast","type":"Vless"},{"name":"slow","type":"Vless"},{"name":"dead","type":"Vless"}]}`)
		case strings.HasSuffix(r.URL.Path, "/delay"):
			if r.URL.Query().Get("expected") != "200" {
				t.Error("candidate probes must reject non-200 responses")
			}
			if strings.Contains(r.URL.Path, "dead") {
				w.WriteHeader(503)
			} else if strings.Contains(r.URL.Path, "fast") {
				io.WriteString(w, `{"delay":10}`)
			} else {
				io.WriteString(w, `{"delay":30}`)
			}
		case r.Method == "PUT":
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			selected = body["name"]
			w.WriteHeader(204)
		default:
			json.NewEncoder(w).Encode(map[string]string{"type": "Selector", "now": selected})
		}
	}))
	defer s.Close()
	b, _ := NewLive(Config{Controller: s.URL, Proxy: "http://127.0.0.1:7890", Group: "节点选择", Provider: "pqjc", RESTURL: "http://example.test/time"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	nodes, err := b.Candidates(context.Background())
	if err != nil || len(nodes) != 2 || nodes[0].Name != "fast" {
		t.Fatal(err, nodes)
	}
	if err = b.Select(context.Background(), "fast"); err != nil || selected != "fast" {
		t.Fatal(err, selected)
	}
}

func TestRESTAndWebSocketActualPayload(t *testing.T) {
	bad := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/time" {
			if bad {
				io.WriteString(w, `{"serverTime":1}`)
			} else {
				json.NewEncoder(w).Encode(map[string]int64{"serverTime": time.Now().UnixMilli()})
			}
			return
		}
		c, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer c.CloseNow()
		for i := 0; i < 3; i++ {
			data, _ := json.Marshal(map[string]any{"e": "trade", "s": "BTCUSDT", "E": time.Now().UnixMilli() + int64(i), "p": "100"})
			if err = c.Write(r.Context(), websocket.MessageText, data); err != nil {
				return
			}
		}
		c.Read(r.Context())
	}))
	defer s.Close()
	b := &Live{Config: Config{RESTURL: s.URL + "/time", WSURL: "ws" + strings.TrimPrefix(s.URL, "http") + "/ws"}, Proxied: s.Client(), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := b.REST(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := b.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	bad = true
	if b.REST(context.Background()) == nil {
		t.Fatal("accepted stale REST payload")
	}
}
