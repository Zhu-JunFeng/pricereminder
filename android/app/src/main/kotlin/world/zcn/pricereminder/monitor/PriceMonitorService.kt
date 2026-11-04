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
import org.json.JSONObject
import org.json.JSONArray
import world.zcn.pricereminder.core.AlertTrigger
import world.zcn.pricereminder.core.TriggerDirection
import world.zcn.pricereminder.core.AlertRule
import world.zcn.pricereminder.core.AlertRuleKind
import world.zcn.pricereminder.core.MarketAlertRule
import world.zcn.pricereminder.core.MarketScanner
import world.zcn.pricereminder.core.PriceBuffer
import world.zcn.pricereminder.core.PricePoint
import world.zcn.pricereminder.core.RuleEngine
import world.zcn.pricereminder.core.TargetDirection
import world.zcn.pricereminder.data.GoldQuote
import world.zcn.pricereminder.data.CombinedTradeDto
import world.zcn.pricereminder.data.RelayMessage
import world.zcn.pricereminder.data.MarketMiniTickerDto
import world.zcn.pricereminder.data.TriggerHistory
import java.util.concurrent.ConcurrentHashMap
import kotlin.time.Duration.Companion.seconds

class PriceMonitorService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private lateinit var notifications: NotificationCoordinator
    private val buffer = PriceBuffer()
    private val latestEventTimes = ConcurrentHashMap<String, Long>()
    private val activeRules = ConcurrentHashMap<String, AlertRule>()
    private val marketScanner = MarketScanner()
    private val pendingPrices = ConcurrentHashMap<String, PricePoint>()
    private var subscribedSymbols = emptySet<String>()
    private var socket: WebSocket? = null
    private val tradeSockets = mutableSetOf<WebSocket>()
    private var generation = 0L
    private var connectionKey = ""
    private var relayVersion = 0L
    private var relaySignature = ""
    private val relayEvents = mutableSetOf<String>()
    private val initialSymbols = mutableSetOf<String>()
    private fun hasMarketRules() = container.localStore.marketRules().any { it.enabled }
    private fun desired() = (container.localStore.rules().filter { it.enabled }.map { it.symbol } + container.localStore.primarySymbol).distinct().filter { it != GoldQuote.SYMBOL }.sorted()

    private var marketSocket: WebSocket? = null
    private var marketRulesSignature = ""
    private var marketRuleKeys = mapOf<String,String>()
    private var validMarketSymbols = emptySet<String>()
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
        scope.launch { goldLoop() }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        connect()
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        generation++
        tradeSockets.forEach { it.cancel() }; tradeSockets.clear()
        persistAllPrices()
        reconnectJob?.cancel()
        socket?.close(1000, "service stopped")
        val closingMarketSocket = marketSocket
        marketSocket = null
        closingMarketSocket?.close(1000, "service stopped")
        scope.cancel()
        MonitorBus.update {
            it.copy(
                running = false, connected = false, message = "监控已停止",
                marketMessage = "全市场扫描已停止", marketContractCount = 0,
            )
        }
        super.onDestroy()
    }

    private suspend fun goldLoop() {
        var last = 0L
        var initialize = true
        buffer.remove(GoldQuote.SYMBOL)
        while (true) {
            try {
                val wanted = container.localStore.primarySymbol == GoldQuote.SYMBOL || container.localStore.rules().any { it.enabled && it.symbol == GoldQuote.SYMBOL }
                if (!wanted) { initialize = true; buffer.remove(GoldQuote.SYMBOL) }
                else {
                    val quote = container.apiClient.goldQuote()
                    val point = PricePoint(GoldQuote.SYMBOL,quote.price,quote.eventTime)
                    MonitorBus.update { it.copy(prices = it.prices + (point.symbol to LivePrice(point.symbol,point.priceText,point.eventTime)),goldChange = quote.dailyChange,
                        goldStatus = if (quote.eligible) "实时价格" else when (quote.state) { "closed" -> "黄金休市"; "calendar_unknown" -> "交易日历待更新，提醒已暂停"; else -> "报价已陈旧，提醒已暂停" }) }
                    if (subscribedSymbols.isEmpty()) MonitorBus.update { it.copy(connected = quote.eligible, warmingUp = false, message = it.goldStatus, source = "黄金服务端") }
                    if (!quote.eligible) { initialize = true; buffer.remove(point.symbol) }
                    else if (point.eventTime > last) {
                        buffer.add(point)
                        refreshRules()
                        if (initialize) {
                            activeRules.values.filter { it.symbol == point.symbol }.forEach { RuleEngine.initialize(it,point,buffer) }
                            initialize = false
                        } else evaluate(point)
                        last = point.eventTime
                    }
                }
            } catch (e: kotlinx.coroutines.CancellationException) { throw e }
            catch (e: Exception) { initialize = true; buffer.remove(GoldQuote.SYMBOL); MonitorBus.update { it.copy(goldStatus = "黄金行情连接失败，提醒已暂停") } }
            delay(5.seconds)
        }
    }

    private fun connectMarketIfNeeded() { connect() }

    private fun configuration(): JSONObject {
        val rules = JSONArray()
        container.localStore.marketRules().filter { it.enabled }.sortedBy { it.id }.forEach { r ->
            rules.put(JSONObject().put("id",r.id).put("windowMinutes",r.windowMinutes).put("thresholdText",r.thresholdText).put("isEnabled",true))
        }
        return JSONObject().put("type","configure").put("protocol",2).put("symbols",JSONArray(desired())).put("marketRules",rules)
    }
    private fun sendConfiguration() {
        val config=configuration()
        val signature=config.toString()
        if(signature==relaySignature)return
        relayVersion++
        if(socket?.send(config.put("version",relayVersion).toString())==true)relaySignature=signature
    }
    private fun recordMarket(triggers: List<AlertTrigger>) {
        triggers.groupBy { it.symbol to it.eventTime }.forEach { (key,items)->notifications.alerts(key.first,items) }
        if(triggers.isNotEmpty())container.localStore.appendHistory(triggers.map {
            TriggerHistory(id="${it.ruleId}:${it.direction}:${it.eventTime}",symbol=it.symbol,direction=it.direction.name.lowercase(),kind="market_percentage",changePercent=it.changePercent?.toPlainString(),windowMinutes=it.windowMinutes,thresholdText=it.thresholdText,targetPriceText=null,priceText=it.priceText,eventTime=it.eventTime)
        })
    }

    override fun onBind(intent: Intent?): IBinder? = null

    private fun connect() {
        refreshRules()
        val symbols=desired()
        if(symbols.size>50) {MonitorBus.update {it.copy(message="最多订阅 50 个普通合约")};return}
        initialSymbols.addAll(symbols.toSet()-subscribedSymbols)
        (symbols.toSet()-subscribedSymbols).forEach { buffer.remove(it) }
        subscribedSymbols=symbols.toSet()
        val key="$connectionSource:" + if(hasMarketRules())"all" else symbols.joinToString(",")
        if(key==connectionKey) {
            if(connectionSource==ConnectionSource.SERVER)sendConfiguration()
            return
        }
        connectionKey=key
        val epoch=++generation
        tradeSockets.forEach {it.cancel()};tradeSockets.clear();socket?.cancel();socket=null
        reconnectJob?.cancel()
        pendingPrices.clear();marketScanner.resetAll();initialSymbols.clear();initialSymbols.addAll(symbols)
        symbols.forEach {buffer.remove(it)}
        relaySignature="";ready=false;lastMessageAt=0L;connectionStartedAt=System.currentTimeMillis()
        val source=connectionSource
        MonitorBus.update {it.copy(connected=false,warmingUp=true,message="正在连接${if(source==ConnectionSource.DIRECT)"币安" else "服务器"}",source=sourceLabel(),marketMessage=if(hasMarketRules())"正在重新积累全市场窗口" else "全市场扫描未启用")}
        scope.launch {
            try {
                if(symbols.isEmpty()&&!hasMarketRules())return@launch
                val groups = if(source==ConnectionSource.DIRECT&&hasMarketRules()) {
                    validMarketSymbols=container.apiClient.binanceContracts().mapTo(mutableSetOf()){it.symbol}
                    validMarketSymbols.sorted().chunked(150)
                } else listOf(symbols)
                if(epoch!=generation)return@launch
                for(group in groups) {
                    val request=if(source==ConnectionSource.DIRECT)container.apiClient.streamRequest(group) else container.apiClient.serverStreamRequest(group)
                    if(epoch!=generation)return@launch
                    val ws=container.apiClient.http.newWebSocket(request,listener(source,epoch))
                    tradeSockets.add(ws);socket=ws
                }
            } catch(e:Exception) {if(epoch==generation){connectionKey="";scheduleReconnect(source,e.message?:"连接失败")}}
        }
    }

    private fun listener(source: ConnectionSource, epoch: Long) = object: WebSocketListener() {
        override fun onOpen(ws:WebSocket,response:Response) {scope.launch {
            if(epoch!=generation)return@launch
            lastMessageAt=System.currentTimeMillis()
            if(source==ConnectionSource.SERVER) {serverConnectedAt=lastMessageAt;sendConfiguration()}
            else {ready=true;updateReadiness()}
        }}
        override fun onMessage(ws:WebSocket,text:String) {scope.launch {
            if(epoch!=generation)return@launch
            try {
                lastMessageAt=System.currentTimeMillis()
                if(source==ConnectionSource.DIRECT) {
                    val trade=container.apiClient.json.decodeFromString<CombinedTradeDto>(text).data
                    if(trade.eventType!="trade"||trade.price=="0"||trade.quantity=="0")return@launch
                    val point=PricePoint(trade.symbol,trade.price,trade.eventTime)
                    val old=pendingPrices[point.symbol]
                    if(old==null||point.eventTime>old.eventTime)pendingPrices[point.symbol]=point
                } else {
                    val batch=JSONObject(text)
                    check(batch.getInt("protocol")==2){"服务器不支持行情协议 v2"}
                    if(batch.getLong("version")!=relayVersion)return@launch
                    val latest=batch.optLong("latestEventTime")
                    check(!hasMarketRules()||latest==0L||System.currentTimeMillis()-latest<=60000){"服务端行情停滞"}
                    ready=true
                    val prices=batch.optJSONArray("prices")?:JSONArray()
                    for(i in 0 until prices.length()) {val p=prices.getJSONObject(i);pendingPrices[p.getString("symbol")]=PricePoint(p.getString("symbol"),p.getString("price"),p.getLong("eventTime"))}
                    val event=batch.optString("eventId")
                    if(event.isNotEmpty()&&relayEvents.add(event)) {
                        val triggers=batch.optJSONArray("triggers")?:JSONArray()
                        val valid=container.localStore.marketRules().filter{it.enabled}.map{it.id}.toSet()
                        recordMarket((0 until triggers.length()).mapNotNull {i->val t=triggers.getJSONObject(i)
                            if(t.getString("ruleId") !in valid)null else AlertTrigger(t.getString("ruleId"),t.getString("symbol"),AlertRuleKind.MARKET_PERCENTAGE,if(t.getString("direction")=="rise")TriggerDirection.RISE else TriggerDirection.FALL,t.optString("changePct").toBigDecimalOrNull(),t.optString("thresholdPct"),t.getInt("windowMinutes"),null,t.getString("price"),t.getString("baselinePrice"),t.getLong("eventTime"))
                        })
                        if(relayEvents.size>2000){relayEvents.clear();relayEvents.add(event)}
                    }
                    MonitorBus.update {it.copy(marketMessage=if(hasMarketRules())"服务端全市场扫描中" else "全市场扫描未启用",lastReceivedAt=lastMessageAt)}
                    updateReadiness()
                }
            } catch(e:Exception) {connectionKey="";scheduleReconnect(source,e.message?:"行情协议错误")}
        }}
        override fun onFailure(ws:WebSocket,t:Throwable,response:Response?) {scope.launch {if(epoch==generation){connectionKey="";scheduleReconnect(source,t.message?:"连接中断")}}}
        override fun onClosed(ws:WebSocket,code:Int,reason:String) {scope.launch {if(epoch==generation){connectionKey="";scheduleReconnect(source,reason)}}}
    }

    private fun process(point: PricePoint) {
            if (!buffer.add(point)) return
            if(initialSymbols.remove(point.symbol)) activeRules.values.filter{it.symbol==point.symbol}.forEach{RuleEngine.initialize(it,point,buffer)}
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
                if (pendingPrices.remove(symbol, point)) {
                    if(connectionSource==ConnectionSource.DIRECT&&hasMarketRules()) {
                        val rules=container.localStore.marketRules().filter{it.enabled}.map{MarketAlertRule(it.id,it.windowMinutes,it.thresholdText,it.enabled)}
                        recordMarket(rules.flatMap{marketScanner.evaluate(it,point)})
                        MonitorBus.update {it.copy(marketMessage="本地全市场实时扫描中",marketContractCount=validMarketSymbols.size,lastMarketReceivedAt=System.currentTimeMillis())}
                    }
                    if(symbol in subscribedSymbols)process(point)
                }
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
        val nextKeys=container.localStore.marketRules().filter{it.enabled}.associate{it.id to "${it.windowMinutes}:${it.thresholdText}"}
        marketScanner.retainRules(nextKeys.keys.filter{nextKeys[it]==marketRuleKeys[it]}.toSet())
        marketRuleKeys=nextKeys
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
                if (created.enabled) {
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
        generation++
        connectionKey=""
        tradeSockets.forEach {it.cancel()};tradeSockets.clear();socket?.cancel();socket=null
        pendingPrices.clear()
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
            connectMarketIfNeeded()
            val desiredSymbols = (activeRules.values.filter { it.enabled }.map { it.symbol } + container.localStore.primarySymbol)
                .distinct().filter { it != GoldQuote.SYMBOL }.toSet()
            if (desiredSymbols != subscribedSymbols) {
                connect()
                continue
            }
            if (desiredSymbols.isEmpty()) continue
            val now = System.currentTimeMillis()
            if (connectionSource == ConnectionSource.SERVER && serverConnectedAt > 0 && now - serverConnectedAt >= 5 * 60_000L) {
                serverConnectedAt = now
                if(container.apiClient.probeDirect()) {
                    connectionSource = ConnectionSource.DIRECT
                    connect()
                    continue
                }
            }
            val silentFor = now - (lastMessageAt.takeIf { it > 0 } ?: connectionStartedAt)
            MonitorBus.update { state ->
                state.copy(
                    staleSymbols = state.prices.values
                        .filter { now - it.eventTime >= (if (it.symbol == GoldQuote.SYMBOL) 120_000 else 30_000) }
                        .mapTo(mutableSetOf()) { it.symbol },
                )
            }
            if (connectionSource == ConnectionSource.DIRECT && hasMarketRules() && silentFor >= 60.seconds.inWholeMilliseconds) {
                socket?.cancel()
                scheduleReconnect(ConnectionSource.DIRECT, "60 秒内未收到有效行情")
                continue
            }
            if (connectionSource == ConnectionSource.SERVER && silentFor >= 60.seconds.inWholeMilliseconds && !interruptionReported) {
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
