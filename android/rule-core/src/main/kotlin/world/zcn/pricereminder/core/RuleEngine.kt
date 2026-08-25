package world.zcn.pricereminder.core

import java.math.BigDecimal
import java.math.MathContext

object RuleEngine {
    const val STALE_MILLIS = 30_000L
    private val hundred = BigDecimal("100")

    fun evaluate(rule: AlertRule, current: PricePoint, buffer: PriceBuffer): List<AlertTrigger> {
        if (!rule.enabled || current.symbol != rule.symbol) return emptyList()
        if (rule.kind == AlertRuleKind.TARGET) return evaluateTarget(rule, current)
        val cutoff = current.eventTime - rule.windowMinutes * 60_000L
        val baseline = buffer.atOrBefore(rule.symbol, cutoff) ?: return emptyList()
        if (cutoff - baseline.eventTime > STALE_MILLIS || baseline.price.signum() == 0) return emptyList()
        val change = current.price.subtract(baseline.price)
            .divide(baseline.price, MathContext.DECIMAL128)
            .multiply(hundred)
        val negativeThreshold = rule.threshold.negate()

        if (rule.riseTriggered && change < rule.threshold) rule.riseTriggered = false
        if (rule.fallTriggered && change > negativeThreshold) rule.fallTriggered = false

        return buildList {
            if (!rule.riseTriggered && change >= rule.threshold) {
                rule.riseTriggered = true
                add(trigger(rule, current, baseline, TriggerDirection.RISE, change))
            }
            if (!rule.fallTriggered && change <= negativeThreshold) {
                rule.fallTriggered = true
                add(trigger(rule, current, baseline, TriggerDirection.FALL, change))
            }
        }
    }

    fun initialize(rule: AlertRule, current: PricePoint, buffer: PriceBuffer): Boolean {
        if (rule.kind == AlertRuleKind.TARGET) {
            val direction = rule.targetDirection ?: return false
            val target = rule.targetPrice ?: return false
            rule.targetTriggered = reached(direction, current.price, target)
            return true
        }
        val cutoff = current.eventTime - rule.windowMinutes * 60_000L
        val baseline = buffer.atOrBefore(rule.symbol, cutoff) ?: return false
        if (cutoff - baseline.eventTime > STALE_MILLIS || baseline.price.signum() == 0) return false
        val change = current.price.subtract(baseline.price)
            .divide(baseline.price, MathContext.DECIMAL128)
            .multiply(hundred)
        rule.riseTriggered = change >= rule.threshold
        rule.fallTriggered = change <= rule.threshold.negate()
        return true
    }

    private fun evaluateTarget(rule: AlertRule, current: PricePoint): List<AlertTrigger> {
        val direction = rule.targetDirection ?: return emptyList()
        val target = rule.targetPrice ?: return emptyList()
        val isReached = reached(direction, current.price, target)
        if (rule.targetTriggered && !isReached) rule.targetTriggered = false
        if (rule.targetTriggered || !isReached) return emptyList()
        rule.targetTriggered = true
        return listOf(
            AlertTrigger(
                ruleId = rule.id, symbol = rule.symbol, kind = AlertRuleKind.TARGET,
                direction = if (direction == TargetDirection.ABOVE) TriggerDirection.RISE else TriggerDirection.FALL,
                changePercent = null, thresholdText = "", windowMinutes = 0,
                targetPriceText = rule.targetPriceText, priceText = current.priceText,
                baselinePriceText = "", eventTime = current.eventTime,
            )
        )
    }

    private fun reached(direction: TargetDirection, current: BigDecimal, target: BigDecimal): Boolean =
        if (direction == TargetDirection.ABOVE) current >= target else current <= target

    private fun trigger(
        rule: AlertRule, current: PricePoint, baseline: PricePoint,
        direction: TriggerDirection, change: BigDecimal,
    ) = AlertTrigger(
        ruleId = rule.id, symbol = rule.symbol, kind = AlertRuleKind.PERCENTAGE, direction = direction,
        changePercent = change, thresholdText = rule.thresholdText,
        windowMinutes = rule.windowMinutes, targetPriceText = null, priceText = current.priceText,
        baselinePriceText = baseline.priceText, eventTime = current.eventTime,
    )
}
