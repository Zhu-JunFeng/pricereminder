package world.zcn.pricereminder.data

import kotlin.test.Test
import kotlin.test.assertEquals
import kotlin.test.assertFalse
import kotlin.test.assertTrue

class GoldQuoteTest {
    @Test fun sourceTimeAndPreviousClose() {
        val now = System.currentTimeMillis()
        val quote = GoldQuote("840", "800", now - 24000, now, "quoted")
        assertTrue(quote.eligible)
        assertEquals("+5.00%", quote.dailyChange)
        assertFalse(quote.copy(eventTime = now - 121000).eligible)
        assertFalse(quote.copy(state = "closed").eligible)
        assertEquals("--", quote.copy(previousClose = "").dailyChange)
        assertEquals("--", quote.copy(previousClose = "0").dailyChange)
        assertEquals("--", quote.copy(previousClose = "-1").dailyChange)
    }
}
