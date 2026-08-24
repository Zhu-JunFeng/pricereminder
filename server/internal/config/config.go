package config

import (
	"errors"
	"os"
)

type Config struct {
	ListenAddr     string
	DatabaseURL    string
	BinanceRESTURL string
	BinanceWSURL   string
	TokenPepper    string
	AllowedOrigin  string
	APNsKeyID      string
	APNsTeamID     string
	APNsPrivateKey string
	APNsBundleID   string
}

func Load() (Config, error) {
	result := Config{
		ListenAddr:     env("LISTEN_ADDR", "127.0.0.1:18443"),
		DatabaseURL:    env("DATABASE_URL", "postgres://pricereminder:pricereminder@127.0.0.1:55432/pricereminder?sslmode=disable"),
		BinanceRESTURL: env("BINANCE_REST_URL", "https://fapi.binance.com"),
		BinanceWSURL:   env("BINANCE_WS_URL", "wss://fstream.binance.com"),
		TokenPepper:    os.Getenv("TOKEN_PEPPER"),
		AllowedOrigin:  os.Getenv("ALLOWED_ORIGIN"),
		APNsKeyID:      os.Getenv("APNS_KEY_ID"),
		APNsTeamID:     os.Getenv("APNS_TEAM_ID"),
		APNsPrivateKey: os.Getenv("APNS_PRIVATE_KEY"),
		APNsBundleID:   env("APNS_BUNDLE_ID", "world.zcn.pricereminder"),
	}
	if len(result.TokenPepper) < 32 {
		return Config{}, errors.New("TOKEN_PEPPER must contain at least 32 bytes")
	}
	return result, nil
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
