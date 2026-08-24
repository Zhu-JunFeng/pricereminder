package binance

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Contract struct {
	Symbol     string `json:"symbol"`
	BaseAsset  string `json:"baseAsset"`
	QuoteAsset string `json:"quoteAsset"`
	TickSize   string `json:"tickSize"`
}

type Catalog struct {
	mu        sync.RWMutex
	contracts []Contract
	bySymbol  map[string]Contract
}

func NewCatalog() *Catalog { return &Catalog{bySymbol: make(map[string]Contract)} }

func (c *Catalog) Replace(contracts []Contract) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.contracts = append([]Contract(nil), contracts...)
	c.bySymbol = make(map[string]Contract, len(contracts))
	for _, contract := range contracts {
		c.bySymbol[contract.Symbol] = contract
	}
}

func (c *Catalog) All() []Contract {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return append([]Contract(nil), c.contracts...)
}

func (c *Catalog) Contains(symbol string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.bySymbol[symbol]
	return ok
}

type exchangeInfo struct {
	Symbols []exchangeSymbol `json:"symbols"`
}
type exchangeSymbol struct {
	Symbol, Status, ContractType, BaseAsset, QuoteAsset string
	Filters                                             []struct{ FilterType, TickSize string } `json:"filters"`
}

func FetchContracts(ctx context.Context, client *http.Client, restURL string) ([]Contract, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(restURL, "/")+"/fapi/v1/exchangeInfo", nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("binance exchangeInfo returned %s", response.Status)
	}
	var payload exchangeInfo
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, err
	}
	result := make([]Contract, 0, len(payload.Symbols))
	for _, item := range payload.Symbols {
		if item.Status != "TRADING" || item.ContractType != "PERPETUAL" || (item.QuoteAsset != "USDT" && item.QuoteAsset != "USDC") {
			continue
		}
		var tickSize string
		for _, filter := range item.Filters {
			if filter.FilterType == "PRICE_FILTER" {
				tickSize = filter.TickSize
				break
			}
		}
		if tickSize == "" {
			continue
		}
		result = append(result, Contract{Symbol: item.Symbol, BaseAsset: item.BaseAsset, QuoteAsset: item.QuoteAsset, TickSize: tickSize})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Symbol < result[j].Symbol })
	return result, nil
}

func RefreshCatalog(ctx context.Context, client *http.Client, restURL string, catalog *Catalog, logger func(string, ...any)) error {
	contracts, err := FetchContracts(ctx, client, restURL)
	if err != nil {
		return err
	}
	catalog.Replace(contracts)
	logger("contract catalog refreshed", "count", len(contracts))
	return nil
}

func RunCatalogRefresh(ctx context.Context, client *http.Client, restURL string, catalog *Catalog, logger func(string, ...any)) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := RefreshCatalog(ctx, client, restURL, catalog, logger); err != nil {
				logger("contract catalog refresh failed", "error", err)
			}
		}
	}
}
