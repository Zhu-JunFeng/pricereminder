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
