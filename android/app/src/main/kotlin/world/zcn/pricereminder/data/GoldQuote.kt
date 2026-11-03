package world.zcn.pricereminder.data

import kotlinx.serialization.Serializable

@Serializable
data class GoldQuote(val price: String, val previousClose: String, val eventTime: Long, val receivedAt: Long, val state: String) {
    val eligible: Boolean get() = state == "quoted" && System.currentTimeMillis()-eventTime <= 120_000
    val dailyChange: String get() = runCatching {
        val close = previousClose.toBigDecimal()
        require(close.signum() > 0)
        val change = price.toBigDecimal().subtract(close).multiply(100.toBigDecimal()).divide(close,2,java.math.RoundingMode.HALF_UP)
        (if (change.signum() > 0) "+" else "") + change.toPlainString() + "%"
    }.getOrDefault("--")
    companion object {
        const val SYMBOL = "SGE_AU9999"
        val contract = ContractDto(SYMBOL,"上海黄金 Au99.99","CNY","0.01")
        fun name(symbol: String) = if (symbol == SYMBOL) "上海黄金 Au99.99" else symbol
    }
}
