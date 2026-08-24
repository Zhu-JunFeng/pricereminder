package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type registration struct {
	DeviceID  string `json:"deviceId"`
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expiresAt"`
}

type expiry struct {
	ExpiresAt int64 `json:"expiresAt"`
}

func TestLocalEndToEnd(t *testing.T) {
	baseURL := os.Getenv("PRICE_REMINDER_E2E_URL")
	if baseURL == "" {
		t.Skip("PRICE_REMINDER_E2E_URL is not set")
	}
	client := &http.Client{Timeout: 10 * time.Second}

	var health struct {
		Status    string `json:"status"`
		Contracts int    `json:"contracts"`
	}
	doJSON(t, client, http.MethodGet, baseURL+"/healthz", "", nil, http.StatusOK, &health)
	if health.Status != "ok" || health.Contracts < 100 {
		t.Fatalf("unexpected health response: %#v", health)
	}

	ios := register(t, client, baseURL, "ios")
	android := register(t, client, baseURL, "android")
	var refreshed expiry
	doJSON(t, client, http.MethodPost, baseURL+"/v1/devices/refresh", ios.Token, nil, http.StatusOK, &refreshed)
	if refreshed.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatal("device token was not renewed")
	}
	doJSON(t, client, http.MethodPost, baseURL+"/v1/devices/refresh", "invalid-token", nil, http.StatusUnauthorized, nil)
	doJSON(t, client, http.MethodPut, baseURL+"/v1/subscriptions", ios.Token,
		map[string]any{"symbols": []string{"BTCUSDT"}}, http.StatusOK, nil)
	doJSON(t, client, http.MethodPut, baseURL+"/v1/subscriptions", ios.Token,
		map[string]any{"symbols": []string{"NOTAREALCONTRACT"}}, http.StatusBadRequest, nil)

	rule := map[string]any{
		"id": uuid.NewString(), "symbol": "btcusdt", "windowMinutes": 1,
		"thresholdText": "1.00", "isEnabled": true,
		"riseTriggered": false, "fallTriggered": false,
	}
	rules := map[string]any{"version": 2, "monitoringEnabled": true, "rules": []any{rule}}
	doJSON(t, client, http.MethodPut, baseURL+"/v1/ios/rules", ios.Token, rules, http.StatusOK, nil)
	var storedRules map[string]any
	doJSON(t, client, http.MethodGet, baseURL+"/v1/ios/rules", ios.Token, nil, http.StatusOK, &storedRules)
	if storedRules["version"] != float64(2) || storedRules["monitoringEnabled"] != true {
		t.Fatalf("unexpected stored iOS rules: %#v", storedRules)
	}
	doJSON(t, client, http.MethodPut, baseURL+"/v1/ios/rules", ios.Token,
		map[string]any{"version": 1, "monitoringEnabled": true, "rules": []any{rule}}, http.StatusConflict, nil)
	doJSON(t, client, http.MethodPut, baseURL+"/v1/ios/rules", android.Token, rules, http.StatusForbidden, nil)

	var lease struct {
		LeaseUntil int64 `json:"leaseUntil"`
	}
	doJSON(t, client, http.MethodPost, baseURL+"/v1/ios/lease", ios.Token,
		map[string]any{"version": 2}, http.StatusOK, &lease)
	if lease.LeaseUntil <= time.Now().UnixMilli() {
		t.Fatal("lease was not renewed")
	}
	doJSON(t, client, http.MethodPost, baseURL+"/v1/ios/lease", ios.Token,
		map[string]any{"version": 1}, http.StatusConflict, nil)

	fakeToken := strings.Repeat("ab", 32)
	doJSON(t, client, http.MethodPut, baseURL+"/v1/ios/push-token", ios.Token,
		map[string]any{"token": fakeToken, "environment": "sandbox"}, http.StatusOK, nil)
	activityID := uuid.NewString()
	doJSON(t, client, http.MethodPut, baseURL+"/v1/ios/live-activities", ios.Token,
		map[string]any{
			"activityId": activityID, "pushToken": fakeToken, "symbol": "BTCUSDT",
			"environment": "sandbox", "expiresAt": time.Now().Add(time.Hour).UnixMilli(),
		}, http.StatusOK, nil)
	doJSON(t, client, http.MethodDelete, baseURL+"/v1/ios/live-activities/"+activityID, ios.Token, nil, http.StatusNoContent, nil)

	var events struct {
		Events []json.RawMessage `json:"events"`
	}
	doJSON(t, client, http.MethodGet, baseURL+"/v1/ios/events", ios.Token, nil, http.StatusOK, &events)
	if len(events.Events) != 0 {
		t.Fatalf("new device should not have events: %#v", events.Events)
	}

	assertDatabaseState(t, ios, android, activityID, fakeToken, lease.LeaseUntil)
	streamPricesAcrossSubscriptionUpdate(t, client, baseURL, ios.Token)
}

func assertDatabaseState(t *testing.T, ios, android registration, activityID, fakeToken string, leaseUntil int64) {
	t.Helper()
	databaseURL := os.Getenv("PRICE_REMINDER_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Log("PRICE_REMINDER_E2E_DATABASE_URL is not set; database assertions skipped")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(ctx)

	var platform, pushToken, pushEnvironment string
	var tokenHashLength int
	if err := connection.QueryRow(ctx, `
		SELECT platform, octet_length(token_hash), COALESCE(push_token, ''), push_environment
		FROM devices WHERE id = $1`, ios.DeviceID,
	).Scan(&platform, &tokenHashLength, &pushToken, &pushEnvironment); err != nil {
		t.Fatal(err)
	}
	if platform != "ios" || tokenHashLength != 32 || pushToken != fakeToken || pushEnvironment != "sandbox" {
		t.Fatalf("unexpected persisted iOS device: platform=%s hash=%d token=%s environment=%s", platform, tokenHashLength, pushToken, pushEnvironment)
	}
	if err := connection.QueryRow(ctx, `SELECT platform FROM devices WHERE id = $1`, android.DeviceID).Scan(&platform); err != nil {
		t.Fatal(err)
	}
	if platform != "android" {
		t.Fatalf("unexpected Android platform: %s", platform)
	}

	var symbols []string
	rows, err := connection.Query(ctx, `SELECT symbol FROM subscriptions WHERE device_id = $1 ORDER BY symbol`, ios.DeviceID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		symbols = append(symbols, symbol)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(symbols) != 1 || symbols[0] != "BTCUSDT" {
		t.Fatalf("unexpected persisted subscriptions: %#v", symbols)
	}

	var version int64
	var payload struct {
		Version           int64 `json:"version"`
		MonitoringEnabled bool  `json:"monitoringEnabled"`
	}
	var persistedLease time.Time
	if err := connection.QueryRow(ctx, `
		SELECT version, payload, foreground_lease_until
		FROM ios_rule_snapshots WHERE device_id = $1`, ios.DeviceID,
	).Scan(&version, &payload, &persistedLease); err != nil {
		t.Fatal(err)
	}
	if version != 2 || payload.Version != 2 || !payload.MonitoringEnabled {
		t.Fatalf("unexpected persisted rule snapshot: version=%d payload=%#v", version, payload)
	}
	if difference := persistedLease.UnixMilli() - leaseUntil; difference < -1 || difference > 1 {
		t.Fatalf("lease mismatch: database=%d api=%d", persistedLease.UnixMilli(), leaseUntil)
	}

	var activityCount int
	if err := connection.QueryRow(ctx, `SELECT count(*) FROM ios_live_activities WHERE activity_id = $1`, activityID).Scan(&activityCount); err != nil {
		t.Fatal(err)
	}
	if activityCount != 0 {
		t.Fatalf("deleted Live Activity still exists: %s", activityID)
	}
}

func register(t *testing.T, client *http.Client, baseURL, platform string) registration {
	t.Helper()
	var result registration
	doJSON(t, client, http.MethodPost, baseURL+"/v1/devices/register", "",
		map[string]any{"platform": platform, "displayName": "E2E"}, http.StatusCreated, &result)
	if result.Token == "" || result.DeviceID == "" || result.ExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("invalid registration: %#v", result)
	}
	return result
}

func streamPricesAcrossSubscriptionUpdate(t *testing.T, client *http.Client, baseURL, token string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.Replace(baseURL, "http://", "ws://", 1)+"/v1/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	connection, _, err := websocket.Dial(ctx, request.URL.String(), &websocket.DialOptions{HTTPHeader: request.Header})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "test complete")
	if err := wsjson.Write(ctx, connection, map[string]any{"type": "resume", "lastEventTime": map[string]int64{}}); err != nil {
		t.Fatal(err)
	}
	ready := false
	btcPrice := false
	ethPrice := false
	subscriptionUpdated := false
	for !ready || !btcPrice || !ethPrice {
		var message struct {
			Type      string `json:"type"`
			Symbol    string `json:"symbol"`
			Price     string `json:"price"`
			EventTime int64  `json:"eventTime"`
		}
		if err := wsjson.Read(ctx, connection, &message); err != nil {
			t.Fatal(err)
		}
		ready = ready || message.Type == "ready"
		if message.Type == "price" && message.Symbol == "BTCUSDT" && message.Price != "" && message.EventTime > 0 {
			btcPrice = true
			if !subscriptionUpdated {
				doJSON(t, client, http.MethodPut, baseURL+"/v1/subscriptions", token,
					map[string]any{"symbols": []string{"ETHUSDT"}}, http.StatusOK, nil)
				subscriptionUpdated = true
			}
		}
		if message.Type == "price" && message.Symbol == "ETHUSDT" && message.Price != "" && message.EventTime > 0 {
			ethPrice = true
		}
	}
}

func doJSON(
	t *testing.T, client *http.Client, method, url, token string, body any, expectedStatus int, target any,
) {
	t.Helper()
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, url, bytes.NewReader(encoded))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != expectedStatus {
		t.Fatalf("%s %s returned %s, want %d", method, url, response.Status, expectedStatus)
	}
	if target != nil {
		if err := json.NewDecoder(response.Body).Decode(target); err != nil {
			t.Fatal(fmt.Errorf("decode %s %s: %w", method, url, err))
		}
	}
}
