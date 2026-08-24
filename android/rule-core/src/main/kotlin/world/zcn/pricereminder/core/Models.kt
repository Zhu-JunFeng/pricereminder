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

data class AlertRule(
    val id: String = UUID.randomUUID().toString(),
    val symbol: String,
    val windowMinutes: Int,
    val thresholdText: String,
    val enabled: Boolean = true,
    var riseTriggered: Boolean = false,
    var fallTriggered: Boolean = false,
) {
    val threshold: BigDecimal = thresholdText.toBigDecimal()

    init {
        require(windowMinutes in 1..60) { "windowMinutes must be between 1 and 60" }
        require(threshold >= BigDecimal("0.1") && threshold <= BigDecimal("100")) {
            "thresholdPct must be between 0.1 and 100"
        }
    }
}

data class AlertTrigger(
    val ruleId: String,
    val symbol: String,
    val direction: TriggerDirection,
    val changePercent: BigDecimal,
    val thresholdText: String,
    val windowMinutes: Int,
    val priceText: String,
    val baselinePriceText: String,
    val eventTime: Long,
)
