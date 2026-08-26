package world.zcn.pricereminder.data

import android.content.Context
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import world.zcn.pricereminder.core.PricePoint
import java.io.File
import java.io.IOException

class LocalStore(context: Context) {
    private val json = Json { ignoreUnknownKeys = true }
    private val masterKey = MasterKey.Builder(context).setKeyScheme(MasterKey.KeyScheme.AES256_GCM).build()
    private val preferences = EncryptedSharedPreferences.create(
        context, "price-reminder", masterKey,
        EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
        EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
    )
    private val priceDirectory = File(context.filesDir, "prices")

    var deviceToken: String?
        get() = preferences.getString("device_token", null)
        set(value) = preferences.edit().putString("device_token", value).apply()

    var primarySymbol: String
        get() = preferences.getString("primary_symbol", "BTCUSDT") ?: "BTCUSDT"
        set(value) = preferences.edit().putString("primary_symbol", value).apply()

    var recentSymbols: List<String>
        get() = decodeList<String>(preferences.getString("recent_symbols", null)).ifEmpty { listOf(primarySymbol) }
        set(value) = preferences.edit().putString(
            "recent_symbols", json.encodeToString(value.distinctBy(String::uppercase).take(3))
        ).apply()

    fun rules(): List<StoredRule> = decodeList(preferences.getString("rules", null))

    fun saveRules(rules: List<StoredRule>) {
        preferences.edit().putString("rules", json.encodeToString(rules)).apply()
    }

    fun history(): List<TriggerHistory> = decodeList(preferences.getString("history", null))

    fun appendHistory(items: List<TriggerHistory>) {
        val cutoff = System.currentTimeMillis() - 30L * 24 * 60 * 60 * 1_000
        val merged = (items + history()).filter { it.eventTime >= cutoff }.distinctBy { it.id }.take(500)
        preferences.edit().putString("history", json.encodeToString(merged)).apply()
    }

    fun loadPricePoints(): List<PricePoint> {
        val cutoff = System.currentTimeMillis() - 60L * 60L * 1_000L
        return priceDirectory.listFiles { file -> file.extension == "json" }
            ?.flatMap { file -> decodeList<PersistedPricePoint>(file.readText()) }
            ?.filter { it.eventTime >= cutoff }
            ?.mapNotNull { runCatching { PricePoint(it.symbol, it.priceText, it.eventTime) }.getOrNull() }
            ?: emptyList()
    }

    @Throws(IOException::class)
    fun savePricePoints(symbol: String, points: List<PricePoint>) {
        require(symbol.matches(Regex("[A-Z0-9]{2,30}"))) { "invalid symbol" }
        priceDirectory.mkdirs()
        val target = File(priceDirectory, "$symbol.json")
        val temporary = File(priceDirectory, "$symbol.tmp")
        val payload = points.map { PersistedPricePoint(it.symbol, it.priceText, it.eventTime) }
        temporary.writeText(json.encodeToString(payload))
        if (!temporary.renameTo(target)) throw IOException("failed to persist $symbol prices")
    }

    private inline fun <reified T> decodeList(value: String?): List<T> =
        value?.let { runCatching { json.decodeFromString<List<T>>(it) }.getOrNull() } ?: emptyList()
}
