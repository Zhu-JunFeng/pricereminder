package stream

import (
	"sync"

	"github.com/google/uuid"

	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/pricebuffer"
)

type Client struct {
	DeviceID uuid.UUID
	Symbols  map[string]struct{}
	Messages chan domain.PricePoint
	Overflow chan struct{}
	once     sync.Once
}

type Hub struct {
	mu      sync.Mutex
	buffer  *pricebuffer.Buffer
	clients map[*Client]struct{}
}

func NewHub(buffer *pricebuffer.Buffer) *Hub {
	return &Hub{buffer: buffer, clients: make(map[*Client]struct{})}
}

func (h *Hub) Publish(point domain.PricePoint) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.buffer.Add(point) {
		return
	}
	for client := range h.clients {
		if _, ok := client.Symbols[point.Symbol]; !ok {
			continue
		}
		select {
		case client.Messages <- point:
		default:
			client.once.Do(func() { close(client.Overflow) })
		}
	}
}

func (h *Hub) Subscribe(deviceID uuid.UUID, symbols []string, lastEventTime map[string]int64) (*Client, []domain.PricePoint, []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := make(map[string]struct{}, len(symbols))
	var replay []domain.PricePoint
	var historyGaps []string
	for _, symbol := range symbols {
		set[symbol] = struct{}{}
		after, provided := lastEventTime[symbol]
		if !provided {
			after = -1
		} else if earliest, ok := h.buffer.Earliest(symbol); ok && after < earliest.EventTime-1000 {
			historyGaps = append(historyGaps, symbol)
		}
		replay = append(replay, h.buffer.Range(symbol, after)...)
	}
	client := &Client{DeviceID: deviceID, Symbols: set, Messages: make(chan domain.PricePoint, 256), Overflow: make(chan struct{})}
	h.clients[client] = struct{}{}
	return client, replay, historyGaps
}

func (h *Hub) Unsubscribe(client *Client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.Messages)
	}
}

func (h *Hub) UpdateSubscriptions(deviceID uuid.UUID, symbols []string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	set := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		set[symbol] = struct{}{}
	}
	for client := range h.clients {
		if client.DeviceID == deviceID {
			client.Symbols = set
		}
	}
}

func (h *Hub) Status() (symbols int, latestEventTime int64) { return h.buffer.Status() }
