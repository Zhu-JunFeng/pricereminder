package world.zcn.pricereminder.core

import java.math.BigDecimal
import java.math.RoundingMode

enum class EntryPriceDirection { RISE, FALL, FLAT }
enum class PositionSide { LONG, SHORT }

data class EntryPriceChange(
    val percentage: BigDecimal,
    val percentageText: String,
    val direction: EntryPriceDirection,
)

object EntryPriceCalculator {
    fun isStale(eventTime: Long, nowMilliseconds: Long): Boolean = nowMilliseconds - eventTime > 30_000

    fun normalize(priceText: String): String {
        val normalized = priceText.trim()
        val price = normalized.toBigDecimalOrNull()
        require(price != null && price > BigDecimal.ZERO) { "entry price must be greater than zero" }
        return normalized
    }

    fun change(
        currentPriceText: String, entryPriceText: String, positionSide: PositionSide,
    ): EntryPriceChange {
        val current = currentPriceText.toBigDecimal()
        val entry = normalize(entryPriceText).toBigDecimal()
        val difference = when (positionSide) {
            PositionSide.LONG -> current.subtract(entry)
            PositionSide.SHORT -> entry.subtract(current)
        }
        var rounded = difference.multiply(BigDecimal("100"))
            .divide(entry, 2, RoundingMode.HALF_UP)
        if (rounded.compareTo(BigDecimal.ZERO) == 0) rounded = BigDecimal.ZERO.setScale(2)
        val direction = when {
            rounded > BigDecimal.ZERO -> EntryPriceDirection.RISE
            rounded < BigDecimal.ZERO -> EntryPriceDirection.FALL
            else -> EntryPriceDirection.FLAT
        }
        val text = when (direction) {
            EntryPriceDirection.RISE -> "+${rounded.toPlainString()}%"
            EntryPriceDirection.FALL -> "${rounded.toPlainString()}%"
            EntryPriceDirection.FLAT -> "0.00%"
        }
        return EntryPriceChange(rounded, text, direction)
    }
}
