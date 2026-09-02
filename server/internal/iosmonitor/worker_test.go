package iosmonitor

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/google/uuid"

	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/rules"
)

func TestValidateSnapshotNormalizesAndRejectsDuplicates(t *testing.T) {
	ruleID := uuid.NewString()
	snapshot, err := ValidateSnapshot(Snapshot{
		Version: 1, MonitoringEnabled: true,
		Rules: []Rule{{ID: ruleID, Symbol: " btcusdt ", WindowMinutes: 5, ThresholdText: "1.00", IsEnabled: true}},
	}, func(symbol string) bool { return symbol == "BTCUSDT" })
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Rules[0].Symbol != "BTCUSDT" || snapshot.Rules[0].ThresholdText != "1" {
		t.Fatalf("snapshot was not normalized: %#v", snapshot.Rules[0])
	}

	snapshot.Rules = append(snapshot.Rules, Rule{ID: uuid.NewString(), Symbol: "BTCUSDT", WindowMinutes: 5, ThresholdText: "1.0"})
	if _, err := ValidateSnapshot(snapshot, func(string) bool { return true }); err == nil {
		t.Fatal("duplicate rule should be rejected")
	}
}

func TestValidateTargetSnapshotNormalizesAndRejectsDuplicates(t *testing.T) {
	snapshot, err := ValidateSnapshot(Snapshot{
		Version: 2, MonitoringEnabled: true,
		Rules: []Rule{{
			ID: uuid.NewString(), Symbol: " btcusdt ", Kind: rules.Target,
			TargetDirection: rules.Above, TargetPriceText: "105.00", IsEnabled: true,
		}},
	}, func(symbol string) bool { return symbol == "BTCUSDT" })
	if err != nil {
		t.Fatal(err)
	}
	item := snapshot.Rules[0]
	if item.Kind != rules.Target || item.Symbol != "BTCUSDT" || item.TargetPriceText != "105" || item.TargetDirection != rules.Above {
		t.Fatalf("target snapshot was not normalized: %#v", item)
	}

	snapshot.Rules = append(snapshot.Rules, Rule{
		ID: uuid.NewString(), Symbol: "BTCUSDT", Kind: rules.Target,
		TargetDirection: rules.Above, TargetPriceText: "105.0", IsEnabled: true,
	})
	if _, err := ValidateSnapshot(snapshot, func(string) bool { return true }); err == nil {
		t.Fatal("duplicate target rule should be rejected")
	}
}

func TestValidateMarketSnapshotNormalizesAndRejectsDuplicates(t *testing.T) {
	snapshot, err := ValidateSnapshot(Snapshot{
		Version: 3, MonitoringEnabled: true,
		MarketRules: []rules.MarketRule{{
			ID: uuid.NewString(), WindowMinutes: 5, ThresholdText: "1.00", Enabled: true,
		}},
	}, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.MarketRules[0].ThresholdText != "1" || !snapshot.MarketRules[0].Enabled {
		t.Fatalf("market snapshot was not normalized: %#v", snapshot.MarketRules[0])
	}

	snapshot.MarketRules = append(snapshot.MarketRules, rules.MarketRule{
		ID: uuid.NewString(), WindowMinutes: 5, ThresholdText: "1.0", Enabled: true,
	})
	if _, err := ValidateSnapshot(snapshot, func(string) bool { return true }); err == nil {
		t.Fatal("duplicate market rule should be rejected")
	}
}

func TestValidateSnapshotEnforcesCombinedRuleLimit(t *testing.T) {
	snapshot := Snapshot{Version: 4, MonitoringEnabled: true}
	for index := 0; index < 49; index++ {
		snapshot.Rules = append(snapshot.Rules, Rule{
			ID: uuid.NewString(), Symbol: "BTCUSDT", Kind: rules.Target,
			TargetDirection: rules.Above, TargetPriceText: strconv.Itoa(index + 1), IsEnabled: true,
		})
	}
	snapshot.MarketRules = []rules.MarketRule{
		{ID: uuid.NewString(), WindowMinutes: 5, ThresholdText: "1", Enabled: true},
		{ID: uuid.NewString(), WindowMinutes: 10, ThresholdText: "2", Enabled: true},
	}
	if _, err := ValidateSnapshot(snapshot, func(string) bool { return true }); err == nil {
		t.Fatal("combined rule limit should be enforced")
	}
}

func TestReplaceSnapshotClearsMarketScannerState(t *testing.T) {
	deviceID := uuid.New()
	worker := &Worker{
		items: map[uuid.UUID]cachedSnapshot{},
		marketScanners: map[uuid.UUID]*rules.MarketScanner{
			deviceID: rules.NewMarketScanner(),
		},
	}
	worker.ReplaceSnapshot(deviceID, Snapshot{Version: 5, MonitoringEnabled: true})
	if _, exists := worker.marketScanners[deviceID]; exists {
		t.Fatal("replaced snapshot must begin with fresh market scanner state")
	}
}

func TestEventIDIsDeterministicAcrossTriggerOrder(t *testing.T) {
	deviceID := uuid.New()
	point, err := domain.NewPricePoint("BTCUSDT", "105", 60_000)
	if err != nil {
		t.Fatal(err)
	}
	first := rules.Trigger{RuleID: uuid.NewString(), Direction: rules.Rise, EventTime: point.EventTime}
	second := rules.Trigger{RuleID: uuid.NewString(), Direction: rules.Fall, EventTime: point.EventTime}
	id1, payload1 := makeEvent(deviceID, point, []rules.Trigger{first, second})
	id2, payload2 := makeEvent(deviceID, point, []rules.Trigger{second, first})
	if id1 != id2 || !json.Valid(payload1) || !json.Valid(payload2) {
		t.Fatalf("event identity should be deterministic: %s %s", id1, id2)
	}
}
