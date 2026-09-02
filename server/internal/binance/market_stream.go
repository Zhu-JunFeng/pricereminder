package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"

	"pricereminder/server/internal/domain"
)

type marketEvent struct {
	EventType  string `json:"e"`
	EventTime  int64  `json:"E"`
	Symbol     string `json:"s"`
	ClosePrice string `json:"c"`
}

func RunMarketStream(
	ctx context.Context, wsURL string, catalog *Catalog, publisher PricePublisher,
	enabled func() bool, logger *slog.Logger,
) {
	for ctx.Err() == nil {
		if !enabled() {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
				continue
			}
		}
		streamContext, cancel := context.WithCancel(ctx)
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-streamContext.Done():
					return
				case <-ticker.C:
					if !enabled() {
						cancel()
						return
					}
				}
			}
		}()
		err := consumeMarketStream(streamContext, wsURL, catalog, publisher)
		cancel()
		if err != nil && ctx.Err() == nil && enabled() {
			logger.Error("binance all-market stream disconnected", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

func consumeMarketStream(ctx context.Context, wsURL string, catalog *Catalog, publisher PricePublisher) error {
	url := strings.TrimRight(wsURL, "/") + "/ws/!miniTicker@arr"
	connection, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	})
	if err != nil {
		if response != nil {
			return fmt.Errorf("dial Binance all-market websocket: %s: %w", response.Status, err)
		}
		return fmt.Errorf("dial Binance all-market websocket: %w", err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "shutdown")
	connection.SetReadLimit(2 << 20)
	for {
		_, data, err := connection.Read(ctx)
		if err != nil {
			return err
		}
		var events []marketEvent
		if err := json.Unmarshal(data, &events); err != nil {
			return fmt.Errorf("decode Binance all-market prices: %w", err)
		}
		for _, event := range events {
			if event.EventType != "24hrMiniTicker" || event.ClosePrice == "0" || !catalog.ContainsUSDT(event.Symbol) {
				continue
			}
			point, err := domain.NewPricePoint(event.Symbol, event.ClosePrice, event.EventTime)
			if err == nil && point.Price.IsPositive() {
				publisher.Publish(point)
			}
		}
	}
}
