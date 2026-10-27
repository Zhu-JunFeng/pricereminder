package watchdog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Config struct {
	HealthURL        string
	Controller       string
	ControllerSecret string
	Proxy            string
	Provider         string
	Group            string
	RelayService     string
	ProxyService     string
	RESTURL          string
	WSURL            string
}

type Live struct {
	Config          Config
	Direct, Proxied *http.Client
	Log             *slog.Logger
}

func NewLive(c Config, log *slog.Logger) (*Live, error) {
	p, err := url.Parse(c.Proxy)
	if err != nil {
		return nil, err
	}
	if p.Scheme != "http" || p.Host == "" {
		return nil, errors.New("HTTP proxy URL required")
	}
	d := http.DefaultTransport.(*http.Transport).Clone()
	d.Proxy = nil
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyURL(p)
	return &Live{Config: c, Direct: &http.Client{Transport: d, Timeout: 10 * time.Second}, Proxied: &http.Client{Transport: t, Timeout: 8 * time.Second}, Log: log}, nil
}

func (b *Live) request(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	r, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(b.Config.Controller, "/")+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if b.Config.ControllerSecret != "" {
		r.Header.Set("Authorization", "Bearer "+b.Config.ControllerSecret)
	}
	resp, err := b.Direct.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("controller %s %s: HTTP %d", method, strings.Split(path, "?")[0], resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
	}
	return nil
}

func (b *Live) Preflight(ctx context.Context) error {
	for _, s := range []string{b.Config.RelayService, b.Config.ProxyService} {
		if err := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", s).Run(); err != nil {
			return fmt.Errorf("service %s inactive or check failed: %w", s, err)
		}
	}
	_, err := b.Current(ctx)
	return err
}

func (b *Live) Health(ctx context.Context) (Health, error) {
	var h Health
	r, err := http.NewRequestWithContext(ctx, "GET", b.Config.HealthURL, nil)
	if err != nil {
		return h, err
	}
	resp, err := b.Direct.Do(r)
	if err != nil {
		return h, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return h, fmt.Errorf("health HTTP %d", resp.StatusCode)
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&h)
	return h, err
}

func (b *Live) REST(ctx context.Context) error {
	r, err := http.NewRequestWithContext(ctx, "GET", b.Config.RESTURL, nil)
	if err != nil {
		return err
	}
	resp, err := b.Proxied.Do(r)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Binance REST HTTP %d", resp.StatusCode)
	}
	var v struct {
		ServerTime int64 `json:"serverTime"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&v); err != nil {
		return err
	}
	if d := time.Now().UnixMilli() - v.ServerTime; d > 30000 || d < -30000 {
		return errors.New("REST serverTime missing or not fresh")
	}
	return nil
}

func (b *Live) Current(ctx context.Context) (string, error) {
	var v struct{ Type, Now string }
	err := b.request(ctx, "GET", "/proxies/"+url.PathEscape(b.Config.Group), nil, &v)
	if err == nil && (v.Type != "Selector" || v.Now == "") {
		err = errors.New("configured proxy group is not a manual Selector")
	}
	return v.Now, err
}

func (b *Live) Select(ctx context.Context, n string) error {
	// Old idle HTTP keep-alives must not validate the previous node.
	b.Proxied.CloseIdleConnections()
	if err := b.request(ctx, "PUT", "/proxies/"+url.PathEscape(b.Config.Group), map[string]string{"name": n}, nil); err != nil {
		return err
	}
	actual, err := b.Current(ctx)
	if err != nil {
		return err
	}
	if actual != n {
		return errors.New("node selection did not take effect")
	}
	return nil
}

func realNode(kind string) bool {
	switch strings.ToLower(kind) {
	case "direct", "reject", "rejectdrop", "reject-drop", "pass", "compatible", "selector", "urltest", "fallback", "loadbalance", "relay", "":
		return false
	}
	return true
}

func (b *Live) Candidates(ctx context.Context) ([]Node, error) {
	var v struct{ Proxies []struct{ Name, Type string } }
	if err := b.request(ctx, "GET", "/providers/proxies/"+url.PathEscape(b.Config.Provider), nil, &v); err != nil {
		return nil, err
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var nodes []Node
	sem := make(chan struct{}, 8)
	seen := map[string]bool{}
	for _, p := range v.Proxies {
		if !realNode(p.Type) || seen[p.Name] {
			continue
		}
		seen[p.Name] = true
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return nil, ctx.Err()
		}
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			defer func() { <-sem }()
			var result struct{ Delay int }
			path := "/proxies/" + url.PathEscape(name) + "/delay?" + url.Values{"url": {b.Config.RESTURL}, "timeout": {"5000"}, "expected": {"200"}}.Encode()
			if err := b.request(ctx, "GET", path, nil, &result); err == nil && result.Delay > 0 {
				mu.Lock()
				nodes = append(nodes, Node{Name: name, Delay: result.Delay})
				mu.Unlock()
			}
		}(p.Name)
	}
	wg.Wait()
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].Delay == nodes[j].Delay {
			return nodes[i].Name < nodes[j].Name
		}
		return nodes[i].Delay < nodes[j].Delay
	})
	b.Log.Info("candidate probes finished", "successful", len(nodes), "total", len(seen))
	return nodes, ctx.Err()
}

func (b *Live) Probe(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, b.Config.WSURL, &websocket.DialOptions{HTTPClient: b.Proxied})
	if err != nil {
		return err
	}
	defer conn.CloseNow()
	conn.SetReadLimit(64 << 10)
	var previous int64
	count := 0
	for count < 3 {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		var v struct {
			Kind   string `json:"e"`
			Symbol string `json:"s"`
			Time   int64  `json:"E"`
			Price  string `json:"p"`
		}
		if err = json.Unmarshal(data, &v); err != nil {
			return err
		}
		price, err := strconv.ParseFloat(v.Price, 64)
		age := time.Now().UnixMilli() - v.Time
		if err == nil && price > 0 && v.Kind == "trade" && v.Symbol == "BTCUSDT" && v.Time > previous && age >= -5000 && age < 10000 {
			previous = v.Time
			count++
		}
	}
	b.Log.Info("WebSocket probe passed", "distinct_events", count)
	return nil
}

// Mihomo does not reliably expose process names. Match source ports to ss
// sockets owned by systemd's current MainPID; never delete by hostname alone.
func ownedPorts(output, pid string) map[string]bool {
	ports := map[string]bool{}
	for _, line := range strings.Split(output, "\n") {
		if !strings.Contains(line, "pid="+pid+",") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		_, port, err := net.SplitHostPort(fields[3])
		if err == nil {
			ports[port] = true
		}
	}
	return ports
}

func (b *Live) Reconnect(ctx context.Context) error {
	pidData, err := exec.CommandContext(ctx, "systemctl", "show", "--property=MainPID", "--value", b.Config.RelayService).Output()
	if err != nil {
		return err
	}
	pid := strings.TrimSpace(string(pidData))
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 1 {
		return errors.New("relay MainPID unavailable")
	}
	sockets, err := exec.CommandContext(ctx, "ss", "-Htnp").Output()
	if err != nil {
		return err
	}
	ports := ownedPorts(string(sockets), pid)
	var v struct {
		Connections []struct {
			ID       string
			Metadata struct {
				Host       string
				SourcePort string
				SourceIP   string
			}
		}
	}
	if err = b.request(ctx, "GET", "/connections", nil, &v); err != nil {
		return err
	}
	count := 0
	for _, c := range v.Connections {
		ip := net.ParseIP(c.Metadata.SourceIP)
		if c.Metadata.Host != "fstream.binance.com" || !ports[c.Metadata.SourcePort] || ip == nil || !ip.IsLoopback() {
			continue
		}
		if err = b.request(ctx, "DELETE", "/connections/"+url.PathEscape(c.ID), nil, nil); err != nil {
			return err
		}
		count++
	}
	b.Log.Info("relay reconnect requested", "closed_owned_connections", count)
	return nil
}
