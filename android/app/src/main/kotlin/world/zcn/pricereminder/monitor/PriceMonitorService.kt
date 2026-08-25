package world.zcn.pricereminder.monitor

import android.app.Service
import android.content.Intent
import android.os.IBinder
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import world.zcn.pricereminder.PriceReminderApplication
import world.zcn.pricereminder.core.AlertRule
import world.zcn.pricereminder.core.AlertRuleKind
import world.zcn.pricereminder.core.PriceBuffer
import world.zcn.pricereminder.core.PricePoint
import world.zcn.pricereminder.core.RuleEngine
import world.zcn.pricereminder.core.TargetDirection
import world.zcn.pricereminder.data.CombinedTradeDto
import world.zcn.pricereminder.data.RelayMessage
import world.zcn.pricereminder.data.TriggerHistory
import java.util.concurrent.ConcurrentHashMap
import kotlin.time.Duration.Companion.seconds

class PriceMonitorService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private lateinit var notifications: NotificationCoordinator
    private val buffer = PriceBuffer()
    private val latestEventTimes = ConcurrentHashMap<String, Long>()
    private val activeRules = ConcurrentHashMap<String, AlertRule>()
    private val pendingPrices = ConcurrentHashMap<String, PricePoint>()
    private var subscribedSymbols = emptySet<String>()
    private var socket: WebSocket? = null
    private var reconnectJob: Job? = null
    private var ready = false
    private var lastMessageAt = 0L
    private var connectionStartedAt = 0L
    private var lastSurfaceUpdateAt = 0L
    private var connectionSource = ConnectionSource.DIRECT
    private var serverConnectedAt = 0L
    private var reconnectCount = 0
    private var lastError: String? = null
    private val lastPersistedEventTimes = ConcurrentHashMap<String, Long>()

    private val container get() = (application as PriceReminderApplication).container
    override fun onCreate() {
        super.onCreate()
        notifications = NotificationCoordinator(this)
        MonitorBus.update { it.copy(running = true, notificationsEnabled = notifications.notificationsEnabled()) }
        restorePrices()
        startForeground(
            NotificationCoordinator.MONITOR_ID,
            notifications.monitor(container.localStore.primarySymbol, "--", "正在连接币安行情")
        )
        scope.launch { watchdog() }
        scope.launch { flushPrices() }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        connectionSource = ConnectionSource.DIRECT
        connect()
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        persistAllPrices()
        reconnectJob?.cancel()
        socket?.close(1000, "service stopped")
        scope.cancel()
        MonitorBus.update { it.copy(running = false, connected = false, message = "监控已停止") }
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun connect() {
        socket?.cancel()
        reconnectJob?.cancel()
        ready = false
        lastMessageAt = 0L
        connectionStartedAt = System.currentTimeMillis()
        val source = connectionSource
        MonitorBus.update {
            it.copy(
                connected = false, warmingUp = true,
                message = if (source == ConnectionSource.DIRECT) "正在直连币安" else "正在连接服务端行情",
                source = if (source == ConnectionSource.DIRECT) "终端直连" else "服务端中继",
                subscribedCount = subscribedSymbols.size,
                notificationsEnabled = notifications.notificationsEnabled(),
            )
        }
        scope.launch {
            runCatching {
                refreshRules()
                val symbols = (activeRules.values.filter { it.enabled }.map { it.symbol } + container.localStore.primarySymbol).distinct().take(50)
                subscribedSymbols = symbols.toSet()
                MonitorBus.update { it.copy(subscribedCount = symbols.size) }
                val request = if (source == ConnectionSource.DIRECT) {
                    container.apiClient.streamRequest(symbols)
                } else {
                    container.apiClient.serverStreamRequest(symbols)
                }
                socket = container.apiClient.http.newWebSocket(request, listener(source))
            }.onFailure { scheduleReconnect(source, it.message ?: "连接失败") }
        }
    }

    private fun listener(source: ConnectionSource) = object : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
            if (webSocket !== socket) return
            if (source == ConnectionSource.SERVER) {
                serverConnectedAt = System.currentTimeMillis()
                webSocket.send(container.apiClient.resumeMessage(latestEventTimes))
            } else {
                ready = true
                updateReadiness()
            }
        }

        override fun onMessage(webSocket: WebSocket, text: String) {
            if (webSocket !== socket) return
            if (source == ConnectionSource.DIRECT) {
                val trade = runCatching { container.apiClient.json.decodeFromString<CombinedTradeDto>(text).data }.getOrNull() ?: return
                if (trade.eventType != "trade" || trade.price == "0" || trade.quantity == "0") return
                runCatching { PricePoint(trade.symbol, trade.price, trade.eventTime) }.getOrNull()
                    ?.let {
                        lastMessageAt = System.currentTimeMillis()
                        MonitorBus.update { state -> state.copy(lastReceivedAt = lastMessageAt, lastError = null) }
                        pendingPrices[it.symbol] = it
                    }
                return
            }
            val message = runCatching { container.apiClient.json.decodeFromString<RelayMessage>(text) }.getOrNull() ?: return
            when (message.type) {
                "ready" -> { ready = true; updateReadiness() }
                "status" -> MonitorBus.update { it.copy(warmingUp = true, message = "服务端币安行情 · 正在补齐价格") }
                "price" -> if (message.symbol != null && message.price != null && message.eventTime != null) {
                    runCatching { PricePoint(message.symbol, message.price, message.eventTime, message.replay) }.getOrNull()
                        ?.let {
                            lastMessageAt = System.currentTimeMillis()
                            MonitorBus.update { state -> state.copy(lastReceivedAt = lastMessageAt, lastError = null) }
                            pendingPrices[it.symbol] = it
                        }
                }
            }
        }

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
            if (webSocket === socket) scheduleReconnect(source, t.message ?: "行情连接中断")
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
            if (webSocket === socket && code != 1000) scheduleReconnect(source, reason.ifBlank { "行情连接关闭" })
        }
    }

    private fun process(point: PricePoint) {
            if (!buffer.add(point)) return
            latestEventTimes[point.symbol] = point.eventTime
            MonitorBus.update { state ->
                state.copy(
                    prices = state.prices + (point.symbol to LivePrice(point.symbol, point.priceText, point.eventTime)),
                    staleSymbols = state.staleSymbols - point.symbol,
                )
            }
            if (ready && !point.replay) evaluate(point)
            persistPriceIfDue(point)
            if (point.replay) {
                MonitorBus.update { it.copy(warmingUp = true, message = "服务端币安行情 · 正在补齐一小时价格") }
            } else if (ready) {
                updateReadiness()
            }
            updateSurface(point)
    }

    private suspend fun flushPrices() {
        while (true) {
            delay(1.seconds)
            val prices = pendingPrices.entries.map { it.key to it.value }.sortedBy { it.first }
            prices.forEach { (symbol, point) ->
                if (pendingPrices.remove(symbol, point)) process(point)
            }
        }
    }

    private fun evaluate(point: PricePoint) {
        refreshRules()
        var stateChanged = false
        val triggers = activeRules.values.filter { it.symbol == point.symbol }.flatMap { rule ->
            val before = rule.riseTriggered to rule.fallTriggered
            val result = RuleEngine.evaluate(rule, point, buffer)
            if (before != (rule.riseTriggered to rule.fallTriggered)) stateChanged = true
            result
        }
        if (stateChanged) persistRuleStates()
        if (triggers.isEmpty()) return
        notifications.alerts(point.symbol, triggers)
        container.localStore.appendHistory(triggers.map {
            TriggerHistory(
                id = "${it.ruleId}:${it.direction}:${it.eventTime}", symbol = it.symbol,
                direction = it.direction.name.lowercase(), kind = it.kind.name.lowercase(),
                changePercent = it.changePercent?.toPlainString(),
                windowMinutes = it.windowMinutes, thresholdText = it.thresholdText,
                targetPriceText = it.targetPriceText,
                priceText = it.priceText, eventTime = it.eventTime,
            )
        })
    }

    private fun refreshRules() {
        val stored = container.localStore.rules()
        val ids = stored.mapTo(mutableSetOf()) { it.id }
        activeRules.keys.removeAll { it !in ids }
        stored.forEach { item ->
            val existing = activeRules[item.id]
            if (existing == null || existing.symbol != item.symbol || existing.windowMinutes != item.windowMinutes ||
                existing.thresholdText != item.thresholdText || existing.enabled != item.enabled ||
                existing.kind.name.lowercase() != item.kind || existing.targetDirection?.name?.lowercase() != item.targetDirection ||
                existing.targetPriceText != item.targetPriceText) {
                val created = AlertRule(
                    item.id, item.symbol, item.windowMinutes, item.thresholdText,
                    item.enabled, item.riseTriggered, item.fallTriggered,
                    AlertRuleKind.valueOf(item.kind.uppercase()),
                    item.targetDirection?.let { TargetDirection.valueOf(it.uppercase()) },
                    item.targetPriceText, item.targetTriggered,
                )
                if (created.kind == AlertRuleKind.TARGET && created.enabled) {
                    buffer.latest(created.symbol)?.let { RuleEngine.initialize(created, it, buffer) }
                }
                activeRules[item.id] = created
            }
        }
    }

    private fun persistRuleStates() {
        val updated = container.localStore.rules().map { item ->
            activeRules[item.id]?.let {
                item.copy(riseTriggered = it.riseTriggered, fallTriggered = it.fallTriggered, targetTriggered = it.targetTriggered)
            } ?: item
        }
        container.localStore.saveRules(updated)
    }

    private fun restorePrices() {
        val restored = runCatching { container.localStore.loadPricePoints() }
            .onFailure { MonitorBus.update { state -> state.copy(message = "本地价格记录读取失败") } }
            .getOrDefault(emptyList())
        buffer.restore(restored)
        restored.groupBy { it.symbol }.forEach { (symbol, points) ->
            points.maxByOrNull { it.eventTime }?.let { latest ->
                latestEventTimes[symbol] = latest.eventTime
                lastPersistedEventTimes[symbol] = latest.eventTime
            }
        }
        val prices = buffer.symbols().mapNotNull { symbol ->
            buffer.latest(symbol)?.let { symbol to LivePrice(symbol, it.priceText, it.eventTime) }
        }.toMap()
        if (prices.isNotEmpty()) MonitorBus.update { it.copy(prices = prices) }
    }

    private fun persistPriceIfDue(point: PricePoint) {
        val last = lastPersistedEventTimes[point.symbol] ?: 0L
        if (point.eventTime - last < 15.seconds.inWholeMilliseconds) return
        runCatching { container.localStore.savePricePoints(point.symbol, buffer.points(point.symbol)) }
            .onSuccess { lastPersistedEventTimes[point.symbol] = point.eventTime }
            .onFailure { MonitorBus.update { state -> state.copy(message = "本地价格记录保存失败") } }
    }

    private fun persistAllPrices() {
        buffer.symbols().forEach { symbol ->
            runCatching { container.localStore.savePricePoints(symbol, buffer.points(symbol)) }
                .onSuccess { buffer.latest(symbol)?.let { lastPersistedEventTimes[symbol] = it.eventTime } }
        }
    }

    private fun updateReadiness() {
        refreshRules()
        val warming = activeRules.values.any { rule ->
            rule.enabled && rule.kind == AlertRuleKind.PERCENTAGE && !buffer.covers(rule.symbol, rule.windowMinutes * 60_000L)
        }
        MonitorBus.update {
            it.copy(
                connected = true, warmingUp = warming,
                message = if (warming) "${sourceLabel()} · 正在积累完整规则窗口" else "${sourceLabel()} · 实时监控中",
            )
        }
    }

    private fun updateSurface(point: PricePoint) {
        if (point.symbol != container.localStore.primarySymbol) return
        val now = System.currentTimeMillis()
        if (now - lastSurfaceUpdateAt < 15.seconds.inWholeMilliseconds) return
        lastSurfaceUpdateAt = now
        startForeground(NotificationCoordinator.MONITOR_ID, notifications.monitor(point.symbol, point.priceText, "实时监控中 · 刚刚更新"))
    }

    private fun scheduleReconnect(failedSource: ConnectionSource, reason: String) {
        if (reconnectJob?.isActive == true) return
        reconnectCount += 1
        lastError = reason
        ready = false
        connectionSource = if (failedSource == ConnectionSource.DIRECT) ConnectionSource.SERVER else ConnectionSource.DIRECT
        val message = if (failedSource == ConnectionSource.DIRECT) {
            "币安直连失败，正在切换服务端"
        } else {
            "服务端连接中断，5 秒后重新检测币安"
        }
        MonitorBus.update {
            it.copy(
                connected = false, warmingUp = true, message = "$message：$reason",
                reconnectCount = reconnectCount, lastError = reason,
                source = if (failedSource == ConnectionSource.DIRECT) "终端直连" else "服务端中继",
                notificationsEnabled = notifications.notificationsEnabled(),
            )
        }
        reconnectJob = scope.launch {
            if (failedSource == ConnectionSource.SERVER) delay(5.seconds)
            connect()
        }
    }

    private suspend fun watchdog() {
        var interruptionReported = false
        while (true) {
            delay(1.seconds)
            refreshRules()
            val desiredSymbols = (activeRules.values.filter { it.enabled }.map { it.symbol } + container.localStore.primarySymbol)
                .distinct().take(50).toSet()
            if (desiredSymbols != subscribedSymbols) {
                connect()
                continue
            }
            val now = System.currentTimeMillis()
            if (connectionSource == ConnectionSource.SERVER && serverConnectedAt > 0 && now - serverConnectedAt >= 5 * 60_000L) {
                serverConnectedAt = now
                connectionSource = ConnectionSource.DIRECT
                connect()
                continue
            }
            val silentFor = now - (lastMessageAt.takeIf { it > 0 } ?: connectionStartedAt)
            MonitorBus.update { state ->
                state.copy(
                    staleSymbols = state.prices.values
                        .filter { now - it.eventTime >= 30.seconds.inWholeMilliseconds }
                        .mapTo(mutableSetOf()) { it.symbol },
                )
            }
            if (connectionSource == ConnectionSource.DIRECT && silentFor >= 3.seconds.inWholeMilliseconds) {
                socket?.cancel()
                scheduleReconnect(ConnectionSource.DIRECT, "3 秒内未收到有效行情")
                continue
            }
            if (silentFor >= 60.seconds.inWholeMilliseconds && !interruptionReported) {
                interruptionReported = true
                notifications.health("价格监控已中断", "连续 60 秒未收到有效行情，正在重连")
                socket?.cancel()
                scheduleReconnect(connectionSource, "连续 60 秒未收到行情")
            } else if (interruptionReported && silentFor < 30.seconds.inWholeMilliseconds) {
                interruptionReported = false
                notifications.health("价格监控已恢复", "币安实时行情连接已经恢复")
            }
        }
    }

    private fun sourceLabel(): String =
        if (connectionSource == ConnectionSource.DIRECT) "直连币安" else "服务端币安行情"

    private enum class ConnectionSource { DIRECT, SERVER }
}
