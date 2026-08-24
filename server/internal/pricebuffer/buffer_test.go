package pricebuffer

import (
	"testing"

	"pricereminder/server/internal/domain"
)

func point(t *testing.T, symbol, price string, timestamp int64) domain.PricePoint {
	t.Helper()
	result, err := domain.NewPricePoint(symbol, price, timestamp)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestBufferCoalescesSameSecondAndRetainsOneHour(t *testing.T) {
	buffer := New()
	buffer.Add(point(t, "BTCUSDT", "100", 0))
	buffer.Add(point(t, "BTCUSDT", "101", 999))
	buffer.Add(point(t, "BTCUSDT", "102", RetentionMillis+1000))

	items := buffer.Range("BTCUSDT", -1)
	if len(items) != 1 || items[0].PriceText != "102" {
		t.Fatalf("unexpected retained points: %#v", items)
	}
}

func TestBufferIgnoresOutOfOrderPoint(t *testing.T) {
	buffer := New()
	buffer.Add(point(t, "BTCUSDT", "101", 2000))
	if buffer.Add(point(t, "BTCUSDT", "100", 1000)) {
		t.Fatal("out-of-order point should be ignored")
	}
}

func TestBufferReportsEarliestPoint(t *testing.T) {
	buffer := New()
	buffer.Add(point(t, "BTCUSDT", "100", 1000))
	buffer.Add(point(t, "BTCUSDT", "101", 2000))
	earliest, ok := buffer.Earliest("BTCUSDT")
	if !ok || earliest.EventTime != 1000 {
		t.Fatalf("unexpected earliest point: %#v %v", earliest, ok)
	}
}
