package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

const deviceLifetime = 30 * 24 * time.Hour

var ErrUnauthorized = errors.New("unauthorized device")
var ErrVersionConflict = errors.New("rule snapshot version conflict")

type Store struct {
	pool   *pgxpool.Pool
	pepper []byte
}

type Device struct {
	ID          uuid.UUID `json:"deviceId"`
	Platform    string    `json:"platform"`
	DisplayName string    `json:"displayName"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

type Registration struct {
	Device
	Token string `json:"token"`
}

func Open(ctx context.Context, databaseURL, pepper string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool, pepper: []byte(pepper)}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	entries, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(entries)
	for _, name := range entries {
		data, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}
		if _, err := s.pool.Exec(ctx, string(data)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}

func (s *Store) Register(ctx context.Context, platform, displayName string) (Registration, error) {
	if platform != "ios" && platform != "android" && platform != "macos" {
		return Registration{}, errors.New("platform must be ios, android, or macos")
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Registration{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	id := uuid.New()
	expiresAt := time.Now().UTC().Add(deviceLifetime)
	_, err := s.pool.Exec(ctx,
		`INSERT INTO devices (id, token_hash, platform, display_name, expires_at) VALUES ($1, $2, $3, $4, $5)`,
		id, s.tokenHash(token), platform, strings.TrimSpace(displayName), expiresAt)
	if err != nil {
		return Registration{}, err
	}
	return Registration{Device: Device{ID: id, Platform: platform, DisplayName: strings.TrimSpace(displayName), ExpiresAt: expiresAt}, Token: token}, nil
}

func (s *Store) Authenticate(ctx context.Context, token string) (Device, error) {
	if token == "" {
		return Device{}, ErrUnauthorized
	}
	var device Device
	var storedHash []byte
	err := s.pool.QueryRow(ctx,
		`SELECT id, token_hash, platform, display_name, expires_at FROM devices WHERE token_hash = $1 AND expires_at > now()`,
		s.tokenHash(token)).Scan(&device.ID, &storedHash, &device.Platform, &device.DisplayName, &device.ExpiresAt)
	if err != nil || subtle.ConstantTimeCompare(storedHash, s.tokenHash(token)) != 1 {
		return Device{}, ErrUnauthorized
	}
	return device, nil
}

func (s *Store) Refresh(ctx context.Context, deviceID uuid.UUID) (time.Time, error) {
	expiresAt := time.Now().UTC().Add(deviceLifetime)
	command, err := s.pool.Exec(ctx, `UPDATE devices SET last_seen_at = now(), expires_at = $2 WHERE id = $1`, deviceID, expiresAt)
	if err != nil || command.RowsAffected() != 1 {
		return time.Time{}, fmt.Errorf("refresh device: %w", err)
	}
	return expiresAt, nil
}

func (s *Store) Touch(ctx context.Context, deviceID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE devices SET last_seen_at = now(), expires_at = now() + interval '30 days' WHERE id = $1`, deviceID)
	return err
}

func (s *Store) SetSubscriptions(ctx context.Context, deviceID uuid.UUID, symbols []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM subscriptions WHERE device_id = $1`, deviceID); err != nil {
		return err
	}
	for _, symbol := range symbols {
		if _, err := tx.Exec(ctx, `INSERT INTO subscriptions (device_id, symbol) VALUES ($1, $2)`, deviceID, symbol); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) Subscriptions(ctx context.Context, deviceID uuid.UUID) ([]string, error) {
	rows, err := s.pool.Query(ctx, `SELECT symbol FROM subscriptions WHERE device_id = $1 ORDER BY symbol`, deviceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, err
		}
		result = append(result, symbol)
	}
	return result, rows.Err()
}

func (s *Store) ActiveSymbols(ctx context.Context) ([]string, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT subscriptions.symbol
		FROM subscriptions
		JOIN devices ON devices.id = subscriptions.device_id
		WHERE devices.expires_at > now()
		ORDER BY subscriptions.symbol`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var symbol string
		if err := rows.Scan(&symbol); err != nil {
			return nil, err
		}
		result = append(result, symbol)
	}
	return result, rows.Err()
}

func (s *Store) tokenHash(token string) []byte {
	hash := sha256.Sum256(append(append([]byte(nil), s.pepper...), []byte(token)...))
	return hash[:]
}
