package rules

import "testing"

func TestMarketScannerUsesRollingExtremesAndResetsTriggeredDirection(t *testing.T) {
	scanner := NewMarketScanner()
	rule, err := NewMarketRule("market-1", 5, "4")
	if err != nil {
		t.Fatal(err)
	}
	points := []struct {
		at    int64
		price string
	}{
		{0, "100"}, {30_000, "98"}, {60_000, "102"}, {61_000, "97"}, {62_000, "102"},
	}
	var directions []Direction
	for _, item := range points {
		got := scanner.Evaluate(rule, mustPoint(t, "BTCUSDT", item.price, item.at))
		for _, trigger := range got {
			directions = append(directions, trigger.Direction)
		}
	}
	if len(directions) != 3 || directions[0] != Rise || directions[1] != Fall || directions[2] != Rise {
		t.Fatalf("unexpected directions after independent resets: %#v", directions)
	}
}

func TestMarketScannerKeepsSparsePricesInsideWindow(t *testing.T) {
	scanner := NewMarketScanner()
	rule, _ := NewMarketRule("market-1", 1, "5")
	if got := scanner.Evaluate(rule, mustPoint(t, "ETHUSDT", "100", 0)); len(got) != 0 {
		t.Fatal(got)
	}
	if got := scanner.Evaluate(rule, mustPoint(t, "ETHUSDT", "105", 40_001)); len(got) != 1 {
		t.Fatalf("expected sparse in-window trigger: %#v", got)
	}
}
