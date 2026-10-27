package watchdog

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

type fake struct {
	now                     time.Time
	current                 string
	selected                []string
	nodes                   []Node
	stale                   bool
	active                  int
	restFailures            int
	restCalls               int
	fail                    map[string]bool
	probeFail               map[string]bool
	preflightErr, healthErr error
	reconnects              int
}

func (f *fake) Preflight(context.Context) error { return f.preflightErr }
func (f *fake) Health(context.Context) (Health, error) {
	t := f.now.UnixMilli()
	if f.stale && f.current == "original" {
		t -= 120000
	}
	return Health{Status: "ok", Latest: t, Active: &f.active}, f.healthErr
}
func (f *fake) REST(context.Context) error {
	f.restCalls++
	if f.current == "original" && (f.restFailures < 0 || f.restCalls <= f.restFailures) || f.fail[f.current] {
		return errors.New("REST timeout")
	}
	return nil
}
func (f *fake) Current(context.Context) (string, error)    { return f.current, nil }
func (f *fake) Candidates(context.Context) ([]Node, error) { return f.nodes, nil }
func (f *fake) Select(_ context.Context, n string) error {
	f.current = n
	f.selected = append(f.selected, n)
	return nil
}
func (f *fake) Probe(context.Context) error {
	if f.fail[f.current] || f.probeFail[f.current] {
		return errors.New("probe failed")
	}
	return nil
}
func (f *fake) Reconnect(context.Context) error { f.reconnects++; return nil }
func setup() (*Runner, *fake) {
	f := &fake{now: time.Unix(1793069339, 0), current: "original", active: 1, nodes: []Node{{Name: "a", Delay: 10}, {Name: "b", Delay: 20}}, fail: map[string]bool{}}
	r := &Runner{Backend: f, State: &State{Failures: map[string]Failure{}}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), Now: func() time.Time { return f.now }, Wait: func(ctx context.Context, d time.Duration) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		f.now = f.now.Add(d)
		return nil
	}}
	return r, f
}
func TestHealthyAndZeroSubscriptions(t *testing.T) {
	for _, zero := range []bool{false, true} {
		r, f := setup()
		if zero {
			f.active = 0
			f.stale = true
		}
		if err := r.Run(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		if len(f.selected) != 0 {
			t.Fatal("healthy switched")
		}
	}
}
func TestStaleRepairAndObservation(t *testing.T) {
	r, f := setup()
	f.stale = true
	start := f.now
	if err := r.Run(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if f.current != "a" || f.reconnects != 1 || f.now.Sub(start) < 45*time.Second {
		t.Fatalf("missing reconfirmation/reconnect/observation: %+v", f)
	}
}
func TestRESTTransientAndConsecutive(t *testing.T) {
	for _, failures := range []int{2, 3} {
		r, f := setup()
		f.restFailures = failures
		if err := r.Run(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		if (len(f.selected) > 0) != (failures == 3) {
			t.Fatal(f.selected)
		}
	}
}
func TestCandidateFailureAndRollback(t *testing.T) {
	for _, all := range []bool{false, true} {
		r, f := setup()
		f.stale = true
		f.fail["a"] = true
		f.fail["b"] = all
		err := r.Run(context.Background(), false)
		if all {
			if f.reconnects != 0 {
				t.Fatal("failed REST candidates must preserve original live stream")
			}
			if err == nil || f.current != "original" || !reflect.DeepEqual(f.selected, []string{"a", "b", "original"}) {
				t.Fatalf("rollback: %v %+v", err, f)
			}
		} else if err != nil || f.current != "b" {
			t.Fatalf("second candidate: %v %+v", err, f)
		}
	}
}

func TestWebSocketRequiredBeforeReconnect(t *testing.T) {
	r, f := setup()
	f.stale = true
	f.probeFail = map[string]bool{"a": true}
	if err := r.Run(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if f.current != "b" || f.reconnects != 1 {
		t.Fatal("unverified candidate closed live stream", f.current, f.reconnects)
	}
}

func TestCorruptStateAndDeadline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadState(path); err == nil {
		t.Fatal("corrupt state silently accepted")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := Sleep(ctx, time.Hour); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestCheckOnlyAndInfrastructureErrors(t *testing.T) {
	for _, kind := range []string{"check", "service", "health"} {
		r, f := setup()
		f.stale = true
		if kind == "service" {
			f.preflightErr = errors.New("inactive")
		}
		if kind == "health" {
			f.healthErr = errors.New("503")
		}
		if err := r.Run(context.Background(), kind == "check"); err == nil {
			t.Fatal("expected failure")
		}
		if len(f.selected) > 0 {
			t.Fatal("unexpected mutation")
		}
	}
}
func TestCooldownAndRetryAllCooled(t *testing.T) {
	r, f := setup()
	f.stale = true
	r.State.Failures["a"] = Failure{Count: 2, Until: f.now.Add(30 * time.Minute)}
	if err := r.Run(context.Background(), false); err != nil || f.current != "b" {
		t.Fatal(err, f.current)
	}
	r, f = setup()
	f.stale = true
	for _, n := range f.nodes {
		r.State.Failures[n.Name] = Failure{Count: 2, Until: f.now.Add(30 * time.Minute)}
	}
	if err := r.Run(context.Background(), false); err != nil || f.current != "a" {
		t.Fatal(err, f.current)
	}
	r, f = setup()
	f.stale = true
	f.fail["a"] = true
	r.State.Failures["a"] = Failure{Count: 1}
	if err := r.Run(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if !r.State.Failures["a"].Until.After(f.now) {
		t.Fatal("missing cooldown")
	}
}
func TestTimeoutRollsBack(t *testing.T) {
	r, f := setup()
	f.stale = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	original := r.Wait
	r.Wait = func(ctx context.Context, d time.Duration) error {
		if f.current != "original" {
			cancel()
			return ctx.Err()
		}
		return original(ctx, d)
	}
	if err := r.Run(ctx, false); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if f.current != "original" {
		t.Fatal("no rollback on timeout")
	}
}
func TestAtMostFiveCandidates(t *testing.T) {
	r, f := setup()
	f.stale = true
	f.nodes = nil
	for _, n := range []string{"a", "b", "c", "d", "e", "f"} {
		f.nodes = append(f.nodes, Node{Name: n})
		f.fail[n] = true
	}
	if r.Run(context.Background(), false) == nil {
		t.Fatal("expected failure")
	}
	if len(f.selected) != 6 {
		t.Fatal(f.selected)
	}
}
func TestStateAndLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	s.Failures["node"] = Failure{Count: 2, Until: time.Now().UTC().Truncate(time.Second)}
	if err = SaveState(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadState(path)
	if err != nil || !reflect.DeepEqual(s, loaded) {
		t.Fatal(err, loaded)
	}
	l, err := Lock(filepath.Join(dir, "lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if other, err := Lock(filepath.Join(dir, "lock")); err == nil {
		other.Close()
		t.Fatal("overlapping lock succeeded")
	}
}
func TestOwnedPorts(t *testing.T) {
	input := `ESTAB 0 0 127.0.0.1:35024 127.0.0.1:7890 users:(("relay",pid=123,fd=9))
ESTAB 0 0 127.0.0.1:35025 127.0.0.1:7890 users:(("other",pid=1234,fd=9))`
	if got := ownedPorts(input, "123"); !reflect.DeepEqual(got, map[string]bool{"35024": true}) {
		t.Fatal(got)
	}
}
