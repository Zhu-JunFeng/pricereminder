package binance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"pricereminder/server/internal/domain"
)

func TestMarketStreamPublishesOnlyTradingUSDTContracts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		connection, err := websocket.Accept(response, request, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer connection.CloseNow()
		_ = connection.Write(request.Context(), websocket.MessageText, []byte(`[
			{"e":"24hrMiniTicker","E":1700000000000,"s":"BTCUSDT","c":"65000.10"},
			{"e":"24hrMiniTicker","E":1700000000000,"s":"BTCUSDC","c":"65001.10"}
		]`))
	}))
	defer server.Close()
	catalog := NewCatalog()
	catalog.Replace([]Contract{{Symbol: "BTCUSDT", QuoteAsset: "USDT"}, {Symbol: "BTCUSDC", QuoteAsset: "USDC"}})
	publisher := recordingPublisher{points: make(chan domain.PricePoint, 2)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- consumeMarketStream(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), catalog, publisher)
	}()
	select {
	case point := <-publisher.points:
		if point.Symbol != "BTCUSDT" {
			t.Fatalf("unexpected market point: %#v", point)
		}
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("market price was not published")
	}
	<-done
}
