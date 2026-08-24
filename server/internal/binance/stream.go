package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"

	"pricereminder/server/internal/domain"
)

type PricePublisher interface{ Publish(domain.PricePoint) }

type SymbolSource interface {
	ActiveSymbols(context.Context) ([]string, error)
}

type tradeEvent struct {
	EventType string          `json:"e"`
	EventTime int64           `json:"E"`
	Symbol    string          `json:"s"`
	Price     string          `json:"p"`
	Quantity  string          `json:"q"`
	TradeType string          `json:"X"`
	Result    json.RawMessage `json:"result"`
	ID        int             `json:"id"`
	Code      int             `json:"code"`
	Message   string          `json:"msg"`
}

type subscriptionRequest struct {
	Method string   `json:"method"`
	Params []string `json:"params"`
	ID     int      `json:"id"`
}

func RunPriceStream(
	ctx context.Context,
	wsURL string,
	symbols SymbolSource,
	catalog *Catalog,
	publisher PricePublisher,
	logger *slog.Logger,
) {
	for ctx.Err() == nil {
		if err := consumePriceStream(ctx, wsURL, symbols, catalog, publisher); err != nil && ctx.Err() == nil {
			logger.Error("binance price stream disconnected", "error", err)
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func consumePriceStream(
	ctx context.Context,
	wsURL string,
	symbols SymbolSource,
	catalog *Catalog,
	publisher PricePublisher,
) error {
	url := strings.TrimRight(wsURL, "/") + "/ws"
	connection, response, err := websocket.Dial(ctx, url, &websocket.DialOptions{
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	})
	if err != nil {
		if response != nil {
			return fmt.Errorf("dial binance websocket: %s: %w", response.Status, err)
		}
		return fmt.Errorf("dial binance websocket: %w", err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "shutdown")
	connection.SetReadLimit(64 << 10)

	streamContext, cancel := context.WithCancel(ctx)
	defer cancel()
	subscriptionErrors := make(chan error, 1)
	go maintainSubscriptions(streamContext, connection, symbols, subscriptionErrors)

	for {
		_, data, err := connection.Read(streamContext)
		if err != nil {
			select {
			case subscriptionErr := <-subscriptionErrors:
				return subscriptionErr
			default:
				return err
			}
		}
		var event tradeEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return fmt.Errorf("decode binance trade: %w", err)
		}
		if event.Code != 0 {
			return fmt.Errorf("binance subscription rejected: code=%d message=%q", event.Code, event.Message)
		}
		if event.EventType != "trade" {
			continue
		}
		if event.TradeType == "NA" || event.Price == "0" || event.Quantity == "0" || !catalog.Contains(event.Symbol) {
			continue
		}
		point, err := domain.NewPricePoint(event.Symbol, event.Price, event.EventTime)
		if err != nil || !point.Price.IsPositive() {
			continue
		}
		publisher.Publish(point)
	}
}

func maintainSubscriptions(ctx context.Context, connection *websocket.Conn, source SymbolSource, errors chan<- error) {
	current := make(map[string]struct{})
	nextID := 1
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := syncSubscriptions(ctx, connection, source, current, &nextID); err != nil {
			select {
			case errors <- err:
			default:
			}
			connection.CloseNow()
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func syncSubscriptions(
	ctx context.Context,
	connection *websocket.Conn,
	source SymbolSource,
	current map[string]struct{},
	nextID *int,
) error {
	symbols, err := source.ActiveSymbols(ctx)
	if err != nil {
		return fmt.Errorf("load active symbols: %w", err)
	}
	desired := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		desired[strings.ToUpper(symbol)] = struct{}{}
	}
	added, removed := subscriptionChanges(current, desired)
	if err := writeSubscription(ctx, connection, "SUBSCRIBE", added, nextID); err != nil {
		return err
	}
	if err := writeSubscription(ctx, connection, "UNSUBSCRIBE", removed, nextID); err != nil {
		return err
	}
	clear(current)
	for symbol := range desired {
		current[symbol] = struct{}{}
	}
	return nil
}

func subscriptionChanges(current, desired map[string]struct{}) (added, removed []string) {
	for symbol := range desired {
		if _, exists := current[symbol]; !exists {
			added = append(added, strings.ToLower(symbol)+"@trade")
		}
	}
	for symbol := range current {
		if _, exists := desired[symbol]; !exists {
			removed = append(removed, strings.ToLower(symbol)+"@trade")
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	return added, removed
}

func writeSubscription(ctx context.Context, connection *websocket.Conn, method string, params []string, nextID *int) error {
	if len(params) == 0 {
		return nil
	}
	request, err := json.Marshal(subscriptionRequest{Method: method, Params: params, ID: *nextID})
	if err != nil {
		return err
	}
	*nextID++
	writeContext, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := connection.Write(writeContext, websocket.MessageText, request); err != nil {
		return fmt.Errorf("%s binance price streams: %w", strings.ToLower(method), err)
	}
	return nil
}
