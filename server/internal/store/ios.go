package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type IOSSnapshot struct {
	DeviceID        uuid.UUID
	Version         int64
	Payload         json.RawMessage
	LeaseUntil      *time.Time
	PushToken       string
	PushEnvironment string
}

type IOSEvent struct {
	ID          string          `json:"id"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"createdAt"`
	DeliveredAt *time.Time      `json:"-"`
	Attempts    int             `json:"-"`
	PushToken   string          `json:"-"`
	Environment string          `json:"-"`
}

type IOSLiveActivity struct {
	ActivityID  string
	DeviceID    uuid.UUID
	PushToken   string
	Symbol      string
	Environment string
	ExpiresAt   time.Time
}

func (s *Store) SaveIOSSnapshot(ctx context.Context, deviceID uuid.UUID, version int64, payload json.RawMessage) error {
	command, err := s.pool.Exec(ctx, `
		INSERT INTO ios_rule_snapshots (device_id, version, payload)
		VALUES ($1, $2, $3)
		ON CONFLICT (device_id) DO UPDATE
		SET version = EXCLUDED.version, payload = EXCLUDED.payload, updated_at = now()
		WHERE ios_rule_snapshots.version <= EXCLUDED.version`, deviceID, version, payload)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrVersionConflict
	}
	return nil
}

func (s *Store) IOSSnapshotForDevice(ctx context.Context, deviceID uuid.UUID) (json.RawMessage, error) {
	var payload json.RawMessage
	err := s.pool.QueryRow(ctx, `
		SELECT payload FROM ios_rule_snapshots
		WHERE device_id = $1`, deviceID).Scan(&payload)
	return payload, err
}

func (s *Store) RenewIOSLease(ctx context.Context, deviceID uuid.UUID, version int64, duration time.Duration) (time.Time, error) {
	until := time.Now().UTC().Add(duration)
	command, err := s.pool.Exec(ctx, `
		UPDATE ios_rule_snapshots
		SET foreground_lease_until = $3, updated_at = now()
		WHERE device_id = $1 AND version = $2`, deviceID, version, until)
	if err != nil {
		return time.Time{}, err
	}
	if command.RowsAffected() != 1 {
		return time.Time{}, ErrVersionConflict
	}
	return until, nil
}

func (s *Store) IOSSnapshots(ctx context.Context) ([]IOSSnapshot, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.device_id, r.version, r.payload, r.foreground_lease_until,
		       COALESCE(d.push_token, ''), d.push_environment
		FROM ios_rule_snapshots r
		JOIN devices d ON d.id = r.device_id
		WHERE d.platform = 'ios' AND d.expires_at > now()`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []IOSSnapshot
	for rows.Next() {
		var item IOSSnapshot
		if err := rows.Scan(&item.DeviceID, &item.Version, &item.Payload, &item.LeaseUntil, &item.PushToken, &item.PushEnvironment); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) UpdateIOSSnapshotState(ctx context.Context, deviceID uuid.UUID, version int64, payload json.RawMessage) (bool, error) {
	command, err := s.pool.Exec(ctx, `
		UPDATE ios_rule_snapshots
		SET payload = $3, updated_at = now()
		WHERE device_id = $1 AND version = $2
		  AND (foreground_lease_until IS NULL OR foreground_lease_until <= now())`, deviceID, version, payload)
	if err != nil {
		return false, err
	}
	return command.RowsAffected() == 1, nil
}

func (s *Store) SetIOSPushToken(ctx context.Context, deviceID uuid.UUID, token, environment string) error {
	command, err := s.pool.Exec(ctx, `
		UPDATE devices SET push_token = $2, push_environment = $3, last_seen_at = now()
		WHERE id = $1 AND platform = 'ios'`, deviceID, token, environment)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrUnauthorized
	}
	return nil
}

func (s *Store) CreateIOSEventIfBackground(
	ctx context.Context, deviceID uuid.UUID, version int64, eventID string, payload json.RawMessage,
) (bool, error) {
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO ios_events (id, device_id, payload)
		SELECT $3, $1, $4
		FROM ios_rule_snapshots
		WHERE device_id = $1 AND version = $2
		  AND (foreground_lease_until IS NULL OR foreground_lease_until <= now())
		ON CONFLICT (id) DO NOTHING`, deviceID, version, eventID, payload)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func (s *Store) PendingIOSEvents(ctx context.Context, deviceID uuid.UUID, afterID string, limit int) ([]IOSEvent, error) {
	rows, err := s.pool.Query(ctx, `
		WITH cursor AS (
			SELECT created_at, id FROM ios_events WHERE device_id = $1 AND id = $2
		)
		SELECT e.id, e.payload, e.created_at
		FROM ios_events e
		WHERE e.device_id = $1 AND e.acknowledged_at IS NULL
		  AND ($2 = '' OR NOT EXISTS (SELECT 1 FROM cursor)
		       OR (e.created_at, e.id) > (SELECT created_at, id FROM cursor))
		ORDER BY e.created_at, e.id
		LIMIT $3`, deviceID, afterID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []IOSEvent
	for rows.Next() {
		var item IOSEvent
		if err := rows.Scan(&item.ID, &item.Payload, &item.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) AcknowledgeIOSEvent(ctx context.Context, deviceID uuid.UUID, eventID string) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE ios_events SET acknowledged_at = now()
		WHERE id = $1 AND device_id = $2`, eventID, deviceID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) IOSEventsDueForDelivery(ctx context.Context, limit int) ([]IOSEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.payload, e.created_at, e.delivered_at, e.delivery_attempts,
		       COALESCE(d.push_token, ''), d.push_environment
		FROM ios_events e
		JOIN devices d ON d.id = e.device_id
		WHERE e.acknowledged_at IS NULL AND e.delivered_at IS NULL
		  AND COALESCE(d.push_token, '') <> ''
		  AND (e.last_delivery_attempt_at IS NULL OR e.last_delivery_attempt_at <= now() - interval '30 seconds')
		  AND e.created_at >= now() - interval '30 days'
		ORDER BY e.created_at
		LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []IOSEvent
	for rows.Next() {
		var item IOSEvent
		if err := rows.Scan(&item.ID, &item.Payload, &item.CreatedAt, &item.DeliveredAt, &item.Attempts, &item.PushToken, &item.Environment); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) MarkIOSEventDeliveryAttempt(ctx context.Context, eventID string, delivered bool) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE ios_events
		SET delivery_attempts = delivery_attempts + 1,
		    last_delivery_attempt_at = now(),
		    delivered_at = CASE WHEN $2 THEN now() ELSE delivered_at END
		WHERE id = $1`, eventID, delivered)
	return err
}

func (s *Store) SaveIOSLiveActivity(ctx context.Context, activity IOSLiveActivity) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO ios_live_activities (activity_id, device_id, push_token, symbol, environment, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (activity_id) DO UPDATE SET
		push_token = EXCLUDED.push_token, symbol = EXCLUDED.symbol,
		environment = EXCLUDED.environment, expires_at = EXCLUDED.expires_at`,
		activity.ActivityID, activity.DeviceID, activity.PushToken, activity.Symbol, activity.Environment, activity.ExpiresAt)
	return err
}

func (s *Store) DeleteIOSLiveActivity(ctx context.Context, deviceID uuid.UUID, activityID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM ios_live_activities WHERE activity_id = $1 AND device_id = $2`, activityID, deviceID)
	return err
}

func (s *Store) IOSLiveActivitiesDue(ctx context.Context, symbol string) ([]IOSLiveActivity, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT activity_id, device_id, push_token, symbol, environment, expires_at
		FROM ios_live_activities
		WHERE symbol = $1 AND expires_at > now()
		  AND (last_update_at IS NULL OR last_update_at <= now() - interval '15 seconds')`, symbol)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []IOSLiveActivity
	for rows.Next() {
		var item IOSLiveActivity
		if err := rows.Scan(&item.ActivityID, &item.DeviceID, &item.PushToken, &item.Symbol, &item.Environment, &item.ExpiresAt); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Store) MarkIOSLiveActivityUpdated(ctx context.Context, activityID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE ios_live_activities SET last_update_at = now() WHERE activity_id = $1`, activityID)
	return err
}

func (s *Store) CleanupIOSData(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM ios_events WHERE created_at < now() - interval '30 days';
		DELETE FROM ios_live_activities WHERE expires_at <= now();`)
	return err
}

func IsNotFound(err error) bool { return errors.Is(err, pgx.ErrNoRows) }
