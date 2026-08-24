package stream

import (
	"testing"

	"github.com/google/uuid"

	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/pricebuffer"
)

func TestSubscribeReportsHistoryGapAndReplaysRetainedPoints(t *testing.T) {
	buffer := pricebuffer.New()
	hub := NewHub(buffer)
	first, _ := domain.NewPricePoint("BTCUSDT", "100", pricebuffer.RetentionMillis+2_000)
	second, _ := domain.NewPricePoint("BTCUSDT", "101", pricebuffer.RetentionMillis+3_000)
	hub.Publish(first)
	hub.Publish(second)

	client, replay, gaps := hub.Subscribe(uuid.New(), []string{"BTCUSDT"}, map[string]int64{"BTCUSDT": 1_000})
	defer hub.Unsubscribe(client)
	if len(gaps) != 1 || gaps[0] != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT history gap, got %#v", gaps)
	}
	if len(replay) != 2 || !replay[0].Replay || !replay[1].Replay {
		t.Fatalf("expected retained replay points, got %#v", replay)
	}
}

func TestUpdateSubscriptionsChangesLiveRouting(t *testing.T) {
	hub := NewHub(pricebuffer.New())
	deviceID := uuid.New()
	client, _, _ := hub.Subscribe(deviceID, []string{"BTCUSDT"}, nil)
	defer hub.Unsubscribe(client)

	hub.UpdateSubscriptions(deviceID, []string{"ETHUSDT"})
	btc, _ := domain.NewPricePoint("BTCUSDT", "60000", 1_000)
	eth, _ := domain.NewPricePoint("ETHUSDT", "3000", 1_000)
	hub.Publish(btc)
	hub.Publish(eth)

	select {
	case received := <-client.Messages:
		if received.Symbol != "ETHUSDT" {
			t.Fatalf("received %s after subscription update", received.Symbol)
		}
	default:
		t.Fatal("updated subscription did not receive ETHUSDT")
	}
}
