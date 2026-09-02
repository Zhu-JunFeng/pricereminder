package rules

import (
	"errors"
	"sort"
	"sync"

	"github.com/shopspring/decimal"

	"pricereminder/server/internal/domain"
)

type MarketRule struct {
	ID            string          `json:"id"`
	WindowMinutes int             `json:"windowMinutes"`
	ThresholdPct  decimal.Decimal `json:"-"`
	ThresholdText string          `json:"thresholdText"`
	Enabled       bool            `json:"isEnabled"`
}

func NewMarketRule(id string, windowMinutes int, thresholdText string) (MarketRule, error) {
	threshold, err := decimal.NewFromString(thresholdText)
	if err != nil || windowMinutes < 1 || windowMinutes > 60 || threshold.LessThan(decimal.RequireFromString("0.1")) || threshold.GreaterThan(decimal.NewFromInt(100)) {
		return MarketRule{}, errors.New("全市场规则时间必须为 1 到 60 分钟，阈值必须为 0.1% 到 100%")
	}
	return MarketRule{ID: id, WindowMinutes: windowMinutes, ThresholdPct: threshold, ThresholdText: threshold.String(), Enabled: true}, nil
}

type marketSample struct {
	eventTime int64
	price     decimal.Decimal
	priceText string
}

type marketWindow struct {
	values []marketSample
	head   int
}

func (w *marketWindow) reset(sample marketSample) { w.values, w.head = []marketSample{sample}, 0 }

func (w *marketWindow) purge(cutoff int64) {
	for w.head < len(w.values) && w.values[w.head].eventTime < cutoff {
		w.head++
	}
	if w.head > 256 && w.head*2 > len(w.values) {
		w.values = append([]marketSample(nil), w.values[w.head:]...)
		w.head = 0
	}
}

func (w *marketWindow) appendMinimum(sample marketSample, cutoff int64) {
	w.purge(cutoff)
	for len(w.values) > w.head && !w.values[len(w.values)-1].price.LessThan(sample.price) {
		w.values = w.values[:len(w.values)-1]
	}
	w.values = append(w.values, sample)
}

func (w *marketWindow) appendMaximum(sample marketSample, cutoff int64) {
	w.purge(cutoff)
	for len(w.values) > w.head && !w.values[len(w.values)-1].price.GreaterThan(sample.price) {
		w.values = w.values[:len(w.values)-1]
	}
	w.values = append(w.values, sample)
}

func (w *marketWindow) extreme() marketSample { return w.values[w.head] }

type marketSymbolState struct {
	lastEventTime int64
	rise          marketWindow
	fall          marketWindow
}

type MarketScanner struct {
	mu     sync.Mutex
	states map[string]map[string]*marketSymbolState
}

func NewMarketScanner() *MarketScanner {
	return &MarketScanner{states: make(map[string]map[string]*marketSymbolState)}
}

func (s *MarketScanner) RetainRules(ids map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.states {
		if _, ok := ids[id]; !ok {
			delete(s.states, id)
		}
	}
}

func (s *MarketScanner) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states = make(map[string]map[string]*marketSymbolState)
}

func (s *MarketScanner) Evaluate(rule MarketRule, current domain.PricePoint) []Trigger {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !rule.Enabled || !current.Price.IsPositive() {
		return nil
	}
	bySymbol := s.states[rule.ID]
	if bySymbol == nil {
		bySymbol = make(map[string]*marketSymbolState)
		s.states[rule.ID] = bySymbol
	}
	sample := marketSample{eventTime: current.EventTime, price: current.Price, priceText: current.PriceText}
	state := bySymbol[current.Symbol]
	if state == nil {
		state = &marketSymbolState{lastEventTime: current.EventTime}
		state.rise.reset(sample)
		state.fall.reset(sample)
		bySymbol[current.Symbol] = state
	}
	if current.EventTime < state.lastEventTime {
		return nil
	}
	cutoff := current.EventTime - int64(rule.WindowMinutes)*60_000
	state.rise.appendMinimum(sample, cutoff)
	state.fall.appendMaximum(sample, cutoff)
	riseBase, fallBase := state.rise.extreme(), state.fall.extreme()
	riseChange := current.Price.Sub(riseBase.price).Div(riseBase.price).Mul(decimal.NewFromInt(100))
	fallChange := current.Price.Sub(fallBase.price).Div(fallBase.price).Mul(decimal.NewFromInt(100))
	result := make([]Trigger, 0, 2)
	if !riseChange.LessThan(rule.ThresholdPct) {
		result = append(result, marketTrigger(rule, current, riseBase, Rise, riseChange))
		state.rise.reset(sample)
	}
	if !fallChange.GreaterThan(rule.ThresholdPct.Neg()) {
		result = append(result, marketTrigger(rule, current, fallBase, Fall, fallChange))
		state.fall.reset(sample)
	}
	state.lastEventTime = current.EventTime
	sort.Slice(result, func(i, j int) bool { return result[i].Direction < result[j].Direction })
	return result
}

func marketTrigger(rule MarketRule, current domain.PricePoint, baseline marketSample, direction Direction, change decimal.Decimal) Trigger {
	return Trigger{
		RuleID: rule.ID, Symbol: current.Symbol, Kind: MarketPercentage, Direction: direction,
		ChangePct: change.String(), ThresholdPct: rule.ThresholdText, WindowMinutes: rule.WindowMinutes,
		Price: current.PriceText, BaselinePrice: baseline.priceText, EventTime: current.EventTime,
	}
}
