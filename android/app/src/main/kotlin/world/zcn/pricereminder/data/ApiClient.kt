package world.zcn.pricereminder.data

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.put
import okhttp3.OkHttpClient
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.util.concurrent.TimeUnit

class ApiClient(
    private val localStore: LocalStore,
    private val serverBaseUrl: String,
) {
    val http = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .pingInterval(20, TimeUnit.SECONDS)
        .build()
    val json = Json { ignoreUnknownKeys = true }

    suspend fun contracts(): List<ContractDto> = runCatching { directContracts() }
        .getOrElse { serverContracts() }.plus(GoldQuote.contract)

    suspend fun goldQuote(): GoldQuote = withContext(Dispatchers.IO) {
        http.newCall(Request.Builder().url("$serverBaseUrl/v1/gold/quote").build()).execute().use {
            check(it.isSuccessful) { "黄金行情暂不可用" }
            json.decodeFromString<GoldQuote>(it.body?.string().orEmpty())
        }
    }

    suspend fun probeDirect(): Boolean = withContext(Dispatchers.IO) {
        val done=java.util.concurrent.CountDownLatch(1)
        val connected=java.util.concurrent.atomic.AtomicBoolean(false)
        val socket=http.newWebSocket(Request.Builder().url("wss://fstream.binance.com/ws").build(),object:okhttp3.WebSocketListener(){
            override fun onOpen(ws:okhttp3.WebSocket,response:okhttp3.Response){connected.set(true);done.countDown()}
            override fun onFailure(ws:okhttp3.WebSocket,t:Throwable,response:okhttp3.Response?){done.countDown()}
        })
        try { done.await(8,TimeUnit.SECONDS); connected.get() } finally { socket.cancel() }
    }

    suspend fun binanceContracts(): List<ContractDto> = directContracts()

    private suspend fun directContracts(): List<ContractDto> = withContext(Dispatchers.IO) {
        val response = http.newCall(
            Request.Builder().url("https://fapi.binance.com/fapi/v1/exchangeInfo").build()
        ).execute()
        response.use {
            check(it.isSuccessful) { "币安合约列表请求失败：HTTP ${it.code}" }
            json.decodeFromString<ExchangeInfoDto>(it.body?.string().orEmpty()).symbols
                .asSequence()
                .filter { symbol ->
                    symbol.status == "TRADING" &&
                        (symbol.contractType == "PERPETUAL" || symbol.contractType == "TRADIFI_PERPETUAL") &&
                        (symbol.quoteAsset == "USDT" || symbol.quoteAsset == "USDC")
                }
                .mapNotNull { symbol ->
                    val tickSize = symbol.filters.firstOrNull { filter -> filter.filterType == "PRICE_FILTER" }?.tickSize
                        ?: return@mapNotNull null
                    ContractDto(symbol.symbol, symbol.baseAsset, symbol.quoteAsset, tickSize)
                }
                .sortedBy(ContractDto::symbol)
                .toList()
        }
    }

    fun streamRequest(symbols: List<String>): Request {
        val streams = symbols.distinct().sorted().joinToString("/") { it.lowercase() + "@trade" }
        require(streams.isNotEmpty()) { "至少选择一个合约" }
        return Request.Builder().url("wss://fstream.binance.com/stream?streams=$streams").build()
    }

    fun allMarketRequest(): Request = Request.Builder()
        .url("wss://fstream.binance.com/ws/!miniTicker@arr").build()

    suspend fun serverStreamRequest(symbols: List<String>): Request = withContext(Dispatchers.IO) {
        val token = authenticatedToken()
        Request.Builder().url(serverBaseUrl.replaceFirst("https://", "wss://") + "/v2/stream")
            .header("Authorization", "Bearer $token").build()
    }

    fun resumeMessage(lastEventTimes: Map<String, Long>): String =
        buildJsonObject {
            put("type", "resume")
            put("lastEventTime", buildJsonObject {
                    lastEventTimes.forEach { (symbol, eventTime) -> put(symbol, eventTime) }
            })
        }.toString()

    private suspend fun serverContracts(): List<ContractDto> = withContext(Dispatchers.IO) {
        val token = authenticatedToken()
        http.newCall(
            Request.Builder().url("$serverBaseUrl/v1/contracts")
                .header("Authorization", "Bearer $token").build()
        ).execute().use {
            check(it.isSuccessful) { "服务端合约列表请求失败：HTTP ${it.code}" }
            json.decodeFromString<ContractsResponse>(it.body?.string().orEmpty()).contracts
        }
    }

    private fun authenticatedToken(): String {
        localStore.deviceToken?.let { token ->
            val request = Request.Builder().url("$serverBaseUrl/v1/devices/refresh")
                .header("Authorization", "Bearer $token")
                .post(ByteArray(0).toRequestBody(null)).build()
            http.newCall(request).execute().use { response ->
                if (response.isSuccessful) return token
                if (response.code != 401) error("服务端设备续期失败：HTTP ${response.code}")
            }
            localStore.deviceToken = null
        }
        val body = "{\"platform\":\"android\",\"displayName\":\"Android\"}"
            .toRequestBody(JSON_MEDIA_TYPE)
        val registration = http.newCall(
            Request.Builder().url("$serverBaseUrl/v1/devices/register").post(body).build()
        ).execute().use {
            check(it.isSuccessful) { "服务端设备注册失败：HTTP ${it.code}" }
            json.decodeFromString<RegistrationResponse>(it.body?.string().orEmpty())
        }
        localStore.deviceToken = registration.token
        return registration.token
    }

    private companion object {
        val JSON_MEDIA_TYPE = "application/json".toMediaType()
    }
}
