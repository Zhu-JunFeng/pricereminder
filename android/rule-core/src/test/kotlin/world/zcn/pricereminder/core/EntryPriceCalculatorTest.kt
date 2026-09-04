package world.zcn.pricereminder.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFailsWith

class EntryPriceCalculatorTest {
    @Test
    fun validatesAndNormalizesEntryPrice() {
        assertEquals("100.0000", EntryPriceCalculator.normalize(" 100.0000 "))
        listOf("", "abc", "0", "-1").forEach {
            assertFailsWith<IllegalArgumentException> { EntryPriceCalculator.normalize(it) }
        }
    }

    @Test
    fun calculatesSignedRoundedPercentageAndNormalizesNegativeZero() {
        assertEquals("+4.27%", EntryPriceCalculator.change("104.265", "100", PositionSide.LONG).percentageText)
        assertEquals("-4.27%", EntryPriceCalculator.change("95.735", "100", PositionSide.LONG).percentageText)
        assertEquals("0.00%", EntryPriceCalculator.change("100", "100", PositionSide.LONG).percentageText)
        assertEquals("+4.27%", EntryPriceCalculator.change("95.735", "100", PositionSide.SHORT).percentageText)
        assertEquals("-4.27%", EntryPriceCalculator.change("104.265", "100", PositionSide.SHORT).percentageText)
        val tinyLoss = EntryPriceCalculator.change("99.999999", "100", PositionSide.LONG)
        assertEquals("0.00%", tinyLoss.percentageText)
        assertEquals(EntryPriceDirection.FLAT, tinyLoss.direction)
    }

    @Test
    fun marksPricesStaleOnlyAfterThirtySeconds() {
        assertEquals(false, EntryPriceCalculator.isStale(1_000, 31_000))
        assertEquals(true, EntryPriceCalculator.isStale(1_000, 31_001))
    }
}
