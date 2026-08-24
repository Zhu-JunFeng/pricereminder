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
import world.zcn.pricereminder.data.TriggerHistory
import java.util.UUID

data class AppUiState(
    val contracts: List<ContractDto> = emptyList(),
    val rules: List<StoredRule> = emptyList(),
    val history: List<TriggerHistory> = emptyList(),
    val primarySymbol: String = "BTCUSDT",
    val loading: Boolean = true,
    val error: String? = null,
)

class MainViewModel(application: Application) : AndroidViewModel(application) {
    private val container = (application as PriceReminderApplication).container
    private val mutableState = MutableStateFlow(
        AppUiState(
            rules = container.localStore.rules(), history = container.localStore.history(),
            primarySymbol = container.localStore.primarySymbol,
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
        mutableState.value = mutableState.value.copy(primarySymbol = symbol)
        syncSubscriptions()
    }

    fun addRule(symbol: String, window: Int, threshold: String): String? {
        val normalized = threshold.trim()
        if (window !in 1..60) return "时间窗口必须在 1 到 60 分钟之间"
        val value = normalized.toBigDecimalOrNull() ?: return "请输入有效百分比"
        if (value < "0.1".toBigDecimal() || value > "100".toBigDecimal()) return "变化阈值必须在 0.1% 到 100% 之间"
        val current = mutableState.value.rules
        if (current.size >= 50) return "每台设备最多 50 条规则"
        if (current.any { it.symbol == symbol && it.windowMinutes == window && it.thresholdText.toBigDecimalOrNull() == value }) {
            return "相同合约、窗口和阈值的规则已存在"
        }
        val updated = current + StoredRule(UUID.randomUUID().toString(), symbol, window, normalized)
        saveRules(updated)
        return null
    }

    fun toggleRule(id: String, enabled: Boolean) {
        saveRules(mutableState.value.rules.map { if (it.id == id) it.copy(enabled = enabled, riseTriggered = false, fallTriggered = false) else it })
    }

    fun deleteRule(id: String) { saveRules(mutableState.value.rules.filterNot { it.id == id }) }

    fun markNotificationPermissionUnavailable() {
        mutableState.value = mutableState.value.copy(error = "系统通知未开启；实时价格和本地规则计算仍会继续")
    }

    private fun saveRules(rules: List<StoredRule>) {
        container.localStore.saveRules(rules)
        mutableState.value = mutableState.value.copy(rules = rules)
        syncSubscriptions()
    }

    private fun syncSubscriptions() {
        // The foreground monitor reads local configuration and updates Binance subscriptions.
    }
}
