package rules

import (
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"pricereminder/server/internal/domain"
	"pricereminder/server/internal/pricebuffer"
)

const StaleMillis int64 = 30 * 1000

type Direction string

const (
	Rise Direction = "rise"
	Fall Direction = "fall"
)

type Rule struct {
	ID            string          `json:"id"`
	Symbol        string          `json:"symbol"`
	WindowMinutes int             `json:"windowMinutes"`
	ThresholdPct  decimal.Decimal `json:"-"`
	ThresholdText string          `json:"thresholdPct"`
	RiseTriggered bool            `json:"riseTriggered"`
	FallTriggered bool            `json:"fallTriggered"`
	Enabled       bool            `json:"enabled"`
}

type Trigger struct {
	RuleID        string    `json:"ruleId"`
	Symbol        string    `json:"symbol"`
	Direction     Direction `json:"direction"`
	ChangePct     string    `json:"changePct"`
	ThresholdPct  string    `json:"thresholdPct"`
	WindowMinutes int       `json:"windowMinutes"`
	Price         string    `json:"price"`
	BaselinePrice string    `json:"baselinePrice"`
	EventTime     int64     `json:"eventTime"`
}

func NewRule(id, symbol string, windowMinutes int, thresholdText string) (Rule, error) {
	if windowMinutes < 1 || windowMinutes > 60 {
		return Rule{}, errors.New("windowMinutes must be between 1 and 60")
	}
	threshold, err := decimal.NewFromString(thresholdText)
	if err != nil || threshold.LessThan(decimal.NewFromFloat(0.1)) || threshold.GreaterThan(decimal.NewFromInt(100)) {
		return Rule{}, errors.New("thresholdPct must be between 0.1 and 100")
	}
	return Rule{ID: id, Symbol: symbol, WindowMinutes: windowMinutes, ThresholdPct: threshold, ThresholdText: thresholdText, Enabled: true}, nil
}

func Evaluate(rule *Rule, current domain.PricePoint, buffer *pricebuffer.Buffer) ([]Trigger, error) {
	if !rule.Enabled || current.Symbol != rule.Symbol {
		return nil, nil
	}
	cutoff := current.EventTime - int64(rule.WindowMinutes)*60*1000
	baseline, ok := buffer.AtOrBefore(rule.Symbol, cutoff)
	if !ok || cutoff-baseline.EventTime > StaleMillis {
		return nil, nil
	}
	if baseline.Price.IsZero() {
		return nil, fmt.Errorf("baseline price is zero for %s", rule.Symbol)
	}
	change := current.Price.Sub(baseline.Price).Div(baseline.Price).Mul(decimal.NewFromInt(100))
	negativeThreshold := rule.ThresholdPct.Neg()

	if rule.RiseTriggered && change.LessThan(rule.ThresholdPct) {
		rule.RiseTriggered = false
	}
	if rule.FallTriggered && change.GreaterThan(negativeThreshold) {
		rule.FallTriggered = false
	}

	var result []Trigger
	if !rule.RiseTriggered && change.GreaterThanOrEqual(rule.ThresholdPct) {
		rule.RiseTriggered = true
		result = append(result, makeTrigger(*rule, current, baseline, Rise, change))
	}
	if !rule.FallTriggered && change.LessThanOrEqual(negativeThreshold) {
		rule.FallTriggered = true
		result = append(result, makeTrigger(*rule, current, baseline, Fall, change))
	}
	return result, nil
}

func InitializeState(rule *Rule, current domain.PricePoint, buffer *pricebuffer.Buffer) bool {
	cutoff := current.EventTime - int64(rule.WindowMinutes)*60*1000
	baseline, ok := buffer.AtOrBefore(rule.Symbol, cutoff)
	if !ok || cutoff-baseline.EventTime > StaleMillis || baseline.Price.IsZero() {
		return false
	}
	change := current.Price.Sub(baseline.Price).Div(baseline.Price).Mul(decimal.NewFromInt(100))
	rule.RiseTriggered = change.GreaterThanOrEqual(rule.ThresholdPct)
	rule.FallTriggered = change.LessThanOrEqual(rule.ThresholdPct.Neg())
	return true
}

func makeTrigger(rule Rule, current, baseline domain.PricePoint, direction Direction, change decimal.Decimal) Trigger {
	return Trigger{
		RuleID: rule.ID, Symbol: rule.Symbol, Direction: direction,
		ChangePct: change.String(), ThresholdPct: rule.ThresholdText,
		WindowMinutes: rule.WindowMinutes, Price: current.PriceText,
		BaselinePrice: baseline.PriceText, EventTime: current.EventTime,
	}
}
