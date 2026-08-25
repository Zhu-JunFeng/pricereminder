package world.zcn.pricereminder.core

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class RuleEngineTest {
    @Test fun equalThresholdTriggersAndRearms() {
        val buffer = PriceBuffer()
        val rule = AlertRule(symbol = "BTCUSDT", windowMinutes = 1, thresholdText = "5")
        buffer.add(PricePoint("BTCUSDT", "100", 0))
        var current = PricePoint("BTCUSDT", "105", 60_000)
        buffer.add(current)
        assertEquals(listOf(TriggerDirection.RISE), RuleEngine.evaluate(rule, current, buffer).map { it.direction })
        current = PricePoint("BTCUSDT", "104", 61_000)
        buffer.add(current)
        assertTrue(RuleEngine.evaluate(rule, current, buffer).isEmpty())
        assertFalse(rule.riseTriggered)
        current = PricePoint("BTCUSDT", "106", 62_000)
        buffer.add(current)
        assertEquals(listOf(TriggerDirection.RISE), RuleEngine.evaluate(rule, current, buffer).map { it.direction })
    }

    @Test fun directionsAreIndependent() {
        val buffer = PriceBuffer()
        val rule = AlertRule(symbol = "ETHUSDT", windowMinutes = 1, thresholdText = "5")
        buffer.add(PricePoint("ETHUSDT", "100", 0))
        var current = PricePoint("ETHUSDT", "106", 60_000)
        buffer.add(current)
        assertEquals(listOf(TriggerDirection.RISE), RuleEngine.evaluate(rule, current, buffer).map { it.direction })
        current = PricePoint("ETHUSDT", "94", 61_000)
        buffer.add(current)
        assertEquals(listOf(TriggerDirection.FALL), RuleEngine.evaluate(rule, current, buffer).map { it.direction })
    }

    @Test fun incompleteWindowDoesNotTrigger() {
        val buffer = PriceBuffer()
        val rule = AlertRule(symbol = "SOLUSDT", windowMinutes = 5, thresholdText = "2")
        buffer.add(PricePoint("SOLUSDT", "100", 0))
        val current = PricePoint("SOLUSDT", "110", 299_999)
        buffer.add(current)
        assertTrue(RuleEngine.evaluate(rule, current, buffer).isEmpty())
    }

    @Test fun targetPriceTriggersOnceAndRearmsAfterLeavingRange() {
        val buffer = PriceBuffer()
        val rule = AlertRule(
            symbol = "BTCUSDT", windowMinutes = 0, thresholdText = "0",
            kind = AlertRuleKind.TARGET, targetDirection = TargetDirection.ABOVE,
            targetPriceText = "105",
        )
        var current = PricePoint("BTCUSDT", "104", 1_000)
        buffer.add(current)
        assertTrue(RuleEngine.evaluate(rule, current, buffer).isEmpty())
        current = PricePoint("BTCUSDT", "105", 2_000)
        buffer.add(current)
        val trigger = RuleEngine.evaluate(rule, current, buffer).single()
        assertEquals(AlertRuleKind.TARGET, trigger.kind)
        assertEquals("105", trigger.targetPriceText)
        current = PricePoint("BTCUSDT", "106", 3_000)
        buffer.add(current)
        assertTrue(RuleEngine.evaluate(rule, current, buffer).isEmpty())
        current = PricePoint("BTCUSDT", "104", 4_000)
        buffer.add(current)
        assertTrue(RuleEngine.evaluate(rule, current, buffer).isEmpty())
        assertFalse(rule.targetTriggered)
    }

    @Test fun restoredBufferKeepsLatestPointPerSecondAndOneHour() {
        val buffer = PriceBuffer()
        buffer.restore(
            listOf(
                PricePoint("BTCUSDT", "99", 1_000),
                PricePoint("BTCUSDT", "100", 2_000),
                PricePoint("BTCUSDT", "101", 2_900),
                PricePoint("BTCUSDT", "102", PriceBuffer.RETENTION_MILLIS + 2_000),
            )
        )

        assertEquals(listOf("101", "102"), buffer.points("BTCUSDT").map { it.priceText })
        assertEquals(setOf("BTCUSDT"), buffer.symbols())
    }
}
