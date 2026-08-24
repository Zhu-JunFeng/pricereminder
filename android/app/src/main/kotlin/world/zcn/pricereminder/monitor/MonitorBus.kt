package world.zcn.pricereminder.monitor

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow

data class LivePrice(val symbol: String, val price: String, val eventTime: Long)

data class MonitorSnapshot(
    val running: Boolean = false,
    val connected: Boolean = false,
    val warmingUp: Boolean = true,
    val staleSymbols: Set<String> = emptySet(),
    val message: String = "监控未启动",
    val prices: Map<String, LivePrice> = emptyMap(),
)

object MonitorBus {
    private val mutable = MutableStateFlow(MonitorSnapshot())
    val state: StateFlow<MonitorSnapshot> = mutable.asStateFlow()

    fun update(transform: (MonitorSnapshot) -> MonitorSnapshot) { mutable.value = transform(mutable.value) }
}
