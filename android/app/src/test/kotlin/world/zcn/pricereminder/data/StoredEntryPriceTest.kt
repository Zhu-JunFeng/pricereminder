package world.zcn.pricereminder.data

import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlin.test.Test
import kotlin.test.assertEquals

class StoredEntryPriceTest {
    @Test
    fun defaultsLegacyEntriesToLong() {
        val entries = Json.decodeFromString<List<StoredEntryPrice>>(
            """[{"symbol":"BTCUSDT","priceText":"100"}]"""
        )

        assertEquals("long", entries.single().positionSide)
    }

    @Test
    fun persistsShortEntries() {
        val entry = StoredEntryPrice("BTCUSDT", "100", "short")
        val restored = Json.decodeFromString<List<StoredEntryPrice>>(
            Json.encodeToString(listOf(entry))
        )

        assertEquals(entry, restored.single())
    }
}
