package api

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"pricereminder/server/internal/iosmonitor"
	"pricereminder/server/internal/store"
)

const iosLeaseDuration = 25 * time.Second

func (s *Server) getIOSRules(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	payload, err := s.store.IOSSnapshotForDevice(r.Context(), device.ID)
	if err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "rules_not_found", "iOS 规则尚未同步")
			return
		}
		writeError(w, http.StatusInternalServerError, "load_failed", "iOS 规则读取失败")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) setIOSRules(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	var request iosmonitor.Snapshot
	if !decodeJSON(w, r, &request) {
		return
	}
	snapshot, err := iosmonitor.ValidateSnapshot(request, s.catalog.Contains)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_rules", err.Error())
		return
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_rules", "规则快照格式错误")
		return
	}
	if err := s.store.SaveIOSSnapshot(r.Context(), device.ID, snapshot.Version, payload); err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			writeError(w, http.StatusConflict, "stale_rule_version", "规则版本已过期")
			return
		}
		writeError(w, http.StatusInternalServerError, "save_failed", "iOS 规则保存失败")
		return
	}
	s.iosWorker.ReplaceSnapshot(device.ID, snapshot)
	writeJSON(w, http.StatusOK, map[string]any{"version": snapshot.Version})
}

func (s *Server) renewIOSLease(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	var request struct {
		Version int64 `json:"version"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	until, err := s.store.RenewIOSLease(r.Context(), device.ID, request.Version, iosLeaseDuration)
	if err != nil {
		if errors.Is(err, store.ErrVersionConflict) {
			writeError(w, http.StatusConflict, "rule_version_mismatch", "前台租约对应的规则版本不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "lease_failed", "前台租约续期失败")
		return
	}
	s.iosWorker.RenewLease(device.ID, request.Version, until)
	writeJSON(w, http.StatusOK, map[string]any{"leaseUntil": until.UnixMilli()})
}

func (s *Server) setIOSPushToken(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	var request struct {
		Token       string `json:"token"`
		Environment string `json:"environment"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Token = strings.ToLower(strings.TrimSpace(request.Token))
	if !validPushToken(request.Token) || (request.Environment != "sandbox" && request.Environment != "production") {
		writeError(w, http.StatusBadRequest, "invalid_push_token", "APNs token 或环境无效")
		return
	}
	if err := s.store.SetIOSPushToken(r.Context(), device.ID, request.Token, request.Environment); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", "APNs token 保存失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (s *Server) setIOSLiveActivity(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	var request struct {
		ActivityID  string `json:"activityId"`
		PushToken   string `json:"pushToken"`
		Symbol      string `json:"symbol"`
		Environment string `json:"environment"`
		ExpiresAt   int64  `json:"expiresAt"`
	}
	if !decodeJSON(w, r, &request) {
		return
	}
	request.Symbol = strings.ToUpper(strings.TrimSpace(request.Symbol))
	request.PushToken = strings.ToLower(strings.TrimSpace(request.PushToken))
	expiresAt := time.UnixMilli(request.ExpiresAt)
	if _, err := uuid.Parse(request.ActivityID); err != nil || !validPushToken(request.PushToken) || !s.catalog.Contains(request.Symbol) ||
		(request.Environment != "sandbox" && request.Environment != "production") || expiresAt.Before(time.Now()) || expiresAt.After(time.Now().Add(8*time.Hour)) {
		writeError(w, http.StatusBadRequest, "invalid_live_activity", "实时活动参数无效")
		return
	}
	if err := s.store.SaveIOSLiveActivity(r.Context(), store.IOSLiveActivity{
		ActivityID: request.ActivityID, DeviceID: device.ID, PushToken: request.PushToken,
		Symbol: request.Symbol, Environment: request.Environment, ExpiresAt: expiresAt,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, "save_failed", "实时活动 token 保存失败")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (s *Server) deleteIOSLiveActivity(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	activityID := r.PathValue("activityId")
	if _, err := uuid.Parse(activityID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_activity_id", "实时活动 ID 无效")
		return
	}
	if err := s.store.DeleteIOSLiveActivity(r.Context(), device.ID, activityID); err != nil {
		writeError(w, http.StatusInternalServerError, "delete_failed", "实时活动删除失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) getIOSEvents(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	limit := 100
	if value := r.URL.Query().Get("limit"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(w, http.StatusBadRequest, "invalid_limit", "limit 必须在 1 到 100 之间")
			return
		}
		limit = parsed
	}
	events, err := s.store.PendingIOSEvents(r.Context(), device.ID, r.URL.Query().Get("after"), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_failed", "后台提醒读取失败")
		return
	}
	payloads := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		payloads = append(payloads, event.Payload)
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": payloads})
}

func (s *Server) acknowledgeIOSEvent(w http.ResponseWriter, r *http.Request) {
	device, ok := requireIOSDevice(w, r)
	if !ok {
		return
	}
	eventID := r.PathValue("eventId")
	if len(eventID) != 64 {
		writeError(w, http.StatusBadRequest, "invalid_event_id", "提醒事件 ID 无效")
		return
	}
	if err := s.store.AcknowledgeIOSEvent(r.Context(), device.ID, eventID); err != nil {
		if store.IsNotFound(err) {
			writeError(w, http.StatusNotFound, "event_not_found", "提醒事件不存在")
			return
		}
		writeError(w, http.StatusInternalServerError, "ack_failed", "提醒确认失败")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func requireIOSDevice(w http.ResponseWriter, r *http.Request) (store.Device, bool) {
	device := currentDevice(r.Context())
	if device.Platform != "ios" {
		writeError(w, http.StatusForbidden, "ios_only", "该接口只允许 iOS 设备使用")
		return store.Device{}, false
	}
	return device, true
}

func validPushToken(value string) bool {
	if len(value) < 32 || len(value) > 256 || len(value)%2 != 0 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
