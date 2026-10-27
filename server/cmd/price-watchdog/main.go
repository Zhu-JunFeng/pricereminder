package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"pricereminder/server/internal/watchdog"
)

func main() { os.Exit(run()) }
func run() int {
	check := flag.Bool("check-only", false, "check without selecting nodes or writing cooldown state")
	stateDir := flag.String("state-dir", "/var/lib/pricereminder-watchdog", "state and lock directory")
	health := flag.String("health-url", "http://127.0.0.1:18443/healthz", "relay health URL")
	controller := flag.String("controller", "http://127.0.0.1:9090", "Mihomo controller")
	proxy := flag.String("proxy", "http://127.0.0.1:7890", "Mihomo HTTP proxy")
	flag.Parse()
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := os.MkdirAll(*stateDir, 0700); err != nil {
		log.Error("state directory", "error", err)
		return 1
	}
	lock, err := watchdog.Lock(filepath.Join(*stateDir, "run.lock"))
	if err != nil {
		log.Error("inspection lock unavailable", "error", err)
		return 1
	}
	defer lock.Close()
	statePath := filepath.Join(*stateDir, "state.json")
	state, err := watchdog.LoadState(statePath)
	if err != nil {
		log.Error("load cooldown state", "error", err)
		return 1
	}
	backend, err := watchdog.NewLive(watchdog.Config{HealthURL: *health, Controller: *controller, ControllerSecret: os.Getenv("MIHOMO_CONTROLLER_SECRET"), Proxy: *proxy, Provider: "pqjc", Group: "节点选择", RelayService: "pricereminder.service", ProxyService: "mihomo.service", RESTURL: "https://fapi.binance.com/fapi/v1/time", WSURL: "wss://fstream.binance.com/ws/btcusdt@trade"}, log)
	if err != nil {
		log.Error("configuration", "error", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 7*time.Minute+30*time.Second)
	defer cancel()
	r := watchdog.Runner{Backend: backend, State: state, Log: log}
	err = r.Run(ctx, *check)
	if !*check {
		if e := watchdog.SaveState(statePath, state); e != nil {
			log.Error("save cooldown state", "error", e)
			return 1
		}
	}
	if err != nil {
		log.Error("inspection failed", "error", err)
		return 1
	}
	return 0
}
