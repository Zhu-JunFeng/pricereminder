package apns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrNotConfigured = errors.New("APNs credentials are not configured")

type Client struct {
	keyID, teamID, bundleID string
	privateKey              *ecdsa.PrivateKey
	http                    *http.Client
	mu                      sync.Mutex
	jwt                     string
	jwtCreatedAt            time.Time
}

func (c *Client) Configured() bool { return c.privateKey != nil }

func New(keyID, teamID, privateKeyText, bundleID string) (*Client, error) {
	client := &Client{keyID: keyID, teamID: teamID, bundleID: bundleID, http: &http.Client{Timeout: 15 * time.Second}}
	if keyID == "" && teamID == "" && privateKeyText == "" {
		return client, nil
	}
	if keyID == "" || teamID == "" || privateKeyText == "" {
		return nil, errors.New("APNS_KEY_ID, APNS_TEAM_ID, and APNS_PRIVATE_KEY must be configured together")
	}
	block, _ := pem.Decode([]byte(strings.ReplaceAll(privateKeyText, `\n`, "\n")))
	if block == nil {
		return nil, errors.New("APNS_PRIVATE_KEY is not valid PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse APNs private key: %w", err)
	}
	key, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("APNs private key is not EC")
	}
	client.privateKey = key
	return client, nil
}

func (c *Client) SendAlert(ctx context.Context, token, environment, eventID string, payload json.RawMessage) error {
	requestPayload, err := makeAlertPayload(eventID, payload)
	if err != nil {
		return err
	}
	return c.send(ctx, token, environment, c.bundleID, "alert", "10", requestPayload)
}

func makeAlertPayload(eventID string, payload json.RawMessage) ([]byte, error) {
	var event struct {
		Symbol    string `json:"symbol"`
		EventTime int64  `json:"eventTime"`
		Triggers  []struct {
			Kind          string `json:"kind"`
			Direction     string `json:"direction"`
			ChangePct     string `json:"changePct"`
			ThresholdPct  string `json:"thresholdPct"`
			WindowMinutes int    `json:"windowMinutes"`
			TargetPrice   string `json:"targetPrice"`
			Price         string `json:"price"`
		} `json:"triggers"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		return nil, err
	}
	if len(event.Triggers) == 0 {
		return nil, errors.New("iOS event has no triggers")
	}
	parts := make([]string, 0, len(event.Triggers))
	for _, trigger := range event.Triggers {
		if trigger.Kind == "target" {
			direction := "达到或高于"
			if trigger.Direction == "fall" {
				direction = "达到或低于"
			}
			parts = append(parts, fmt.Sprintf("%s目标价 %s", direction, trigger.TargetPrice))
			continue
		}
		direction := "上涨"
		if trigger.Direction == "fall" {
			direction = "下跌"
		}
		parts = append(parts, fmt.Sprintf("%d分钟%s %s%%（阈值 %s%%）", trigger.WindowMinutes, direction, trigger.ChangePct, trigger.ThresholdPct))
	}
	body := strings.Join(parts, "；") + " · 最新价 " + event.Triggers[0].Price
	title := event.Symbol + " 价格预警"
	if event.Triggers[0].Kind == "market_percentage" {
		title = event.Symbol + " 全市场预警"
	}
	return json.Marshal(map[string]any{
		"aps": map[string]any{
			"alert": map[string]string{"title": title, "body": body},
			"sound": "default",
		},
		"eventId": eventID,
	})
}

func (c *Client) SendLiveActivity(ctx context.Context, token, environment, symbol, price string, eventTime int64) error {
	payload, _ := json.Marshal(map[string]any{
		"aps": map[string]any{
			"timestamp": eventTime / 1_000,
			"event":     "update",
			"content-state": map[string]any{
				"symbol": symbol, "price": price, "direction": "flat", "eventTime": eventTime,
			},
			"stale-date": eventTime/1_000 + 30,
		},
	})
	return c.send(ctx, token, environment, c.bundleID+".push-type.liveactivity", "liveactivity", "5", payload)
}

func (c *Client) send(ctx context.Context, deviceToken, environment, topic, pushType, priority string, payload []byte) error {
	if c.privateKey == nil {
		return ErrNotConfigured
	}
	if deviceToken == "" {
		return errors.New("APNs device token is empty")
	}
	host := "https://api.push.apple.com"
	if environment == "sandbox" {
		host = "https://api.sandbox.push.apple.com"
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, host+"/3/device/"+deviceToken, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	token, err := c.authorizationToken()
	if err != nil {
		return err
	}
	request.Header.Set("authorization", "bearer "+token)
	request.Header.Set("apns-topic", topic)
	request.Header.Set("apns-push-type", pushType)
	request.Header.Set("apns-priority", priority)
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(response.Body, 4<<10))
	return fmt.Errorf("APNs returned %s: %s", response.Status, strings.TrimSpace(string(body)))
}

func (c *Client) authorizationToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.jwt != "" && time.Since(c.jwtCreatedAt) < 50*time.Minute {
		return c.jwt, nil
	}
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": c.keyID})
	claims, _ := json.Marshal(map[string]any{"iss": c.teamID, "iat": time.Now().Unix()})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256Sum([]byte(unsigned))
	r, s, err := ecdsa.Sign(rand.Reader, c.privateKey, digest)
	if err != nil {
		return "", err
	}
	signature := append(padded(r, 32), padded(s, 32)...)
	c.jwt = unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	c.jwtCreatedAt = time.Now()
	return c.jwt, nil
}

func sha256Sum(value []byte) []byte {
	sum := sha256.Sum256(value)
	return sum[:]
}

func padded(value *big.Int, size int) []byte {
	result := make([]byte, size)
	bytes := value.Bytes()
	copy(result[size-len(bytes):], bytes)
	return result
}
