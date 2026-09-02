package world.zcn.pricereminder

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import world.zcn.pricereminder.data.ContractDto
import world.zcn.pricereminder.data.StoredRule
import world.zcn.pricereminder.data.StoredMarketRule
import world.zcn.pricereminder.data.TriggerHistory
import world.zcn.pricereminder.monitor.MonitorBus
import world.zcn.pricereminder.monitor.NotificationCoordinator
import java.util.UUID

data class AppUiState(
    val contracts: List<ContractDto> = emptyList(),
    val rules: List<StoredRule> = emptyList(),
    val marketRules: List<StoredMarketRule> = emptyList(),
    val history: List<TriggerHistory> = emptyList(),
    val primarySymbol: String = "BTCUSDT",
    val recentSymbols: List<String> = listOf("BTCUSDT"),
    val loading: Boolean = true,
    val error: String? = null,
    val selfCheckMessage: String? = null,
)

class MainViewModel(application: Application) : AndroidViewModel(application) {
    private val container = (application as PriceReminderApplication).container
    private val mutableState = MutableStateFlow(
        AppUiState(
            rules = container.localStore.rules(), marketRules = container.localStore.marketRules(),
            history = container.localStore.history(),
            primarySymbol = container.localStore.primarySymbol,
            recentSymbols = container.localStore.recentSymbols,
        )
    )
    val state: StateFlow<AppUiState> = mutableState.asStateFlow()

    init { refreshContracts() }

    fun refreshContracts() {
        viewModelScope.launch {
            mutableState.value = mutableState.value.copy(loading = true, error = null)
            runCatching { container.apiClient.contracts() }
                .onSuccess { mutableState.value = mutableState.value.copy(contracts = it, loading = false) }
                .onFailure { mutableState.value = mutableState.value.copy(loading = false, error = it.message ?: "合约列表加载失败") }
        }
    }

    fun selectPrimary(symbol: String) {
        container.localStore.primarySymbol = symbol
        recordRecentSymbol(symbol)
        mutableState.value = mutableState.value.copy(primarySymbol = symbol)
        syncSubscriptions()
    }

    fun addRule(symbol: String, window: Int, threshold: String): String? {
        val normalized = threshold.trim()
        if (window !in 1..60) return "时间窗口必须在 1 到 60 分钟之间"
        val value = normalized.toBigDecimalOrNull() ?: return "请输入有效百分比"
        if (value < "0.1".toBigDecimal() || value > "100".toBigDecimal()) return "变化阈值必须在 0.1% 到 100% 之间"
        val current = mutableState.value.rules
        if (current.size + mutableState.value.marketRules.size >= 50) return "每台设备最多 50 条规则"
        if (current.any { it.kind == "percentage" && it.symbol == symbol && it.windowMinutes == window && it.thresholdText.toBigDecimalOrNull() == value }) {
            return "相同合约、窗口和阈值的规则已存在"
        }
        val updated = current + StoredRule(UUID.randomUUID().toString(), symbol, window, normalized)
        recordRecentSymbol(symbol)
        saveRules(updated)
        return null
    }

    fun addTargetRule(symbol: String, direction: String, targetPrice: String): String? {
        val normalized = targetPrice.trim()
        val value = normalized.toBigDecimalOrNull() ?: return "请输入有效目标价格"
        if (value <= java.math.BigDecimal.ZERO) return "目标价格必须大于 0"
        val current = mutableState.value.rules
        if (current.size + mutableState.value.marketRules.size >= 50) return "每台设备最多 50 条规则"
        if (current.any {
                it.kind == "target" && it.symbol == symbol && it.targetDirection == direction &&
                    it.targetPriceText?.toBigDecimalOrNull() == value
            }) return "相同合约、方向和目标价格的规则已存在"
        val updated = current + StoredRule(
            id = UUID.randomUUID().toString(), symbol = symbol, windowMinutes = 0,
            thresholdText = "0", kind = "target", targetDirection = direction,
            targetPriceText = normalized,
        )
        recordRecentSymbol(symbol)
        saveRules(updated)
        return null
    }

    fun addMarketRule(window: Int, threshold: String): String? {
        val normalized = threshold.trim()
        if (window !in 1..60) return "时间窗口必须在 1 到 60 分钟之间"
        val value = normalized.toBigDecimalOrNull() ?: return "请输入有效百分比"
        if (value < "0.1".toBigDecimal() || value > "100".toBigDecimal()) return "变化阈值必须在 0.1% 到 100% 之间"
        val current = mutableState.value.marketRules
        if (mutableState.value.rules.size + current.size >= 50) return "每台设备最多 50 条规则"
        if (current.any { it.windowMinutes == window && it.thresholdText.toBigDecimalOrNull() == value }) {
            return "相同窗口和阈值的全市场规则已存在"
        }
        saveMarketRules(current + StoredMarketRule(UUID.randomUUID().toString(), window, normalized))
        return null
    }

    fun updateMarketRule(id: String, window: Int, threshold: String): String? {
        val normalized = threshold.trim()
        if (window !in 1..60) return "时间窗口必须在 1 到 60 分钟之间"
        val value = normalized.toBigDecimalOrNull() ?: return "请输入有效百分比"
        if (value < "0.1".toBigDecimal() || value > "100".toBigDecimal()) return "变化阈值必须在 0.1% 到 100% 之间"
        val current = mutableState.value.marketRules
        if (current.any { it.id != id && it.windowMinutes == window && it.thresholdText.toBigDecimalOrNull() == value }) {
            return "相同窗口和阈值的全市场规则已存在"
        }
        saveMarketRules(current.map {
            if (it.id == id) StoredMarketRule(id, window, normalized, it.enabled) else it
        })
        return null
    }

    fun toggleMarketRule(id: String, enabled: Boolean) {
        saveMarketRules(mutableState.value.marketRules.map { if (it.id == id) it.copy(enabled = enabled) else it })
    }

    fun deleteMarketRule(id: String) {
        saveMarketRules(mutableState.value.marketRules.filterNot { it.id == id })
    }

    fun updateRule(
        id: String, symbol: String, kind: String, window: Int, threshold: String,
        direction: String, targetPrice: String,
    ): String? {
        val current = mutableState.value.rules
        val existing = current.firstOrNull { it.id == id } ?: return null
        val replacement = if (kind == "percentage") {
            val normalized = threshold.trim()
            if (window !in 1..60) return "时间窗口必须在 1 到 60 分钟之间"
            val value = normalized.toBigDecimalOrNull() ?: return "请输入有效百分比"
            if (value < "0.1".toBigDecimal() || value > "100".toBigDecimal()) return "变化阈值必须在 0.1% 到 100% 之间"
            if (current.any {
                    it.id != id && it.kind == "percentage" && it.symbol == symbol &&
                        it.windowMinutes == window && it.thresholdText.toBigDecimalOrNull() == value
                }) return "相同合约、窗口和阈值的规则已存在"
            StoredRule(id, symbol, window, normalized, enabled = existing.enabled)
        } else {
            val normalized = targetPrice.trim()
            val value = normalized.toBigDecimalOrNull() ?: return "请输入有效目标价格"
            if (value <= java.math.BigDecimal.ZERO) return "目标价格必须大于 0"
            if (current.any {
                    it.id != id && it.kind == "target" && it.symbol == symbol &&
                        it.targetDirection == direction && it.targetPriceText?.toBigDecimalOrNull() == value
                }) return "相同合约、方向和目标价格的规则已存在"
            StoredRule(
                id, symbol, 0, "0", enabled = existing.enabled, kind = "target",
                targetDirection = direction, targetPriceText = normalized,
            )
        }
        recordRecentSymbol(symbol)
        saveRules(current.map { if (it.id == id) replacement else it })
        return null
    }

    fun recordRecentSymbol(symbol: String) {
        val recent = (listOf(symbol.uppercase()) + mutableState.value.recentSymbols)
            .distinctBy(String::uppercase).take(3)
        container.localStore.recentSymbols = recent
        mutableState.value = mutableState.value.copy(recentSymbols = recent)
    }

    fun toggleRule(id: String, enabled: Boolean) {
        saveRules(mutableState.value.rules.map {
            if (it.id == id) it.copy(enabled = enabled, riseTriggered = false, fallTriggered = false, targetTriggered = false) else it
        })
    }

    fun deleteRule(id: String) { saveRules(mutableState.value.rules.filterNot { it.id == id }) }

    fun markNotificationPermissionUnavailable() {
        mutableState.value = mutableState.value.copy(error = "系统通知未开启；实时价格和本地规则计算仍会继续")
    }

    fun refreshSelfCheck() {
        val application = getApplication<Application>()
        val enabled = NotificationCoordinator(application).notificationsEnabled()
        MonitorBus.update { it.copy(notificationsEnabled = enabled) }
        mutableState.value = mutableState.value.copy(selfCheckMessage = "状态已刷新")
    }

    fun sendTestNotification() {
        val sent = NotificationCoordinator(getApplication()).test()
        mutableState.value = mutableState.value.copy(
            selfCheckMessage = if (sent) "测试通知已发送" else "通知权限未开启，请前往系统设置允许通知"
        )
    }

    private fun saveRules(rules: List<StoredRule>) {
        container.localStore.saveRules(rules)
        mutableState.value = mutableState.value.copy(rules = rules)
        syncSubscriptions()
    }

    private fun saveMarketRules(rules: List<StoredMarketRule>) {
        container.localStore.saveMarketRules(rules)
        mutableState.value = mutableState.value.copy(marketRules = rules)
        syncSubscriptions()
    }

    private fun syncSubscriptions() {
        // The foreground monitor reads local configuration and updates Binance subscriptions.
    }
}
