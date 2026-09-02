package world.zcn.pricereminder.core

import java.math.BigDecimal
import java.util.ArrayDeque

class MarketScanner {
    private data class Sample(val eventTime: Long, val price: BigDecimal, val priceText: String)
    private data class SymbolState(
        var lastEventTime: Long,
        val rise: ArrayDeque<Sample> = ArrayDeque(),
        val fall: ArrayDeque<Sample> = ArrayDeque(),
    )

    private val states = mutableMapOf<String, MutableMap<String, SymbolState>>()

    @Synchronized
    fun retainRules(ids: Set<String>) { states.keys.retainAll(ids) }

    @Synchronized
    fun resetAll() = states.clear()

    @Synchronized
    fun evaluate(rule: MarketAlertRule, current: PricePoint): List<AlertTrigger> {
        if (!rule.enabled || current.price <= BigDecimal.ZERO) return emptyList()
        val bySymbol = states.getOrPut(rule.id) { mutableMapOf() }
        val sample = Sample(current.eventTime, current.price, current.priceText)
        val state = bySymbol.getOrPut(current.symbol) {
            SymbolState(current.eventTime).apply {
                rise.add(sample)
                fall.add(sample)
            }
        }
        if (current.eventTime < state.lastEventTime) return emptyList()
        val cutoff = current.eventTime - rule.windowMinutes * 60_000L
        state.rise.purge(cutoff)
        while (state.rise.isNotEmpty() && state.rise.last().price >= sample.price) state.rise.removeLast()
        state.rise.addLast(sample)
        state.fall.purge(cutoff)
        while (state.fall.isNotEmpty() && state.fall.last().price <= sample.price) state.fall.removeLast()
        state.fall.addLast(sample)

        val riseBase = state.rise.first()
        val fallBase = state.fall.first()
        val riseChange = current.price.subtract(riseBase.price).divide(riseBase.price, 16, java.math.RoundingMode.HALF_UP)
            .multiply(BigDecimal("100"))
        val fallChange = current.price.subtract(fallBase.price).divide(fallBase.price, 16, java.math.RoundingMode.HALF_UP)
            .multiply(BigDecimal("100"))
        val result = mutableListOf<AlertTrigger>()
        if (riseChange >= rule.threshold) {
            result += trigger(rule, current, riseBase, TriggerDirection.RISE, riseChange)
            state.rise.reset(sample)
        }
        if (fallChange <= rule.threshold.negate()) {
            result += trigger(rule, current, fallBase, TriggerDirection.FALL, fallChange)
            state.fall.reset(sample)
        }
        state.lastEventTime = current.eventTime
        return result
    }

    private fun ArrayDeque<Sample>.purge(cutoff: Long) {
        while (isNotEmpty() && first().eventTime < cutoff) removeFirst()
    }

    private fun ArrayDeque<Sample>.reset(sample: Sample) {
        clear()
        add(sample)
    }

    private fun trigger(
        rule: MarketAlertRule, current: PricePoint, baseline: Sample,
        direction: TriggerDirection, change: BigDecimal,
    ) = AlertTrigger(
        rule.id, current.symbol, AlertRuleKind.MARKET_PERCENTAGE, direction, change,
        rule.thresholdText, rule.windowMinutes, null, current.priceText, baseline.priceText, current.eventTime,
    )
}
