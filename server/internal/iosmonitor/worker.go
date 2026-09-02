package iosmonitor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"pricereminder/server/internal/apns"
	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/pricebuffer"
	"pricereminder/server/internal/rules"
	"pricereminder/server/internal/store"
)

type Rule struct {
	ID              string                `json:"id"`
	Symbol          string                `json:"symbol"`
	Kind            rules.Kind            `json:"kind"`
	WindowMinutes   int                   `json:"windowMinutes"`
	ThresholdText   string                `json:"thresholdText"`
	IsEnabled       bool                  `json:"isEnabled"`
	RiseTriggered   bool                  `json:"riseTriggered"`
	FallTriggered   bool                  `json:"fallTriggered"`
	TargetDirection rules.TargetDirection `json:"targetDirection,omitempty"`
	TargetPriceText string                `json:"targetPriceText,omitempty"`
	TargetTriggered bool                  `json:"targetTriggered,omitempty"`
}

type Snapshot struct {
	Version           int64              `json:"version"`
	MonitoringEnabled bool               `json:"monitoringEnabled"`
	Rules             []Rule             `json:"rules"`
	MarketRules       []rules.MarketRule `json:"marketRules,omitempty"`
}

type cachedSnapshot struct {
	DeviceID   uuid.UUID
	Snapshot   Snapshot
	LeaseUntil time.Time
}

type Worker struct {
	store          *store.Store
	buffer         *pricebuffer.Buffer
	push           *apns.Client
	logger         *slog.Logger
	prices         chan domain.PricePoint
	marketPrices   chan domain.PricePoint
	mu             sync.RWMutex
	items          map[uuid.UUID]cachedSnapshot
	marketScanners map[uuid.UUID]*rules.MarketScanner
}

func New(dataStore *store.Store, buffer *pricebuffer.Buffer, push *apns.Client, logger *slog.Logger) *Worker {
	return &Worker{
		store: dataStore, buffer: buffer, push: push, logger: logger,
		prices: make(chan domain.PricePoint, 2048), marketPrices: make(chan domain.PricePoint, 8192),
		items: make(map[uuid.UUID]cachedSnapshot), marketScanners: make(map[uuid.UUID]*rules.MarketScanner),
	}
}

func ValidateSnapshot(snapshot Snapshot, validSymbol func(string) bool) (Snapshot, error) {
	if snapshot.Version < 1 {
		return Snapshot{}, errors.New("version must be positive")
	}
	if len(snapshot.Rules)+len(snapshot.MarketRules) > 50 {
		return Snapshot{}, errors.New("每台设备最多 50 条规则")
	}
	marketSeen := make(map[string]struct{}, len(snapshot.MarketRules))
	for index := range snapshot.MarketRules {
		item := &snapshot.MarketRules[index]
		if _, err := uuid.Parse(item.ID); err != nil {
			return Snapshot{}, errors.New("market rule id must be UUID")
		}
		rule, err := rules.NewMarketRule(item.ID, item.WindowMinutes, item.ThresholdText)
		if err != nil {
			return Snapshot{}, err
		}
		rule.Enabled = item.Enabled
		*item = rule
		key := strconv.Itoa(item.WindowMinutes) + ":" + item.ThresholdText
		if _, exists := marketSeen[key]; exists {
			return Snapshot{}, errors.New("相同窗口和阈值的全市场规则已存在")
		}
		marketSeen[key] = struct{}{}
	}
	seen := make(map[string]struct{}, len(snapshot.Rules))
	for index := range snapshot.Rules {
		item := &snapshot.Rules[index]
		if item.Kind == "" {
			item.Kind = rules.Percentage
		}
		item.Symbol = strings.ToUpper(strings.TrimSpace(item.Symbol))
		if _, err := uuid.Parse(item.ID); err != nil {
			return Snapshot{}, errors.New("rule id must be UUID")
		}
		if !validSymbol(item.Symbol) {
			return Snapshot{}, errors.New("合约不存在或不可交易: " + item.Symbol)
		}
		var rule rules.Rule
		var err error
		if item.Kind == rules.Target {
			rule, err = rules.NewTargetRule(item.ID, item.Symbol, item.TargetDirection, item.TargetPriceText)
		} else {
			rule, err = rules.NewRule(item.ID, item.Symbol, item.WindowMinutes, item.ThresholdText)
		}
		if err != nil {
			return Snapshot{}, err
		}
		item.Kind = rule.Kind
		if item.Kind == rules.Target {
			item.TargetPriceText = rule.TargetPriceText
		} else {
			item.ThresholdText = rule.ThresholdPct.String()
		}
		key := string(item.Kind) + ":" + item.Symbol + ":" + strconv.Itoa(item.WindowMinutes) + ":" +
			item.ThresholdText + ":" + string(item.TargetDirection) + ":" + item.TargetPriceText
		if _, exists := seen[key]; exists {
			return Snapshot{}, errors.New("相同合约、窗口和阈值的规则已存在")
		}
		seen[key] = struct{}{}
	}
	return snapshot, nil
}

func (w *Worker) Load(ctx context.Context) error {
	items, err := w.store.IOSSnapshots(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		var snapshot Snapshot
		if err := json.Unmarshal(item.Payload, &snapshot); err != nil {
			w.logger.Error("invalid stored iOS snapshot", "device_id", item.DeviceID, "error", err)
			continue
		}
		lease := time.Time{}
		if item.LeaseUntil != nil {
			lease = *item.LeaseUntil
		}
		w.items[item.DeviceID] = cachedSnapshot{DeviceID: item.DeviceID, Snapshot: snapshot, LeaseUntil: lease}
	}
	return nil
}

func (w *Worker) ReplaceSnapshot(deviceID uuid.UUID, snapshot Snapshot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	current := w.items[deviceID]
	w.items[deviceID] = cachedSnapshot{DeviceID: deviceID, Snapshot: snapshot, LeaseUntil: current.LeaseUntil}
	delete(w.marketScanners, deviceID)
}

func (w *Worker) RenewLease(deviceID uuid.UUID, version int64, until time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	item, ok := w.items[deviceID]
	if !ok || item.Snapshot.Version != version {
		return
	}
	item.LeaseUntil = until
	w.items[deviceID] = item
}

func (w *Worker) Publish(point domain.PricePoint)       { w.prices <- point }
func (w *Worker) PublishMarket(point domain.PricePoint) { w.marketPrices <- point }

func (w *Worker) MarketEnabled() bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, cached := range w.items {
		if !cached.Snapshot.MonitoringEnabled {
			continue
		}
		for _, rule := range cached.Snapshot.MarketRules {
			if rule.Enabled {
				return true
			}
		}
	}
	return false
}

func (w *Worker) Run(ctx context.Context) {
	deliveryTicker := time.NewTicker(15 * time.Second)
	cleanupTicker := time.NewTicker(time.Hour)
	defer deliveryTicker.Stop()
	defer cleanupTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case point := <-w.prices:
			w.evaluate(ctx, point)
			w.updateLiveActivities(ctx, point)
		case point := <-w.marketPrices:
			w.evaluateMarket(ctx, point)
		case <-deliveryTicker.C:
			w.deliverPending(ctx)
		case <-cleanupTicker.C:
			if err := w.store.CleanupIOSData(ctx); err != nil {
				w.logger.Error("clean iOS data", "error", err)
			}
		}
	}
}

func (w *Worker) evaluateMarket(ctx context.Context, point domain.PricePoint) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	for deviceID, cached := range w.items {
		if !cached.Snapshot.MonitoringEnabled || cached.LeaseUntil.After(now) || len(cached.Snapshot.MarketRules) == 0 {
			delete(w.marketScanners, deviceID)
			continue
		}
		scanner := w.marketScanners[deviceID]
		if scanner == nil {
			scanner = rules.NewMarketScanner()
			w.marketScanners[deviceID] = scanner
		}
		ids := make(map[string]struct{})
		var triggers []rules.Trigger
		for _, rule := range cached.Snapshot.MarketRules {
			if !rule.Enabled {
				continue
			}
			ids[rule.ID] = struct{}{}
			triggers = append(triggers, scanner.Evaluate(rule, point)...)
		}
		scanner.RetainRules(ids)
		if len(triggers) == 0 {
			continue
		}
		eventID, payload := makeEvent(deviceID, point, triggers)
		created, err := w.store.CreateIOSEventIfBackground(ctx, deviceID, cached.Snapshot.Version, eventID, payload)
		if err != nil {
			w.logger.Error("create iOS market event", "device_id", deviceID, "error", err)
		} else if created {
			w.logger.Info("iOS background market alert created", "device_id", deviceID, "event_id", eventID)
		}
	}
}

func (w *Worker) evaluate(ctx context.Context, point domain.PricePoint) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	for deviceID, cached := range w.items {
		if !cached.Snapshot.MonitoringEnabled || cached.LeaseUntil.After(now) {
			continue
		}
		var triggers []rules.Trigger
		stateChanged := false
		for index := range cached.Snapshot.Rules {
			item := &cached.Snapshot.Rules[index]
			if item.Symbol != point.Symbol {
				continue
			}
			var rule rules.Rule
			var err error
			if item.Kind == rules.Target {
				rule, err = rules.NewTargetRule(item.ID, item.Symbol, item.TargetDirection, item.TargetPriceText)
			} else {
				rule, err = rules.NewRule(item.ID, item.Symbol, item.WindowMinutes, item.ThresholdText)
			}
			if err != nil {
				continue
			}
			rule.Enabled = item.IsEnabled
			rule.RiseTriggered = item.RiseTriggered
			rule.FallTriggered = item.FallTriggered
			rule.TargetTriggered = item.TargetTriggered
			beforeRise, beforeFall, beforeTarget := rule.RiseTriggered, rule.FallTriggered, rule.TargetTriggered
			result, err := rules.Evaluate(&rule, point, w.buffer)
			if err != nil {
				w.logger.Error("evaluate iOS rule", "device_id", deviceID, "rule_id", item.ID, "error", err)
				continue
			}
			item.RiseTriggered = rule.RiseTriggered
			item.FallTriggered = rule.FallTriggered
			item.TargetTriggered = rule.TargetTriggered
			stateChanged = stateChanged || beforeRise != rule.RiseTriggered || beforeFall != rule.FallTriggered || beforeTarget != rule.TargetTriggered
			triggers = append(triggers, result...)
		}
		if stateChanged {
			payload, _ := json.Marshal(cached.Snapshot)
			updated, err := w.store.UpdateIOSSnapshotState(ctx, deviceID, cached.Snapshot.Version, payload)
			if err != nil {
				w.logger.Error("persist iOS rule state", "device_id", deviceID, "error", err)
				continue
			}
			if !updated {
				continue
			}
			w.items[deviceID] = cached
		}
		if len(triggers) == 0 {
			continue
		}
		eventID, payload := makeEvent(deviceID, point, triggers)
		created, err := w.store.CreateIOSEventIfBackground(ctx, deviceID, cached.Snapshot.Version, eventID, payload)
		if err != nil {
			w.logger.Error("create iOS event", "device_id", deviceID, "error", err)
		} else if created {
			w.logger.Info("iOS background alert created", "device_id", deviceID, "event_id", eventID)
		}
	}
}

func (w *Worker) deliverPending(ctx context.Context) {
	if !w.push.Configured() {
		return
	}
	items, err := w.store.IOSEventsDueForDelivery(ctx, 100)
	if err != nil {
		w.logger.Error("load iOS events for delivery", "error", err)
		return
	}
	for _, item := range items {
		err := w.push.SendAlert(ctx, item.PushToken, item.Environment, item.ID, item.Payload)
		if markErr := w.store.MarkIOSEventDeliveryAttempt(ctx, item.ID, err == nil); markErr != nil {
			w.logger.Error("mark iOS event delivery", "event_id", item.ID, "error", markErr)
		}
		if err != nil {
			w.logger.Error("send iOS alert", "event_id", item.ID, "error", err)
		}
	}
}

func (w *Worker) updateLiveActivities(ctx context.Context, point domain.PricePoint) {
	if !w.push.Configured() {
		return
	}
	items, err := w.store.IOSLiveActivitiesDue(ctx, point.Symbol)
	if err != nil {
		w.logger.Error("load Live Activities", "symbol", point.Symbol, "error", err)
		return
	}
	for _, item := range items {
		if err := w.push.SendLiveActivity(ctx, item.PushToken, item.Environment, point.Symbol, point.PriceText, point.EventTime); err != nil {
			w.logger.Error("update Live Activity", "activity_id", item.ActivityID, "error", err)
			continue
		}
		if err := w.store.MarkIOSLiveActivityUpdated(ctx, item.ActivityID); err != nil {
			w.logger.Error("mark Live Activity updated", "activity_id", item.ActivityID, "error", err)
		}
	}
}

func makeEvent(deviceID uuid.UUID, point domain.PricePoint, triggers []rules.Trigger) (string, json.RawMessage) {
	sort.Slice(triggers, func(i, j int) bool {
		if triggers[i].RuleID == triggers[j].RuleID {
			return triggers[i].Direction < triggers[j].Direction
		}
		return triggers[i].RuleID < triggers[j].RuleID
	})
	identity := deviceID.String() + ":" + point.Symbol + ":" + strconv.FormatInt(point.EventTime, 10)
	for _, trigger := range triggers {
		identity += ":" + trigger.RuleID + ":" + string(trigger.Direction)
	}
	digest := sha256.Sum256([]byte(identity))
	eventID := hex.EncodeToString(digest[:])
	payload, _ := json.Marshal(map[string]any{
		"id": eventID, "symbol": point.Symbol, "eventTime": point.EventTime, "triggers": triggers,
	})
	return eventID, payload
}
