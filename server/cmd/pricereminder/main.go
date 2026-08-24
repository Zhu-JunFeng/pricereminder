package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"pricereminder/server/internal/api"
	"pricereminder/server/internal/apns"
	"pricereminder/server/internal/binance"
	"pricereminder/server/internal/config"
	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/iosmonitor"
	"pricereminder/server/internal/pricebuffer"
	"pricereminder/server/internal/store"
	"pricereminder/server/internal/stream"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	configuration, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dataStore, err := store.Open(ctx, configuration.DatabaseURL, configuration.TokenPepper)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer dataStore.Close()
	if err := dataStore.Migrate(ctx); err != nil {
		logger.Error("database migration failed", "error", err)
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 20 * time.Second}
	catalog := binance.NewCatalog()
	logFields := func(message string, fields ...any) { logger.Info(message, fields...) }
	if err := binance.RefreshCatalog(ctx, httpClient, configuration.BinanceRESTURL, catalog, logFields); err != nil {
		logger.Error("binance contract catalog unavailable", "error", err)
		os.Exit(1)
	}
	go binance.RunCatalogRefresh(ctx, httpClient, configuration.BinanceRESTURL, catalog, logFields)

	buffer := pricebuffer.New()
	hub := stream.NewHub(buffer)
	pushClient, err := apns.New(configuration.APNsKeyID, configuration.APNsTeamID, configuration.APNsPrivateKey, configuration.APNsBundleID)
	if err != nil {
		logger.Error("invalid APNs configuration", "error", err)
		os.Exit(1)
	}
	iosWorker := iosmonitor.New(dataStore, buffer, pushClient, logger)
	if err := iosWorker.Load(ctx); err != nil {
		logger.Error("load iOS background rules", "error", err)
		os.Exit(1)
	}
	go iosWorker.Run(ctx)
	go binance.RunPriceStream(ctx, configuration.BinanceWSURL, dataStore, catalog, priceFanout{hub: hub, iosWorker: iosWorker}, logger)

	server := &http.Server{
		Addr:              configuration.ListenAddr,
		Handler:           api.New(dataStore, catalog, hub, iosWorker, configuration.AllowedOrigin, logger).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       75 * time.Second,
	}
	go func() {
		logger.Info("server listening", "address", configuration.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
}

type priceFanout struct {
	hub       *stream.Hub
	iosWorker *iosmonitor.Worker
}

func (p priceFanout) Publish(point domain.PricePoint) {
	p.hub.Publish(point)
	p.iosWorker.Publish(point)
}
