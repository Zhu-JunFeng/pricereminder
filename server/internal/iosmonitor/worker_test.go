package iosmonitor

import (
	"encoding/json"
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
