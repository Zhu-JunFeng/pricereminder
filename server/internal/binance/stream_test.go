package binance

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"pricereminder/server/internal/domain"
)

type recordingPublisher struct {
	points chan domain.PricePoint
}

func (p recordingPublisher) Publish(point domain.PricePoint) {
	p.points <- point
}

type staticSymbols []string

func (s staticSymbols) ActiveSymbols(context.Context) ([]string, error) {
	return []string(s), nil
}

func TestConsumePriceStreamSubscribesAndPublishes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/ws" {
			http.NotFound(response, request)
			return
		}
		connection, err := websocket.Accept(response, request, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer connection.CloseNow()
		_, data, err := connection.Read(request.Context())
		if err != nil {
			t.Error(err)
			return
		}
		var subscription subscriptionRequest
		if err := json.Unmarshal(data, &subscription); err != nil {
			t.Error(err)
			return
		}
		if subscription.Method != "SUBSCRIBE" || len(subscription.Params) != 1 || subscription.Params[0] != "btcusdt@trade" {
			t.Errorf("unexpected subscription: %#v", subscription)
			return
		}
		if err := connection.Write(request.Context(), websocket.MessageText, []byte(`{"result":null,"id":1}`)); err != nil {
			t.Error(err)
			return
		}
		if err := connection.Write(request.Context(), websocket.MessageText, []byte(`{"e":"trade","E":1699999999000,"s":"BTCUSDT","p":"0","q":"0","X":"NA"}`)); err != nil {
			t.Error(err)
			return
		}
		_ = connection.Write(request.Context(), websocket.MessageText, []byte(`{"e":"trade","E":1700000000000,"s":"BTCUSDT","p":"65000.10","q":"0.1","X":"MARKET"}`))
	}))
	defer server.Close()

	catalog := NewCatalog()
	catalog.Replace([]Contract{{Symbol: "BTCUSDT"}})
	publisher := recordingPublisher{points: make(chan domain.PricePoint, 1)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- consumePriceStream(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), staticSymbols{"BTCUSDT"}, catalog, publisher)
	}()

	select {
	case point := <-publisher.points:
		if point.Symbol != "BTCUSDT" || point.Price.String() != "65000.1" || point.EventTime != 1700000000000 {
			t.Fatalf("unexpected price point: %#v", point)
		}
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("price was not published")
	}
	<-done
}
