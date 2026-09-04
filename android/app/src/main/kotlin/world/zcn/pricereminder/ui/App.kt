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
import androidx.compose.material.icons.outlined.Edit
import androidx.compose.material.icons.outlined.NotificationsActive
import androidx.compose.material.icons.outlined.Search
import androidx.compose.material.icons.outlined.Settings
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.AlertDialog
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
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableLongStateOf
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
import world.zcn.pricereminder.data.StoredEntryPrice
import world.zcn.pricereminder.data.StoredRule
import world.zcn.pricereminder.data.StoredMarketRule
import world.zcn.pricereminder.core.ContractOrdering
import world.zcn.pricereminder.core.EntryPriceCalculator
import world.zcn.pricereminder.core.EntryPriceDirection
import world.zcn.pricereminder.core.PositionSide
import world.zcn.pricereminder.monitor.MonitorBus
import kotlinx.coroutines.delay
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
                0 -> MarketScreen(state, monitor, viewModel, onStartMonitor, onStopMonitor)
                1 -> RulesScreen(state, viewModel)
                else -> SettingsScreen(state, monitor, viewModel)
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
    viewModel: MainViewModel,
    onStart: () -> Unit,
    onStop: () -> Unit,
) {
    val current = monitor.prices[state.primarySymbol]
    var now by remember { mutableLongStateOf(System.currentTimeMillis()) }
    LaunchedEffect(current?.eventTime) {
        while (true) {
            now = System.currentTimeMillis()
            delay(1_000)
        }
    }
    val currentIsStale = state.primarySymbol in monitor.staleSymbols
        || (current != null && EntryPriceCalculator.isStale(current.eventTime, now))
    var editingEntryPrice by remember(state.primarySymbol) { mutableStateOf(false) }
    Column(Modifier.fillMaxSize().padding(horizontal = 20.dp, vertical = 18.dp)) {
        Text("实时价格", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(18.dp))
        ContractPicker(state.contracts, state.recentSymbols, state.primarySymbol, "主合约", viewModel::selectPrimary)
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
        Spacer(Modifier.height(14.dp))
        EntryPriceSummary(
            entryPrice = state.entryPrices[state.primarySymbol],
            currentPrice = current?.price,
            stale = currentIsStale,
            onEdit = { editingEntryPrice = true },
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
    if (editingEntryPrice) {
        EntryPriceEditor(
            symbol = state.primarySymbol,
            savedEntry = state.entryPrices[state.primarySymbol],
            canUseCurrent = current != null && !currentIsStale,
            onUseCurrent = { viewModel.useCurrentPriceAsEntry(state.primarySymbol, it) },
            onSave = { price, side -> viewModel.setEntryPrice(state.primarySymbol, price, side) },
            onClear = { viewModel.clearEntryPrice(state.primarySymbol) },
            onDismiss = { editingEntryPrice = false },
        )
    }
}

@Composable
private fun EntryPriceSummary(
    entryPrice: StoredEntryPrice?, currentPrice: String?, stale: Boolean, onEdit: () -> Unit,
) {
    TextButton(onClick = onEdit, contentPadding = androidx.compose.foundation.layout.PaddingValues(0.dp)) {
        if (entryPrice == null) {
            Text("设置开仓价")
        } else {
            val positionSide = PositionSide.valueOf(entryPrice.positionSide.uppercase())
            val sideText = if (positionSide == PositionSide.LONG) "多" else "空"
            Text("$sideText · 开仓 ${entryPrice.priceText}", color = MaterialTheme.colorScheme.onSurfaceVariant)
            if (currentPrice != null) {
                val change = EntryPriceCalculator.change(currentPrice, entryPrice.priceText, positionSide)
                Spacer(Modifier.width(8.dp))
                Text(
                    if (stale) "${change.percentageText} · 价格已陈旧" else change.percentageText,
                    color = if (stale) MaterialTheme.colorScheme.onSurfaceVariant else when (change.direction) {
                        EntryPriceDirection.RISE -> RiseColor
                        EntryPriceDirection.FALL -> FallColor
                        EntryPriceDirection.FLAT -> MaterialTheme.colorScheme.onSurfaceVariant
                    },
                )
            }
        }
        Spacer(Modifier.width(7.dp))
        Icon(Icons.Outlined.Edit, contentDescription = "编辑开仓价", modifier = Modifier.width(16.dp))
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun EntryPriceEditor(
    symbol: String,
    savedEntry: StoredEntryPrice?,
    canUseCurrent: Boolean,
    onUseCurrent: (PositionSide) -> String?,
    onSave: (String, PositionSide) -> String?,
    onClear: () -> Unit,
    onDismiss: () -> Unit,
) {
    var priceText by remember(symbol, savedEntry) { mutableStateOf(savedEntry?.priceText ?: "") }
    var positionSide by remember(symbol, savedEntry) {
        mutableStateOf(
            savedEntry?.let { PositionSide.valueOf(it.positionSide.uppercase()) } ?: PositionSide.LONG
        )
    }
    var error by remember(symbol) { mutableStateOf<String?>(null) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("$symbol 开仓参考价") },
        text = {
            Column {
                SingleChoiceSegmentedButtonRow(Modifier.fillMaxWidth()) {
                    listOf(PositionSide.LONG to "多单", PositionSide.SHORT to "空单")
                        .forEachIndexed { index, item ->
                            SegmentedButton(
                                selected = positionSide == item.first,
                                onClick = { positionSide = item.first },
                                shape = SegmentedButtonDefaults.itemShape(index, 2),
                            ) { Text(item.second) }
                        }
                }
                Spacer(Modifier.height(12.dp))
                OutlinedTextField(
                    value = priceText,
                    onValueChange = { priceText = it.filter { char -> char.isDigit() || char == '.' } },
                    label = { Text("开仓价格") },
                    singleLine = true,
                    modifier = Modifier.fillMaxWidth(),
                )
                TextButton(
                    onClick = {
                        error = onUseCurrent(positionSide)
                        if (error == null) onDismiss()
                    },
                    enabled = canUseCurrent,
                ) { Text("使用当前价") }
                Text(
                    "按所选多空方向计算价格收益率，不包含杠杆和仓位数量。",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall) }
                if (savedEntry != null) {
                    TextButton(onClick = { onClear(); onDismiss() }) {
                        Text("清除开仓价", color = MaterialTheme.colorScheme.error)
                    }
                }
            }
        },
        confirmButton = {
            TextButton(onClick = {
                error = onSave(priceText, positionSide)
                if (error == null) onDismiss()
            }) { Text("保存") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("取消") } },
    )
}

@Composable
private fun RulesScreen(state: AppUiState, viewModel: MainViewModel) {
    var adding by remember { mutableStateOf(false) }
    var editingRule by remember { mutableStateOf<StoredRule?>(null) }
    var editingMarketRule by remember { mutableStateOf<StoredMarketRule?>(null) }
    LazyColumn(Modifier.fillMaxSize(), contentPadding = androidx.compose.foundation.layout.PaddingValues(20.dp)) {
        item {
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
                Column {
                    Text("价格预警", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
                    Text("单合约、全市场与目标价格 · ${state.rules.size + state.marketRules.size}/50", color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Button(onClick = {
                    if (adding) {
                        adding = false
                        editingRule = null
                        editingMarketRule = null
                    } else {
                        adding = true
                        editingRule = null
                        editingMarketRule = null
                    }
                }, shape = RoundedCornerShape(8.dp)) {
                    Icon(Icons.Outlined.AddAlert, contentDescription = null)
                    Spacer(Modifier.width(6.dp))
                    Text(if (adding) "收起" else "添加")
                }
            }
            if (adding) AddRuleForm(
                contracts = state.contracts,
                recentSymbols = state.recentSymbols,
                initialSymbol = state.primarySymbol,
                initialRule = editingRule,
                initialMarketRule = editingMarketRule,
                onSavePercentage = viewModel::addRule,
                onSaveMarket = viewModel::addMarketRule,
                onSaveTarget = viewModel::addTargetRule,
                onUpdate = viewModel::updateRule,
                onUpdateMarket = viewModel::updateMarketRule,
                onSelectSymbol = viewModel::recordRecentSymbol,
                onSaved = { adding = false; editingRule = null; editingMarketRule = null },
            )
            Spacer(Modifier.height(18.dp))
        }
        if (state.rules.isEmpty() && state.marketRules.isEmpty()) {
            item { EmptyMessage("还没有预警规则", "可添加单合约、全市场或目标价格提醒。") }
        }
        items(state.rules, key = { it.id }) { rule ->
            Row(Modifier.fillMaxWidth().padding(vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text(rule.symbol, fontWeight = FontWeight.SemiBold)
                    Text(
                        if (rule.kind == "target") {
                            "${if (rule.targetDirection == "above") "达到或高于" else "达到或低于"} ${rule.targetPriceText ?: "--"}"
                        } else "${rule.windowMinutes} 分钟内上涨或下跌 ≥ ${rule.thresholdText}%",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(checked = rule.enabled, onCheckedChange = { viewModel.toggleRule(rule.id, it) })
                IconButton(onClick = { editingRule = rule; editingMarketRule = null; adding = true }) {
                    Icon(Icons.Outlined.Edit, contentDescription = "编辑规则")
                }
                IconButton(onClick = { viewModel.deleteRule(rule.id) }) { Icon(Icons.Outlined.Delete, contentDescription = "删除规则") }
            }
            HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.45f))
        }
        items(state.marketRules, key = { "market:${it.id}" }) { rule ->
            Row(Modifier.fillMaxWidth().padding(vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
                Column(Modifier.weight(1f)) {
                    Text("全部 USDT 永续", fontWeight = FontWeight.SemiBold)
                    Text(
                        "${rule.windowMinutes} 分钟内上涨或下跌 ≥ ${rule.thresholdText}%",
                        style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                Switch(checked = rule.enabled, onCheckedChange = { viewModel.toggleMarketRule(rule.id, it) })
                IconButton(onClick = { editingMarketRule = rule; editingRule = null; adding = true }) {
                    Icon(Icons.Outlined.Edit, contentDescription = "编辑全市场规则")
                }
                IconButton(onClick = { viewModel.deleteMarketRule(rule.id) }) {
                    Icon(Icons.Outlined.Delete, contentDescription = "删除全市场规则")
                }
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
                    Text(if (event.kind == "target") "${event.symbol} · 目标价" else "${event.symbol} · ${event.windowMinutes}分钟", fontWeight = FontWeight.Medium)
                    Text(
                        if (event.kind == "target") {
                            "${if (rise) "达到或高于" else "达到或低于"} ${event.targetPriceText ?: "--"} · ${event.priceText}"
                        } else "${event.changePercent?.toBigDecimalOrNull()?.setScale(2, RoundingMode.HALF_UP)}% · ${event.priceText}",
                        style = MaterialTheme.typography.bodySmall,
                    )
                }
                Text(DateFormat.getDateTimeInstance(DateFormat.SHORT, DateFormat.SHORT).format(Date(event.eventTime)), style = MaterialTheme.typography.labelSmall)
            }
        }
    }
}

@Composable
private fun AddRuleForm(
    contracts: List<ContractDto>,
    recentSymbols: List<String>,
    initialSymbol: String,
    initialRule: StoredRule?,
    initialMarketRule: StoredMarketRule?,
    onSavePercentage: (String, Int, String) -> String?,
    onSaveMarket: (Int, String) -> String?,
    onSaveTarget: (String, String, String) -> String?,
    onUpdate: (String, String, String, Int, String, String, String) -> String?,
    onUpdateMarket: (String, Int, String) -> String?,
    onSelectSymbol: (String) -> Unit,
    onSaved: () -> Unit,
) {
    val editorKey = initialRule?.id ?: initialMarketRule?.id
    var symbol by remember(editorKey) { mutableStateOf(initialRule?.symbol ?: initialSymbol) }
    var kind by remember(editorKey) { mutableStateOf(if (initialMarketRule != null) "market_percentage" else initialRule?.kind ?: "percentage") }
    var window by remember(editorKey) { mutableStateOf((initialMarketRule?.windowMinutes ?: initialRule?.windowMinutes?.takeIf { it > 0 } ?: 5).toString()) }
    var threshold by remember(editorKey) { mutableStateOf(initialMarketRule?.thresholdText ?: initialRule?.thresholdText ?: "3") }
    var targetDirection by remember(editorKey) { mutableStateOf(initialRule?.targetDirection ?: "above") }
    var targetPrice by remember(editorKey) { mutableStateOf(initialRule?.targetPriceText ?: "") }
    var error by remember(editorKey) { mutableStateOf<String?>(null) }
    Column(Modifier.fillMaxWidth().padding(top = 18.dp, bottom = 6.dp)) {
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            if (initialMarketRule != null) {
                TextButton(onClick = {}) { Text("✓ 全市场") }
            } else {
                TextButton(onClick = { kind = "percentage" }) { Text(if (kind == "percentage") "✓ 涨跌幅" else "涨跌幅") }
                if (initialRule == null) {
                    TextButton(onClick = { kind = "market_percentage" }) { Text(if (kind == "market_percentage") "✓ 全市场" else "全市场") }
                }
                TextButton(onClick = { kind = "target" }) { Text(if (kind == "target") "✓ 目标价格" else "目标价格") }
            }
        }
        if (kind == "market_percentage") {
            Text("范围：币安全部 USDT 永续", color = MaterialTheme.colorScheme.onSurfaceVariant)
        } else {
            ContractPicker(contracts, recentSymbols, symbol, "U本位永续合约") {
                symbol = it
                onSelectSymbol(it)
            }
        }
        Spacer(Modifier.height(10.dp))
        if (kind != "target") {
            Row(horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                OutlinedTextField(window, { window = it.filter(Char::isDigit) }, label = { Text("分钟") }, modifier = Modifier.weight(1f), singleLine = true, shape = RoundedCornerShape(8.dp))
                OutlinedTextField(threshold, { threshold = it.filter { char -> char.isDigit() || char == '.' } }, label = { Text("变化 %") }, modifier = Modifier.weight(1f), singleLine = true, shape = RoundedCornerShape(8.dp))
            }
        } else {
            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                TextButton(onClick = { targetDirection = "above" }) { Text(if (targetDirection == "above") "✓ 达到或高于" else "达到或高于") }
                TextButton(onClick = { targetDirection = "below" }) { Text(if (targetDirection == "below") "✓ 达到或低于" else "达到或低于") }
            }
            OutlinedTextField(
                targetPrice, { targetPrice = it.filter { char -> char.isDigit() || char == '.' } },
                label = { Text("目标价格") }, modifier = Modifier.fillMaxWidth(), singleLine = true,
                shape = RoundedCornerShape(8.dp),
            )
        }
        error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 6.dp)) }
        Button(
            onClick = {
                error = if (initialMarketRule != null) {
                    onUpdateMarket(initialMarketRule.id, window.toIntOrNull() ?: 0, threshold)
                } else if (initialRule != null) {
                    onUpdate(initialRule.id, symbol, kind, window.toIntOrNull() ?: 0, threshold, targetDirection, targetPrice)
                } else if (kind == "market_percentage") {
                    onSaveMarket(window.toIntOrNull() ?: 0, threshold)
                } else if (kind == "percentage") onSavePercentage(symbol, window.toIntOrNull() ?: 0, threshold)
                else onSaveTarget(symbol, targetDirection, targetPrice)
                if (error == null) onSaved()
            },
            modifier = Modifier.fillMaxWidth().padding(top = 12.dp), shape = RoundedCornerShape(8.dp),
        ) { Text(if (initialRule == null && initialMarketRule == null) "保存预警" else "保存修改") }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun ContractPicker(
    contracts: List<ContractDto>, recentSymbols: List<String>, selected: String,
    label: String, onSelect: (String) -> Unit,
) {
    var expanded by remember { mutableStateOf(false) }
    var query by remember { mutableStateOf("") }
    val bySymbol = contracts.associateBy { it.symbol }
    val matches = ContractOrdering.orderedSymbols(contracts.map { it.symbol }, recentSymbols, query)
        .take(50).mapNotNull(bySymbol::get)
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
private fun SettingsScreen(
    state: AppUiState,
    monitor: world.zcn.pricereminder.monitor.MonitorSnapshot,
    viewModel: MainViewModel,
) {
    Column(Modifier.fillMaxSize().verticalScroll(rememberScrollState()).padding(20.dp)) {
        Text("设置", style = MaterialTheme.typography.headlineMedium, fontWeight = FontWeight.SemiBold)
        Spacer(Modifier.height(22.dp))
        SettingRow("行情来源", "币安 U 本位永续 · 最新成交价")
        SettingRow("价格保留", "本机与服务器均为最近 1 小时")
        SettingRow("系统展示", "常驻通知最多每 15 秒刷新")
        SettingRow("设备身份", "匿名设备令牌 · 连续 30 天未使用过期")
        SettingRow("当前状态", monitor.message)
        SettingRow("连接路径", monitor.source)
        SettingRow("订阅合约", "${monitor.subscribedCount} 个")
        SettingRow("行情延迟", latestDelayText(monitor))
        SettingRow("最后接收", monitor.lastReceivedAt?.let { DateFormat.getTimeInstance(DateFormat.MEDIUM).format(Date(it)) } ?: "尚未收到")
        SettingRow("重连次数", monitor.reconnectCount.toString())
        SettingRow("通知权限", if (monitor.notificationsEnabled) "已允许" else "未允许")
        SettingRow("全市场扫描", monitor.marketMessage)
        SettingRow("全市场覆盖", "${monitor.marketContractCount} 个合约")
        SettingRow("全市场最后接收", monitor.lastMarketReceivedAt?.let {
            DateFormat.getTimeInstance(DateFormat.MEDIUM).format(Date(it))
        } ?: "尚未收到")
        monitor.lastError?.let { SettingRow("最近错误", it) }
        Spacer(Modifier.height(18.dp))
        Text("连接自检", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.SemiBold)
        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
            Button(onClick = viewModel::refreshSelfCheck, shape = RoundedCornerShape(8.dp)) { Text("刷新状态") }
            TextButton(onClick = viewModel::sendTestNotification) { Text("发送测试通知") }
        }
        state.selfCheckMessage?.let {
            Text(it, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Spacer(Modifier.height(18.dp))
        Text("规则和触发历史只保存在此设备。卸载重装会被视为新设备，无法恢复旧配置。", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}

private fun latestDelayText(monitor: world.zcn.pricereminder.monitor.MonitorSnapshot): String {
    val latest = monitor.prices.values.maxOfOrNull { it.eventTime } ?: return "尚未收到行情"
    val delay = (System.currentTimeMillis() - latest).coerceAtLeast(0)
    return if (delay < 1_000) "< 1 秒" else "%.1f 秒".format(delay / 1_000.0)
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
