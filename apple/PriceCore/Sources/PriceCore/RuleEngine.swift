import Foundation

public enum RuleEngine {
    public static let staleMilliseconds: Int64 = 30_000

    public static func evaluate(rule: inout AlertRule, current: PricePoint, buffer: PriceBuffer) -> [AlertTrigger] {
        guard rule.isEnabled, rule.symbol == current.symbol else { return [] }
        if rule.kind == .target {
            return evaluateTarget(rule: &rule, current: current)
        }
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
        if rule.kind == .target {
            guard let direction = rule.targetDirection, let target = rule.targetPrice else { return false }
            rule.targetTriggered = reached(direction: direction, current: current.price, target: target)
            return true
        }
        let cutoff = current.eventTime - Int64(rule.windowMinutes) * 60_000
        guard let baseline = buffer.point(atOrBefore: cutoff, symbol: rule.symbol),
              cutoff - baseline.eventTime <= staleMilliseconds,
              baseline.price != .zero else { return false }
        let change = (current.price - baseline.price) / baseline.price * 100
        rule.riseTriggered = change >= rule.threshold
        rule.fallTriggered = change <= -rule.threshold
        return true
    }

    private static func evaluateTarget(rule: inout AlertRule, current: PricePoint) -> [AlertTrigger] {
        guard let direction = rule.targetDirection, let target = rule.targetPrice else { return [] }
        let isReached = reached(direction: direction, current: current.price, target: target)
        if rule.targetTriggered, !isReached { rule.targetTriggered = false }
        guard !rule.targetTriggered, isReached else { return [] }
        rule.targetTriggered = true
        return [AlertTrigger(
            ruleID: rule.id, symbol: rule.symbol, kind: .target,
            direction: direction == .above ? .rise : .fall, changePercent: nil,
            thresholdText: "", windowMinutes: 0, targetPriceText: rule.targetPriceText,
            priceText: current.priceText, baselinePriceText: "", eventTime: current.eventTime
        )]
    }

    private static func reached(direction: TargetDirection, current: Decimal, target: Decimal) -> Bool {
        direction == .above ? current >= target : current <= target
    }

    private static func trigger(
        rule: AlertRule, direction: TriggerDirection, change: Decimal,
        current: PricePoint, baseline: PricePoint
    ) -> AlertTrigger {
        AlertTrigger(
            ruleID: rule.id, symbol: rule.symbol, kind: .percentage, direction: direction,
            changePercent: change, thresholdText: rule.thresholdText,
            windowMinutes: rule.windowMinutes, targetPriceText: nil, priceText: current.priceText,
            baselinePriceText: baseline.priceText, eventTime: current.eventTime
        )
    }
}
