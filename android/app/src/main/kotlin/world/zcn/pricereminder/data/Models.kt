package world.zcn.pricereminder.data

import kotlinx.serialization.Serializable

@Serializable
data class StoredRule(
    val id: String,
    val symbol: String,
    val windowMinutes: Int,
    val thresholdText: String,
    val enabled: Boolean = true,
    val riseTriggered: Boolean = false,
    val fallTriggered: Boolean = false,
    val kind: String = "percentage",
    val targetDirection: String? = null,
    val targetPriceText: String? = null,
    val targetTriggered: Boolean = false,
)

@Serializable
data class StoredMarketRule(
    val id: String,
    val windowMinutes: Int,
    val thresholdText: String,
    val enabled: Boolean = true,
)

@Serializable
data class StoredEntryPrice(
    val symbol: String,
    val priceText: String,
    val positionSide: String = "long",
)

@Serializable
data class TriggerHistory(
    val id: String,
    val symbol: String,
    val direction: String,
    val kind: String = "percentage",
    val changePercent: String? = null,
    val windowMinutes: Int,
    val thresholdText: String,
    val targetPriceText: String? = null,
    val priceText: String,
    val eventTime: Long,
)

@Serializable
data class ContractDto(
    val symbol: String,
    val baseAsset: String,
    val quoteAsset: String,
    val tickSize: String,
)

@Serializable
data class ExchangeInfoDto(val symbols: List<ExchangeSymbolDto>)

@Serializable
data class ExchangeSymbolDto(
    val symbol: String,
    val status: String,
    val contractType: String,
    val baseAsset: String,
    val quoteAsset: String,
    val filters: List<ExchangeFilterDto>,
)

@Serializable
data class ExchangeFilterDto(val filterType: String, val tickSize: String? = null)

@Serializable
data class ContractsResponse(val contracts: List<ContractDto>)

@Serializable
data class RegistrationResponse(val deviceId: String, val token: String, val expiresAt: Long)

@Serializable
data class SubscriptionsResponse(val symbols: List<String>)

@Serializable
data class RelayMessage(
    val type: String,
    val symbol: String? = null,
    val price: String? = null,
    val eventTime: Long? = null,
    val replay: Boolean = false,
    val state: String? = null,
    val reason: String? = null,
    val serverTime: Long? = null,
)

@Serializable
data class CombinedTradeDto(val stream: String, val data: BinanceTradeDto)

@Serializable
data class BinanceTradeDto(
    @kotlinx.serialization.SerialName("e") val eventType: String,
    @kotlinx.serialization.SerialName("E") val eventTime: Long,
    @kotlinx.serialization.SerialName("s") val symbol: String,
    @kotlinx.serialization.SerialName("p") val price: String,
    @kotlinx.serialization.SerialName("q") val quantity: String,
)

@Serializable
data class MarketMiniTickerDto(
    @kotlinx.serialization.SerialName("e") val eventType: String,
    @kotlinx.serialization.SerialName("E") val eventTime: Long,
    @kotlinx.serialization.SerialName("s") val symbol: String,
    @kotlinx.serialization.SerialName("c") val closePrice: String,
)

@Serializable
data class PersistedPricePoint(
    val symbol: String,
    val priceText: String,
    val eventTime: Long,
)
