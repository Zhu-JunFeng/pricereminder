package apns

import (
	"encoding/json"
	"testing"
)

func TestTargetAlertPayload(t *testing.T) {
	payload, err := makeAlertPayload("event-1", json.RawMessage(`{
		"symbol":"BTCUSDT",
		"triggers":[{"kind":"target","direction":"fall","targetPrice":"95000","price":"94999"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		APS struct {
			Alert struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			} `json:"alert"`
		} `json:"aps"`
		EventID string `json:"eventId"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.APS.Alert.Title != "BTCUSDT 价格预警" || result.APS.Alert.Body != "达到或低于目标价 95000 · 最新价 94999" || result.EventID != "event-1" {
		t.Fatalf("unexpected target alert payload: %#v", result)
	}
}

func TestMarketPercentageAlertPayload(t *testing.T) {
	payload, err := makeAlertPayload("event-market", json.RawMessage(`{
		"symbol":"ETHUSDT",
		"triggers":[{"kind":"market_percentage","direction":"rise","changePct":"4.0816","thresholdPct":"4","windowMinutes":5,"price":"102"}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		APS struct {
			Alert struct {
				Title string `json:"title"`
				Body  string `json:"body"`
			} `json:"alert"`
		} `json:"aps"`
	}
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatal(err)
	}
	if result.APS.Alert.Title != "ETHUSDT 全市场预警" ||
		result.APS.Alert.Body != "5分钟上涨 4.0816%（阈值 4%） · 最新价 102" {
		t.Fatalf("unexpected market alert payload: %#v", result)
	}
}
