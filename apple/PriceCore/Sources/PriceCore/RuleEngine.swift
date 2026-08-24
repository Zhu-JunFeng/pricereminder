import Foundation

public enum RuleEngine {
    public static let staleMilliseconds: Int64 = 30_000

    public static func evaluate(rule: inout AlertRule, current: PricePoint, buffer: PriceBuffer) -> [AlertTrigger] {
        guard rule.isEnabled, rule.symbol == current.symbol else { return [] }
        let cutoff = current.eventTime - Int64(rule.windowMinutes) * 60_000
        guard let baseline = buffer.point(atOrBefore: cutoff, symbol: rule.symbol),
              cutoff - baseline.eventTime <= staleMilliseconds,
              baseline.price != .zero else { return [] }

        let change = (current.price - baseline.price) / baseline.price * 100
        let threshold = rule.threshold

        if rule.riseTriggered, change < threshold { rule.riseTriggered = false }
        if rule.fallTriggered, change > -threshold { rule.fallTriggered = false }

        var triggers: [AlertTrigger] = []
        if !rule.riseTriggered, change >= threshold {
            rule.riseTriggered = true
            triggers.append(trigger(rule: rule, direction: .rise, change: change, current: current, baseline: baseline))
        }
        if !rule.fallTriggered, change <= -threshold {
            rule.fallTriggered = true
            triggers.append(trigger(rule: rule, direction: .fall, change: change, current: current, baseline: baseline))
        }
        return triggers
    }

    @discardableResult
    public static func initialize(rule: inout AlertRule, current: PricePoint, buffer: PriceBuffer) -> Bool {
        let cutoff = current.eventTime - Int64(rule.windowMinutes) * 60_000
        guard let baseline = buffer.point(atOrBefore: cutoff, symbol: rule.symbol),
              cutoff - baseline.eventTime <= staleMilliseconds,
              baseline.price != .zero else { return false }
        let change = (current.price - baseline.price) / baseline.price * 100
        rule.riseTriggered = change >= rule.threshold
        rule.fallTriggered = change <= -rule.threshold
        return true
    }

    private static func trigger(
        rule: AlertRule, direction: TriggerDirection, change: Decimal,
        current: PricePoint, baseline: PricePoint
    ) -> AlertTrigger {
        AlertTrigger(
            ruleID: rule.id, symbol: rule.symbol, direction: direction,
            changePercent: change, thresholdText: rule.thresholdText,
            windowMinutes: rule.windowMinutes, priceText: current.priceText,
            baselinePriceText: baseline.priceText, eventTime: current.eventTime
        )
    }
}
