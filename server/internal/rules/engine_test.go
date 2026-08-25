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

func TestTargetPriceTriggersOnceAndRearms(t *testing.T) {
	buffer := pricebuffer.New()
	rule, err := NewTargetRule("target-1", "BTCUSDT", Above, "105")
	if err != nil {
		t.Fatal(err)
	}
	current := mustPoint(t, "BTCUSDT", "104", 1_000)
	buffer.Add(current)
	got, _ := Evaluate(&rule, current, buffer)
	if len(got) != 0 {
		t.Fatalf("target should wait below threshold: %#v", got)
	}
	current = mustPoint(t, "BTCUSDT", "105", 2_000)
	buffer.Add(current)
	got, _ = Evaluate(&rule, current, buffer)
	if len(got) != 1 || got[0].Kind != Target || got[0].TargetPrice != "105" {
		t.Fatalf("equal target should trigger: %#v", got)
	}
	current = mustPoint(t, "BTCUSDT", "106", 3_000)
	buffer.Add(current)
	got, _ = Evaluate(&rule, current, buffer)
	if len(got) != 0 {
		t.Fatalf("target should not repeat while reached: %#v", got)
	}
	current = mustPoint(t, "BTCUSDT", "104", 4_000)
	buffer.Add(current)
	_, _ = Evaluate(&rule, current, buffer)
	if rule.TargetTriggered {
		t.Fatal("target should rearm after leaving range")
	}
}
