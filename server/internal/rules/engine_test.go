package rules

import (
	"testing"

	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/pricebuffer"
)

func mustPoint(t *testing.T, symbol, price string, eventTime int64) domain.PricePoint {
	t.Helper()
	point, err := domain.NewPricePoint(symbol, price, eventTime)
	if err != nil {
		t.Fatal(err)
	}
	return point
}

func TestEqualThresholdTriggersAndRearmsImmediately(t *testing.T) {
	buffer := pricebuffer.New()
	rule, _ := NewRule("r1", "BTCUSDT", 1, "5")
	buffer.Add(mustPoint(t, "BTCUSDT", "100", 0))

	current := mustPoint(t, "BTCUSDT", "105", 60000)
	buffer.Add(current)
	got, _ := Evaluate(&rule, current, buffer)
	if len(got) != 1 || got[0].Direction != Rise {
		t.Fatalf("expected rise trigger, got %#v", got)
	}

	current = mustPoint(t, "BTCUSDT", "104", 61000)
	buffer.Add(current)
	got, _ = Evaluate(&rule, current, buffer)
	if len(got) != 0 || rule.RiseTriggered {
		t.Fatalf("expected rearmed rule, got %#v", got)
	}

	current = mustPoint(t, "BTCUSDT", "106", 62000)
	buffer.Add(current)
	got, _ = Evaluate(&rule, current, buffer)
	if len(got) != 1 {
		t.Fatalf("expected second trigger, got %#v", got)
	}
}

func TestDirectionsAreIndependent(t *testing.T) {
	buffer := pricebuffer.New()
	rule, _ := NewRule("r1", "ETHUSDT", 1, "5")
	buffer.Add(mustPoint(t, "ETHUSDT", "100", 0))
	up := mustPoint(t, "ETHUSDT", "106", 60000)
	buffer.Add(up)
	got, _ := Evaluate(&rule, up, buffer)
	if len(got) != 1 || got[0].Direction != Rise {
		t.Fatalf("expected rise, got %#v", got)
	}

	down := mustPoint(t, "ETHUSDT", "94", 61000)
	buffer.Add(down)
	got, _ = Evaluate(&rule, down, buffer)
	if len(got) != 1 || got[0].Direction != Fall {
		t.Fatalf("expected fall, got %#v", got)
	}
}

func TestRequiresCompleteWindow(t *testing.T) {
	buffer := pricebuffer.New()
	rule, _ := NewRule("r1", "SOLUSDT", 5, "2")
	buffer.Add(mustPoint(t, "SOLUSDT", "100", 0))
	current := mustPoint(t, "SOLUSDT", "110", 299999)
	buffer.Add(current)
	got, _ := Evaluate(&rule, current, buffer)
	if len(got) != 0 {
		t.Fatalf("expected no trigger, got %#v", got)
	}
}
