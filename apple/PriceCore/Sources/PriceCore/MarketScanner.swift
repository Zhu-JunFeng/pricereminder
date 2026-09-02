import Foundation

public struct MarketScanner: Sendable {
    private struct Sample: Sendable {
        let eventTime: Int64
        let price: Decimal
        let priceText: String
    }

    private struct DirectionWindow: Sendable {
        var samples: [Sample] = []
        var head = 0

        mutating func reset(with sample: Sample) {
            samples = [sample]
            head = 0
        }

        mutating func appendMinimum(_ sample: Sample, cutoff: Int64) {
            purge(before: cutoff)
            while samples.count > head, let last = samples.last, last.price >= sample.price {
                samples.removeLast()
            }
            samples.append(sample)
            compactIfNeeded()
        }

        mutating func appendMaximum(_ sample: Sample, cutoff: Int64) {
            purge(before: cutoff)
            while samples.count > head, let last = samples.last, last.price <= sample.price {
                samples.removeLast()
            }
            samples.append(sample)
            compactIfNeeded()
        }

        var extreme: Sample? { head < samples.count ? samples[head] : nil }

        private mutating func purge(before cutoff: Int64) {
            while head < samples.count, samples[head].eventTime < cutoff { head += 1 }
        }

        private mutating func compactIfNeeded() {
            if head > 256, head * 2 > samples.count {
                samples.removeFirst(head)
                head = 0
            }
        }
    }

    private struct SymbolState: Sendable {
        var lastEventTime: Int64
        var rise = DirectionWindow()
        var fall = DirectionWindow()
    }

    private var states: [UUID: [String: SymbolState]] = [:]

    public init() {}

    public mutating func retainRules(_ ids: Set<UUID>) {
        states = states.filter { ids.contains($0.key) }
    }

    public mutating func resetAll() { states.removeAll() }

    public mutating func evaluate(rule: MarketAlertRule, current: PricePoint) -> [AlertTrigger] {
        guard rule.isEnabled, current.price > .zero else { return [] }
        let sample = Sample(eventTime: current.eventTime, price: current.price, priceText: current.priceText)
        var state = states[rule.id]?[current.symbol] ?? SymbolState(lastEventTime: current.eventTime)
        guard current.eventTime >= state.lastEventTime else { return [] }

        let cutoff = current.eventTime - Int64(rule.windowMinutes) * 60_000
        state.rise.appendMinimum(sample, cutoff: cutoff)
        state.fall.appendMaximum(sample, cutoff: cutoff)
        let riseBase = state.rise.extreme
        let fallBase = state.fall.extreme
        let riseChange = riseBase.map { (current.price - $0.price) / $0.price * 100 }
        let fallChange = fallBase.map { (current.price - $0.price) / $0.price * 100 }
        var triggers: [AlertTrigger] = []

        if let riseBase, let riseChange, riseChange >= rule.threshold {
            triggers.append(makeTrigger(rule: rule, current: current, baseline: riseBase, direction: .rise, change: riseChange))
            state.rise.reset(with: sample)
        }
        if let fallBase, let fallChange, fallChange <= -rule.threshold {
            triggers.append(makeTrigger(rule: rule, current: current, baseline: fallBase, direction: .fall, change: fallChange))
            state.fall.reset(with: sample)
        }
        state.lastEventTime = current.eventTime
        states[rule.id, default: [:]][current.symbol] = state
        return triggers
    }

    private func makeTrigger(
        rule: MarketAlertRule, current: PricePoint, baseline: Sample,
        direction: TriggerDirection, change: Decimal
    ) -> AlertTrigger {
        AlertTrigger(
            ruleID: rule.id, symbol: current.symbol, kind: .marketPercentage,
            direction: direction, changePercent: change, thresholdText: rule.thresholdText,
            windowMinutes: rule.windowMinutes, targetPriceText: nil,
            priceText: current.priceText, baselinePriceText: baseline.priceText,
            eventTime: current.eventTime
        )
    }
}
