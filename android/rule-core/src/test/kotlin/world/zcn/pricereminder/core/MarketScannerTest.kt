package world.zcn.pricereminder.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertTrue

class MarketScannerTest {
    @Test
    fun rollingExtremesTriggerBeforeFullWindowAndResetDirectionsIndependently() {
        val scanner = MarketScanner()
        val rule = MarketAlertRule(id = "market-1", windowMinutes = 5, thresholdText = "4")
        val inputs = listOf(
            0L to "100",
            30_000L to "98",
            60_000L to "102",
            61_000L to "97",
            62_000L to "102",
        )

        val triggers = inputs.flatMap { (eventTime, price) ->
            scanner.evaluate(rule, PricePoint("BTCUSDT", price, eventTime))
        }

        assertEquals(
            listOf(TriggerDirection.RISE, TriggerDirection.FALL, TriggerDirection.RISE),
            triggers.map { it.direction },
        )
        assertEquals(AlertRuleKind.MARKET_PERCENTAGE, triggers.first().kind)
        assertEquals("98", triggers.first().baselinePriceText)
        assertTrue(triggers.first().changePercent!! > "4.08".toBigDecimal())
    }

    @Test
    fun sparsePricesInsideWindowRemainEligible() {
        val scanner = MarketScanner()
        val rule = MarketAlertRule(id = "market-2", windowMinutes = 1, thresholdText = "5")

        assertTrue(scanner.evaluate(rule, PricePoint("ETHUSDT", "100", 0)).isEmpty())
        assertEquals(
            listOf(TriggerDirection.RISE),
            scanner.evaluate(rule, PricePoint("ETHUSDT", "105", 40_001)).map { it.direction },
        )
    }

    @Test
    fun rulesAndContractsKeepIndependentState() {
        val scanner = MarketScanner()
        val fast = MarketAlertRule(id = "fast", windowMinutes = 1, thresholdText = "2")
        val slow = MarketAlertRule(id = "slow", windowMinutes = 5, thresholdText = "5")

        listOf(fast, slow).forEach { rule ->
            scanner.evaluate(rule, PricePoint("BTCUSDT", "100", 0))
            scanner.evaluate(rule, PricePoint("ETHUSDT", "200", 0))
        }

        assertEquals(1, scanner.evaluate(fast, PricePoint("BTCUSDT", "102", 1_000)).size)
        assertTrue(scanner.evaluate(slow, PricePoint("BTCUSDT", "102", 1_000)).isEmpty())
        assertTrue(scanner.evaluate(fast, PricePoint("ETHUSDT", "202", 1_000)).isEmpty())
    }
}
