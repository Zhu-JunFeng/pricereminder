package world.zcn.pricereminder.core

import java.math.BigDecimal
import java.util.UUID

data class PricePoint(
    val symbol: String,
    val priceText: String,
    val eventTime: Long,
    val replay: Boolean = false,
) {
    val price: BigDecimal = priceText.toBigDecimal()
}

enum class TriggerDirection { RISE, FALL }
enum class AlertRuleKind { PERCENTAGE, TARGET }
enum class TargetDirection { ABOVE, BELOW }

object ContractOrdering {
    fun orderedSymbols(symbols: List<String>, recentSymbols: List<String>, query: String = ""): List<String> {
        val normalized = query.trim()
        val filtered = symbols.filter { normalized.isEmpty() || it.contains(normalized, ignoreCase = true) }
        val ranks = recentSymbols.take(3).mapIndexed { index, symbol -> symbol.uppercase() to index }.toMap()
        return filtered.withIndex().sortedWith(
            compareBy<IndexedValue<String>> { ranks[it.value.uppercase()] ?: Int.MAX_VALUE }
                .thenBy { it.index }
        ).map { it.value }
    }
}

data class AlertRule(
    val id: String = UUID.randomUUID().toString(),
    val symbol: String,
    val windowMinutes: Int,
    val thresholdText: String,
    val enabled: Boolean = true,
    var riseTriggered: Boolean = false,
    var fallTriggered: Boolean = false,
    val kind: AlertRuleKind = AlertRuleKind.PERCENTAGE,
    val targetDirection: TargetDirection? = null,
    val targetPriceText: String? = null,
    var targetTriggered: Boolean = false,
) {
    val threshold: BigDecimal = thresholdText.toBigDecimalOrNull() ?: BigDecimal.ZERO
    val targetPrice: BigDecimal? = targetPriceText?.toBigDecimalOrNull()

    init {
        if (kind == AlertRuleKind.PERCENTAGE) {
            require(windowMinutes in 1..60) { "windowMinutes must be between 1 and 60" }
            require(threshold >= BigDecimal("0.1") && threshold <= BigDecimal("100")) {
                "thresholdPct must be between 0.1 and 100"
            }
        } else {
            require(targetDirection != null) { "targetDirection is required" }
            require(targetPrice != null && targetPrice > BigDecimal.ZERO) { "targetPrice must be greater than zero" }
        }
    }
}

data class AlertTrigger(
    val ruleId: String,
    val symbol: String,
    val kind: AlertRuleKind,
    val direction: TriggerDirection,
    val changePercent: BigDecimal?,
    val thresholdText: String,
    val windowMinutes: Int,
    val targetPriceText: String?,
    val priceText: String,
    val baselinePriceText: String,
    val eventTime: Long,
)
