// Package watchdog checks the relay and repairs only proxy-related failures.
package watchdog

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

type Health struct {
	Status string `json:"status"`
	Latest int64  `json:"latestEventTime"`
	Active *int   `json:"activeSubscriptions"`
}

func (h Health) Stale(now time.Time) bool {
	return h.Active != nil && *h.Active > 0 && now.UnixMilli()-h.Latest > 60000
}

type Node struct {
	Name  string
	Delay int
}
type Failure struct {
	Count int       `json:"count"`
	Until time.Time `json:"until"`
}
type State struct {
	Failures map[string]Failure `json:"failures"`
}

type Backend interface {
	Preflight(context.Context) error
	Health(context.Context) (Health, error)
	REST(context.Context) error
	Current(context.Context) (string, error)
	Candidates(context.Context) ([]Node, error)
	Select(context.Context, string) error
	Probe(context.Context) error
	// Reconnect closes only sockets owned by the configured relay service.
	Reconnect(context.Context) error
}

type Runner struct {
	Backend Backend
	State   *State
	Log     *slog.Logger
	Now     func() time.Time
	Wait    func(context.Context, time.Duration) error
}

func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func (r *Runner) health(ctx context.Context) (Health, error) {
	h, err := r.Backend.Health(ctx)
	if err == nil && (h.Status != "ok" || h.Active == nil || *h.Active < 0) {
		err = errors.New("invalid health or subscription status unavailable")
	}
	if err == nil {
		r.Log.Info("health checked", "active_subscriptions", *h.Active, "event_age_ms", r.Now().UnixMilli()-h.Latest)
	}
	return h, err
}

func (r *Runner) rest(ctx context.Context) error {
	var err error
	for i := 0; i < 3; i++ {
		err = r.Backend.REST(ctx)
		r.Log.Info("REST checked", "attempt", i+1, "error", err)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if i < 2 {
			if e := r.Wait(ctx, time.Second); e != nil {
				return e
			}
		}
	}
	return err
}

func (r *Runner) Run(ctx context.Context, checkOnly bool) (err error) {
	if r.Now == nil {
		r.Now = time.Now
	}
	if r.Wait == nil {
		r.Wait = Sleep
	}
	if r.State.Failures == nil {
		r.State.Failures = map[string]Failure{}
	}
	started := r.Now()
	defer func() {
		r.Log.Info("inspection finished", "duration_ms", r.Now().Sub(started).Milliseconds(), "error", err)
	}()
	if err = r.Backend.Preflight(ctx); err != nil {
		return fmt.Errorf("preflight: %w", err)
	}
	original, err := r.Backend.Current(ctx)
	if err != nil {
		return err
	}
	h, err := r.health(ctx)
	if err != nil {
		return err
	}
	stale := h.Stale(r.Now())
	if stale {
		if err = r.Wait(ctx, 10*time.Second); err != nil {
			return err
		}
		h, err = r.health(ctx)
		if err != nil {
			return err
		}
		stale = h.Stale(r.Now())
	}
	restErr := r.rest(ctx)
	if !stale && restErr == nil {
		delete(r.State.Failures, original)
		r.Log.Info("healthy", "node", original)
		return nil
	}
	r.Log.Warn("repair needed", "node", original, "stale", stale, "rest_error", restErr)
	if checkOnly {
		return errors.New("check-only: repair needed; no changes made")
	}
	nodes, err := r.Backend.Candidates(ctx)
	if err != nil {
		return err
	}
	eligible := make([]Node, 0, len(nodes))
	for _, n := range nodes {
		if n.Name != original && !r.State.Failures[n.Name].Until.After(r.Now()) {
			eligible = append(eligible, n)
		}
	}
	if len(eligible) == 0 {
		for _, n := range nodes {
			if n.Name != original {
				eligible = append(eligible, n)
			}
		}
	}
	if len(eligible) > 5 {
		eligible = eligible[:5]
	}
	if len(eligible) == 0 {
		return errors.New("no alternative provider nodes")
	}
	changed := false
	reconnected := false
	success := false
	// Reserve a separate bounded cleanup context even when the run times out.
	defer func() {
		if !changed || success {
			return
		}
		cleanup, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		e := r.Backend.Select(cleanup, original)
		if e == nil && reconnected {
			e = r.Backend.Reconnect(cleanup)
		}
		r.Log.Warn("rolled back", "node", original, "error", e)
		if e != nil {
			err = errors.Join(err, fmt.Errorf("rollback: %w", e))
		}
	}()
	for _, n := range eligible {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e := r.Backend.Preflight(ctx); e != nil {
			return e
		}
		if _, e := r.health(ctx); e != nil {
			return e
		}
		r.Log.Info("trying node", "from", original, "to", n.Name, "probe_delay_ms", n.Delay)
		changed = true // A timed-out controller request can still have applied.
		e := r.Backend.Select(ctx, n.Name)
		if e == nil {
			e = r.rest(ctx)
		}
		if e == nil {
			e = r.Backend.Probe(ctx)
		}
		if e == nil {
			reconnected = true
			e = r.Backend.Reconnect(ctx)
		}
		if e == nil {
			e = r.recovered(ctx)
		}
		if e == nil {
			success = true
			delete(r.State.Failures, n.Name)
			r.Log.Info("recovered", "from", original, "to", n.Name, "duration_ms", r.Now().Sub(started).Milliseconds())
			return nil
		}
		f := r.State.Failures[n.Name]
		f.Count++
		if f.Count >= 2 {
			f.Until = r.Now().Add(30 * time.Minute)
		}
		r.State.Failures[n.Name] = f
		r.Log.Warn("candidate failed", "node", n.Name, "error", e, "cooldown_until", f.Until)
	}
	return errors.New("all attempted nodes failed")
}

func (r *Runner) recovered(ctx context.Context) error {
	deadline := r.Now().Add(60 * time.Second)
	var previous int64
	ready := false
	for !r.Now().After(deadline) {
		h, err := r.health(ctx)
		if err != nil {
			return err
		}
		if *h.Active == 0 {
			ready = true
			break
		}
		if !h.Stale(r.Now()) && previous > 0 && h.Latest > previous {
			ready = true
			break
		}
		previous = h.Latest
		if err = r.Wait(ctx, 5*time.Second); err != nil {
			return err
		}
	}
	if !ready {
		return errors.New("relay did not resume within 60 seconds")
	}
	observationStart := previous
	activeAtEnd := 0
	for i := 0; i < 6; i++ {
		if err := r.Wait(ctx, 5*time.Second); err != nil {
			return err
		}
		h, err := r.health(ctx)
		if err != nil {
			return err
		}
		if h.Stale(r.Now()) {
			return errors.New("relay stalled during observation")
		}
		activeAtEnd = *h.Active
		previous = h.Latest
	}
	if activeAtEnd > 0 && previous <= observationStart {
		return errors.New("relay timestamp stopped advancing during observation")
	}
	return r.rest(ctx)
}
