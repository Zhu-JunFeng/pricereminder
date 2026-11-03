// gold-probe samples without registering a device, publishing prices or sending alerts.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"pricereminder/server/internal/gold"
)

func main() {
	duration := flag.Duration("duration", 30*time.Minute, "sampling duration")
	interval := flag.Duration("interval", 5*time.Second, "poll interval (minimum 5 seconds)")
	flag.Parse()
	if *interval < 5*time.Second || *duration <= 0 {
		fmt.Fprintln(os.Stderr, "invalid duration or interval")
		os.Exit(2)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, *duration)
	defer timeout()
	client := &http.Client{Timeout: 8 * time.Second}
	encoder := json.NewEncoder(os.Stdout)
	var previous int64
	for {
		started := time.Now()
		quote, err := gold.Fetch(ctx, client)
		if ctx.Err() != nil {
			return
		}
		entry := map[string]any{"receivedAt": time.Now().UTC().Format(time.RFC3339Nano), "requestMs": time.Since(started).Milliseconds()}
		if err != nil {
			entry["error"] = err.Error()
		} else {
			entry["quote"] = quote
			entry["sourceAgeMs"] = quote.ReceivedAt - quote.EventTime
			entry["sourceTimeAdvanced"] = quote.EventTime > previous
			entry["sourceTimeRegressed"] = previous > 0 && quote.EventTime < previous
			if quote.EventTime > previous {
				previous = quote.EventTime
			}
		}
		if err := encoder.Encode(entry); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		timer := time.NewTimer(*interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
