package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"pricereminder/server/internal/binance"
	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/iosmonitor"
	"pricereminder/server/internal/store"
	"pricereminder/server/internal/stream"
)

type Server struct {
	store         *store.Store
	catalog       *binance.Catalog
	hub           *stream.Hub
	allowedOrigin string
	logger        *slog.Logger
	iosWorker     *iosmonitor.Worker
}

type deviceContextKey struct{}

func New(dataStore *store.Store, catalog *binance.Catalog, hub *stream.Hub, iosWorker *iosmonitor.Worker, allowedOrigin string, logger *slog.Logger) *Server {
	return &Server{store: dataStore, catalog: catalog, hub: hub, iosWorker: iosWorker, allowedOrigin: allowedOrigin, logger: logger}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("POST /v1/devices/register", s.register)
	mux.Handle("POST /v1/devices/refresh", s.auth(http.HandlerFunc(s.refresh)))
	mux.Handle("GET /v1/contracts", s.auth(http.HandlerFunc(s.contracts)))
	mux.Handle("PUT /v1/subscriptions", s.auth(http.HandlerFunc(s.setSubscriptions)))
	mux.Handle("GET /v1/subscriptions", s.auth(http.HandlerFunc(s.getSubscriptions)))
	mux.Handle("GET /v1/stream", s.auth(http.HandlerFunc(s.priceStream)))
	mux.Handle("PUT /v1/ios/rules", s.auth(http.HandlerFunc(s.setIOSRules)))
	mux.Handle("GET /v1/ios/rules", s.auth(http.HandlerFunc(s.getIOSRules)))
	mux.Handle("POST /v1/ios/lease", s.auth(http.HandlerFunc(s.renewIOSLease)))
	mux.Handle("PUT /v1/ios/push-token", s.auth(http.HandlerFunc(s.setIOSPushToken)))
	mux.Handle("PUT /v1/ios/live-activities", s.auth(http.HandlerFunc(s.setIOSLiveActivity)))
	mux.Handle("DELETE /v1/ios/live-activities/{activityId}", s.auth(http.HandlerFunc(s.deleteIOSLiveActivity)))
	mux.Handle("GET /v1/ios/events", s.auth(http.HandlerFunc(s.getIOSEvents)))
	mux.Handle("POST /v1/ios/events/{eventId}/ack", s.auth(http.HandlerFunc(s.acknowledgeIOSEvent)))
	return s.recover(mux)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	symbols, latestEventTime := s.hub.Status()
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok", "contracts": len(s.catalog.All()),
		"priceSymbols": symbols, "latestEventTime": latestEventTime,
	})
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var request struct{ Platform, DisplayName string }
	if !decodeJSON(w, r, &request) {
		return
	}
	registration, err := s.store.Register(r.Context(), request.Platform, request.DisplayName)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_device", err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"deviceId": registration.ID, "token": registration.Token,
		"expiresAt": registration.ExpiresAt.UnixMilli(),
	})
}

func (s *Server) refresh(w http.ResponseWriter, r *http.Request) {
	device := currentDevice(r.Context())
	expiresAt, err := s.store.Refresh(r.Context(), device.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "refresh_failed", "设备续期失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"expiresAt": expiresAt.UnixMilli()})
}

func (s *Server) contracts(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"contracts": s.catalog.All()})
}

func (s *Server) setSubscriptions(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Symbols []string `json:"symbols"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	symbols, err := validateSymbols(request.Symbols, s.catalog)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_subscriptions", err.Error())
		return
	}
	device := currentDevice(r.Context())
	if err := s.store.SetSubscriptions(r.Context(), device.ID, symbols); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", "订阅保存失败")
		return
	}
	s.hub.UpdateSubscriptions(device.ID, symbols)
	writeJSON(w, http.StatusOK, map[string]any{"symbols": symbols})
}

func (s *Server) getSubscriptions(w http.ResponseWriter, r *http.Request) {
	device := currentDevice(r.Context())
	symbols, err := s.store.Subscriptions(r.Context(), device.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_failed", "订阅读取失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"symbols": symbols})
}

type resumeRequest struct {
	Type          string           `json:"type"`
	LastEventTime map[string]int64 `json:"lastEventTime"`
}

type priceMessage struct {
	Type      string `json:"type"`
	Symbol    string `json:"symbol"`
	Price     string `json:"price"`
	EventTime int64  `json:"eventTime"`
	Replay    bool   `json:"replay"`
}

func (s *Server) priceStream(w http.ResponseWriter, r *http.Request) {
	options := &websocket.AcceptOptions{}
	if s.allowedOrigin != "" {
		options.OriginPatterns = []string{s.allowedOrigin}
	}
	connection, err := websocket.Accept(w, r, options)
	if err != nil {
		return
	}
	defer connection.Close(websocket.StatusNormalClosure, "closed")
	connection.SetReadLimit(64 << 10)

	readContext, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	var resume resumeRequest
	err = wsjson.Read(readContext, connection, &resume)
	cancel()
	if err != nil || resume.Type != "resume" {
		connection.Close(websocket.StatusPolicyViolation, "resume message required")
		return
	}
	device := currentDevice(r.Context())
	symbols, err := s.store.Subscriptions(r.Context(), device.ID)
	if err != nil {
		connection.Close(websocket.StatusInternalError, "subscriptions unavailable")
		return
	}
	client, replay, historyGaps := s.hub.Subscribe(device.ID, symbols, resume.LastEventTime)
	defer s.hub.Unsubscribe(client)
	for _, symbol := range historyGaps {
		if err := wsjson.Write(r.Context(), connection, map[string]any{
			"type": "status", "symbol": symbol, "state": "warming_up", "reason": "history_gap",
		}); err != nil {
			return
		}
	}

	sort.Slice(replay, func(i, j int) bool { return replay[i].EventTime < replay[j].EventTime })
	for _, point := range replay {
		if err := writePrice(r.Context(), connection, point); err != nil {
			return
		}
	}
	if err := wsjson.Write(r.Context(), connection, map[string]any{"type": "ready", "serverTime": time.Now().UnixMilli()}); err != nil {
		return
	}
	_ = s.store.Touch(r.Context(), device.ID)

	for {
		select {
		case <-r.Context().Done():
			return
		case <-client.Overflow:
			connection.Close(websocket.StatusTryAgainLater, "terminal is too slow; reconnect for replay")
			return
		case point, ok := <-client.Messages:
			if !ok {
				return
			}
			if err := writePrice(r.Context(), connection, point); err != nil {
				return
			}
		}
	}
}

func writePrice(ctx context.Context, connection *websocket.Conn, point domain.PricePoint) error {
	return wsjson.Write(ctx, connection, priceMessage{Type: "price", Symbol: point.Symbol, Price: point.PriceText, EventTime: point.EventTime, Replay: point.Replay})
}

func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		device, err := s.store.Authenticate(r.Context(), token)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "设备令牌无效或已过期")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), deviceContextKey{}, device)))
	})
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("http handler panic", "panic", recovered, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal_error", "服务暂时不可用")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func currentDevice(ctx context.Context) store.Device {
	return ctx.Value(deviceContextKey{}).(store.Device)
}

func validateSymbols(values []string, catalog *binance.Catalog) ([]string, error) {
	if len(values) > 50 {
		return nil, errors.New("每台设备最多订阅 50 个合约")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		symbol := strings.ToUpper(strings.TrimSpace(value))
		if symbol == "" {
			continue
		}
		if !catalog.Contains(symbol) {
			return nil, errors.New("合约不存在或不可交易: " + symbol)
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		result = append(result, symbol)
	}
	sort.Strings(result)
	return result, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "请求内容格式错误")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
