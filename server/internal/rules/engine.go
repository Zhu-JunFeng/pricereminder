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
type Kind string
type TargetDirection string

const (
	Rise             Direction       = "rise"
	Fall             Direction       = "fall"
	Percentage       Kind            = "percentage"
	MarketPercentage Kind            = "market_percentage"
	Target           Kind            = "target"
	Above            TargetDirection = "above"
	Below            TargetDirection = "below"
)

type Rule struct {
	ID              string          `json:"id"`
	Symbol          string          `json:"symbol"`
	Kind            Kind            `json:"kind"`
	WindowMinutes   int             `json:"windowMinutes"`
	ThresholdPct    decimal.Decimal `json:"-"`
	ThresholdText   string          `json:"thresholdPct"`
	RiseTriggered   bool            `json:"riseTriggered"`
	FallTriggered   bool            `json:"fallTriggered"`
	Enabled         bool            `json:"enabled"`
	TargetDirection TargetDirection `json:"targetDirection,omitempty"`
	TargetPrice     decimal.Decimal `json:"-"`
	TargetPriceText string          `json:"targetPrice,omitempty"`
	TargetTriggered bool            `json:"targetTriggered,omitempty"`
}

type Trigger struct {
	RuleID        string    `json:"ruleId"`
	Symbol        string    `json:"symbol"`
	Kind          Kind      `json:"kind"`
	Direction     Direction `json:"direction"`
	ChangePct     string    `json:"changePct,omitempty"`
	ThresholdPct  string    `json:"thresholdPct,omitempty"`
	WindowMinutes int       `json:"windowMinutes,omitempty"`
	TargetPrice   string    `json:"targetPrice,omitempty"`
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
	return Rule{ID: id, Symbol: symbol, Kind: Percentage, WindowMinutes: windowMinutes, ThresholdPct: threshold, ThresholdText: thresholdText, Enabled: true}, nil
}

func NewTargetRule(id, symbol string, direction TargetDirection, targetPriceText string) (Rule, error) {
	if direction != Above && direction != Below {
		return Rule{}, errors.New("targetDirection must be above or below")
	}
	targetPrice, err := decimal.NewFromString(targetPriceText)
	if err != nil || !targetPrice.IsPositive() {
		return Rule{}, errors.New("targetPrice must be greater than zero")
	}
	return Rule{ID: id, Symbol: symbol, Kind: Target, TargetDirection: direction,
		TargetPrice: targetPrice, TargetPriceText: targetPrice.String(), Enabled: true}, nil
}

func Evaluate(rule *Rule, current domain.PricePoint, buffer *pricebuffer.Buffer) ([]Trigger, error) {
	if !rule.Enabled || current.Symbol != rule.Symbol {
		return nil, nil
	}
	if rule.Kind == Target {
		return evaluateTarget(rule, current), nil
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

func evaluateTarget(rule *Rule, current domain.PricePoint) []Trigger {
	reached := (rule.TargetDirection == Above && current.Price.GreaterThanOrEqual(rule.TargetPrice)) ||
		(rule.TargetDirection == Below && current.Price.LessThanOrEqual(rule.TargetPrice))
	if rule.TargetTriggered && !reached {
		rule.TargetTriggered = false
	}
	if rule.TargetTriggered || !reached {
		return nil
	}
	rule.TargetTriggered = true
	direction := Rise
	if rule.TargetDirection == Below {
		direction = Fall
	}
	return []Trigger{{RuleID: rule.ID, Symbol: rule.Symbol, Kind: Target, Direction: direction,
		TargetPrice: rule.TargetPriceText, Price: current.PriceText, EventTime: current.EventTime}}
}

func InitializeState(rule *Rule, current domain.PricePoint, buffer *pricebuffer.Buffer) bool {
	if rule.Kind == Target {
		rule.TargetTriggered = (rule.TargetDirection == Above && current.Price.GreaterThanOrEqual(rule.TargetPrice)) ||
			(rule.TargetDirection == Below && current.Price.LessThanOrEqual(rule.TargetPrice))
		return true
	}
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
		RuleID: rule.ID, Symbol: rule.Symbol, Kind: Percentage, Direction: direction,
		ChangePct: change.String(), ThresholdPct: rule.ThresholdText,
		WindowMinutes: rule.WindowMinutes, Price: current.PriceText,
		BaselinePrice: baseline.PriceText, EventTime: current.EventTime,
	}
}
