package world.zcn.pricereminder.ui

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.outlined.ShowChart
import androidx.compose.material.icons.outlined.AddAlert
import androidx.compose.material.icons.outlined.Delete
import androidx.compose.material.icons.outlined.NotificationsActive
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.ExposedDropdownMenuBox
import androidx.compose.material3.ExposedDropdownMenuDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.MenuAnchorType
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import world.zcn.pricereminder.AppUiState
import world.zcn.pricereminder.MainViewModel
import world.zcn.pricereminder.data.ContractDto
import world.zcn.pricereminder.monitor.MonitorBus
import java.math.RoundingMode
import java.text.DateFormat
import java.util.Date

@Composable
fun PriceReminderApp(viewModel: MainViewModel, onStartMonitor: () -> Unit, onStopMonitor: () -> Unit) {
    val state by viewModel.state.collectAsStateWithLifecycle()
    val monitor by MonitorBus.state.collectAsStateWithLifecycle()
    var tab by remember { mutableIntStateOf(0) }

    Scaffold(
        bottomBar = {
            NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                listOf("行情" to Icons.AutoMirrored.Outlined.ShowChart, "预警" to Icons.Outlined.NotificationsActive, "设置" to Icons.Outlined.Settings).forEachIndexed { index, item ->
                    NavigationBarItem(
                        selected = tab == index, onClick = { tab = index },
                        icon = { Icon(item.second, contentDescription = null) }, label = { Text(item.first) },
                    )
                }
            }
        }
    ) { padding ->
        Column(Modifier.fillMaxSize().padding(padding)) {
            state.error?.let { StatusBanner(it, MaterialTheme.colorScheme.error) }
            when (tab) {
                0 -> MarketScreen(state, monitor, viewModel::selectPrimary, onStartMonitor, onStopMonitor)
                1 -> RulesScreen(state, viewModel)
                else -> SettingsScreen(state, monitor.message)
            }
        }
    }
}

@Composable
private fun StatusBanner(message: String, color: Color) {
    Surface(color = color.copy(alpha = 0.1f)) {
        Text(message, color = color, style = MaterialTheme.typography.bodyMedium, modifier = Modifier.fillMaxWidth().padding(12.dp))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun MarketScreen(
    state: AppUiState,
    monitor: world.zcn.pricereminder.monitor.MonitorSnapshot,
    onSelect: (String) -> Unit,
    onStart: () -> Unit,
    onStop: () -> Unit,
) {
    val current = monitor.prices[state.primarySymbol]
    val currentIsStale = state.primarySymbol in monitor.staleSymbols
    Column(Modifier.fillMaxSize().padding(horizontal = 20.dp, vertical = 18.dp)) {
        Text("实时价格", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(18.dp))
        ContractPicker(state.contracts, state.primarySymbol, "主合约", onSelect)
        Spacer(Modifier.height(28.dp))
        Text(state.primarySymbol, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        Text(
            current?.price ?: "--",
            style = MaterialTheme.typography.displaySmall.copy(fontFeatureSettings = "tnum"),
            fontWeight = FontWeight.SemiBold,
        )
        Text(
            if (current == null) "等待第一条价格" else "币安最新成交价 · ${DateFormat.getTimeInstance(DateFormat.MEDIUM).format(Date(current.eventTime))}",
            style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(28.dp))
        HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.6f))
        Row(Modifier.fillMaxWidth().padding(vertical = 16.dp), verticalAlignment = Alignment.CenterVertically) {
            Column(Modifier.weight(1f)) {
                Text(
                    when {
                        !monitor.connected -> "监控未连接"
                        currentIsStale -> "价格已陈旧"
                        monitor.warmingUp -> "数据积累中"
                        else -> "实时监控中"
                    },
                    fontWeight = FontWeight.SemiBold,
                )
                Text(monitor.message, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (monitor.running) TextButton(onClick = onStop) { Text("停止") }
            else Button(onClick = onStart, shape = RoundedCornerShape(8.dp)) { Text("开始监控") }
        }
        if (state.loading) Box(Modifier.fillMaxWidth().padding(24.dp), contentAlignment = Alignment.Center) { CircularProgressIndicator() }
    }
}

@Composable
private fun RulesScreen(state: AppUiState, viewModel: MainViewModel) {
    var adding by remember { mutableStateOf(false) }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(20.dp)) {
        item {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                Column {
                    Text("价格预警", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
                    Text("滚动窗口双向提醒 · ${state.rules.size}/50", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Button(onClick = { adding = !adding }, shape = RoundedCornerShape(8.dp)) {
                    Icon(Icons.Outlined.AddAlert, contentDescription = null)
                    Spacer(Modifier.width(6.dp))
                    Text(if (adding) "收起" else "添加")
                }
            }
            if (adding) AddRuleForm(state.contracts, state.primarySymbol) { symbol, window, threshold ->
                viewModel.addRule(symbol, window, threshold)
            }
            Spacer(Modifier.height(18.dp))
        }
        if (state.rules.isEmpty()) {
            item { EmptyMessage("还没有预警规则", "添加一条规则后，监控服务会开始积累完整时间窗口。") }
        }
        items(state.rules, key = { it.id }) { rule ->
            Row(Modifier.fillMaxWidth().padding(vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(rule.symbol, fontWeight = FontWeight.SemiBold)
                    Text("${rule.windowMinutes} 分钟内上涨或下跌 ≥ ${rule.thresholdText}%", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Switch(checked = rule.enabled, onCheckedChange = { viewModel.toggleRule(rule.id, it) })
                IconButton(onClick = { viewModel.deleteRule(rule.id) }) { Icon(Icons.Outlined.Delete, contentDescription = "删除规则") }
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.45f))
        }
        item {
            Spacer(Modifier.height(24.dp))
            Text("最近触发", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
            Spacer(Modifier.height(8.dp))
            if (state.history.isEmpty()) EmptyMessage("暂无触发记录", "触发历史只保存在本机，并在 30 天后自动清理。")
        }
        items(state.history.take(30), key = { it.id }) { event ->
            val rise = event.direction == "rise"
            Row(Modifier.fillMaxWidth().padding(vertical = 10.dp), verticalAlignment = Alignment.CenterVertically) {
                Text(if (rise) "↑" else "↓", color = if (rise) RiseColor else FallColor, style = MaterialTheme.typography.titleLarge)
                Spacer(Modifier.width(10.dp))
                Column(Modifier.weight(1f)) {
                    Text("${event.symbol} · ${event.windowMinutes}分钟", fontWeight = FontWeight.Medium)
                    Text("${event.changePercent.toBigDecimalOrNull()?.setScale(2, RoundingMode.HALF_UP)}% · ${event.priceText}", style = MaterialTheme.typography.bodySmall)
                }
                Text(DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(event.eventTime)), style = MaterialTheme.typography.labelSmall)
            }
        }
    }
}

@Composable
private fun AddRuleForm(contracts: List<ContractDto>, initialSymbol: String, onSave: (String, Int, String) -> String?) {
    var symbol by remember { mutableStateOf(initialSymbol) }
    var window by remember { mutableStateOf("5") }
    var threshold by remember { mutableStateOf("3") }
    var error by remember { mutableStateOf<String?>(null) }
    Column(Modifier.fillMaxWidth().padding(top = 18.dp, bottom = 6.dp)) {
        ContractPicker(contracts, symbol, "U本位永续合约") { symbol = it }
        Spacer(Modifier.height(10.dp))
        Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
            OutlinedTextField(window, { window = it.filter(Char::isDigit) }, label = { Text("分钟") }, modifier = Modifier.weight(1f), singleLine = true, shape = RoundedCornerShape(8.dp))
            OutlinedTextField(threshold, { threshold = it.filter { char -> char.isDigit() || char == '.' } }, label = { Text("变化 %") }, modifier = Modifier.weight(1f), singleLine = true, shape = RoundedCornerShape(8.dp))
        }
        error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 6.dp)) }
        Button(
            onClick = { error = onSave(symbol, window.toIntOrNull() ?: 0, threshold) },
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp), shape = RoundedCornerShape(8.dp),
        ) { Text("保存预警") }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ContractPicker(contracts: List<ContractDto>, selected: String, label: String, onSelect: (String) -> Unit) {
    var expanded by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    val matches = if (query.isBlank()) contracts.take(50) else contracts.filter {
        it.symbol.contains(query.trim(), ignoreCase = true)
    }.take(50)
    ExposedDropdownMenuBox(expanded, {
        expanded = it
        if (it) query = ""
    }) {
        OutlinedTextField(
            value = if (expanded) query else selected,
            onValueChange = { query = it; expanded = true },
            label = { Text(label) },
            placeholder = { Text("输入合约代码搜索") },
            leadingIcon = { Icon(Icons.Outlined.Search, contentDescription = null) },
            trailingIcon = { ExposedDropdownMenuDefaults.TrailingIcon(expanded) },
            modifier = Modifier.menuAnchor(MenuAnchorType.PrimaryEditable).fillMaxWidth(),
            singleLine = true,
            shape = RoundedCornerShape(8.dp),
        )
        ExposedDropdownMenu(expanded, { expanded = false }) {
            matches.forEach { item ->
                DropdownMenuItem(
                    text = { Text(item.symbol) },
                    onClick = { onSelect(item.symbol); expanded = false; query = "" },
                )
            }
        }
    }
}

@Composable
private fun SettingsScreen(state: AppUiState, monitorMessage: String) {
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp)) {
        Text("设置", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(22.dp))
        SettingRow("行情来源", "币安 U 本位永续 · 最新成交价")
        SettingRow("价格保留", "本机与服务器均为最近 1 小时")
        SettingRow("系统展示", "常驻通知最多每 15 秒刷新")
        SettingRow("设备身份", "匿名设备令牌 · 连续 30 天未使用过期")
        SettingRow("当前状态", monitorMessage)
        Spacer(Modifier.height(18.dp))
        Text("规则和触发历史只保存在此设备。卸载重装会被视为新设备，无法恢复旧配置。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

@Composable
private fun SettingRow(label: String, value: String) {
    Row(Modifier.fillMaxWidth().padding(vertical = 14.dp), verticalAlignment = Alignment.Top) {
        Text(label, modifier = Modifier.width(92.dp), fontWeight = FontWeight.Medium)
        Text(value, modifier = Modifier.weight(1f), color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.45f))
}

@Composable
private fun EmptyMessage(title: String, detail: String) {
    Column(Modifier.fillMaxWidth().padding(vertical = 22.dp)) {
        Text(title, fontWeight = FontWeight.SemiBold)
        Text(detail, color = MaterialTheme.colorScheme.onSurfaceVariant, style = MaterialTheme.typography.bodySmall, maxLines = 3, overflow = TextOverflow.Ellipsis)
    }
}
