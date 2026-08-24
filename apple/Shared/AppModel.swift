import Foundation
import PriceCore
import SwiftUI
import UserNotifications

#if os(iOS)
@preconcurrency import ActivityKit
import UIKit
#elseif os(macOS)
import AppKit
#endif

@MainActor
final class AppModel: ObservableObject {
    @Published private(set) var contracts: [Contract] = []
    @Published private(set) var prices: [String: PricePoint] = [:]
    @Published var rules: [AlertRule] = LocalPersistence.loadRules()
    @Published private(set) var history: [TriggerRecord] = LocalPersistence.loadHistory()
    @Published private(set) var monitorState: MonitorState = .disconnected
    @Published private(set) var statusMessage = "监控未启动"
    @Published private(set) var backgroundStatusMessage = "仅支持前台监控"
    @Published var primarySymbol = UserDefaults.standard.string(forKey: "primarySymbol") ?? "BTCUSDT"
    @Published var menuSymbols: [String] = UserDefaults.standard.stringArray(forKey: "menuSymbols") ?? ["BTCUSDT"]

    private let market: BinanceMarketClient
    private let stream: PriceStream
    private var buffer = PriceBuffer()
    private var streamTask: Task<Void, Never>?
    private var relayRetryTask: Task<Void, Never>?
    private var watchdogTask: Task<Void, Never>?
    private var priceFlushTask: Task<Void, Never>?
    private var pendingPrices: [String: PricePoint] = [:]
    private var lastReceivedAt: Date?
    private var connectionStartedAt: Date?
    private var connectionFailureNotified = false
    private var usingServerRelay = false
    private var notificationsAllowed = false
    private var lastSurfaceUpdate = Date.distantPast
    private var lastPersistedEventTimes: [String: Int64] = [:]
    @Published private(set) var monitoringEnabled = false
    #if os(iOS)
    private var activity: Activity<PriceActivityAttributes>?
    private let iosBackgroundService = IOSBackgroundService()
    private var foregroundLeaseTask: Task<Void, Never>?
    private var liveActivityTokenTask: Task<Void, Never>?
    private var iosBackgroundPrepared = false
    #endif

    init() {
        self.market = BinanceMarketClient()
        self.stream = PriceStream()

        let restored = LocalPersistence.loadPrices()
        buffer.restore(restored)
        for symbol in buffer.symbols {
            if let latest = buffer.latest(symbol: symbol) {
                prices[symbol] = latest
                lastPersistedEventTimes[symbol] = latest.eventTime
            }
        }
    }

    func bootstrap(platform: String) async {
        do {
            contracts = try await market.contracts()
            if !monitoringEnabled {
                statusMessage = "可直接连接币安行情"
            }
        } catch {
            do {
                contracts = try await ServerConnectionService.shared.contracts(
                    platform: platform, displayName: platform == "macos" ? "Mac" : "iPhone"
                )
                if !monitoringEnabled {
                    statusMessage = "通过服务端获取币安合约"
                }
            } catch {
                if !monitoringEnabled {
                    statusMessage = "合约列表获取失败：\(error.localizedDescription)"
                }
            }
        }
    }

    func startMonitoring() async {
        guard !monitoringEnabled else { return }
        monitoringEnabled = true
        #if os(iOS)
        if !iosBackgroundPrepared {
            initializeRuleStatesFromCurrentPrices()
        }
        #endif
        monitorState = .connecting
        statusMessage = "正在连接"
        lastReceivedAt = nil
        connectionFailureNotified = false
        connect()
        #if !UI_TEST_HARNESS
        Task { await refreshNotificationAuthorization() }
        #endif
        #if os(iOS)
        await syncIOSBackgroundConfiguration()
        #endif
    }

    func stopMonitoring() {
        persistAllPrices()
        monitoringEnabled = false
        streamTask?.cancel()
        relayRetryTask?.cancel()
        watchdogTask?.cancel()
        priceFlushTask?.cancel()
        priceFlushTask = nil
        pendingPrices.removeAll()
        connectionStartedAt = nil
        connectionFailureNotified = false
        Task { await stream.disconnect() }
        monitorState = .disconnected
        statusMessage = "监控已停止"
        #if os(iOS)
        Task { await syncIOSBackgroundConfiguration() }
        #endif
    }

    func addRule(symbol: String, windowMinutes: Int, threshold: String) throws {
        let candidate = try AlertRule(symbol: symbol, windowMinutes: windowMinutes, thresholdText: threshold)
        guard rules.count < 50 else { throw PriceCoreError.invalidThreshold }
        guard !rules.contains(where: { $0.symbol == candidate.symbol && $0.windowMinutes == candidate.windowMinutes && $0.threshold == candidate.threshold }) else {
            throw PriceCoreError.duplicateRule
        }
        rules.append(candidate)
        LocalPersistence.saveRules(rules)
        rulesDidChange()
    }

    func setRuleEnabled(id: UUID, enabled: Bool) {
        guard let index = rules.firstIndex(where: { $0.id == id }) else { return }
        rules[index].isEnabled = enabled
        rules[index].riseTriggered = false
        rules[index].fallTriggered = false
        LocalPersistence.saveRules(rules)
        rulesDidChange()
    }

    func deleteRule(id: UUID) {
        rules.removeAll { $0.id == id }
        LocalPersistence.saveRules(rules)
        rulesDidChange()
    }

    func selectPrimary(_ symbol: String) {
        primarySymbol = symbol
        UserDefaults.standard.set(symbol, forKey: "primarySymbol")
        syncSubscriptionsIfMonitoring()
        #if os(iOS)
        Task { await syncIOSBackgroundConfiguration() }
        #endif
    }

    func setMenuSymbols(_ symbols: [String]) {
        menuSymbols = Array(symbols.prefix(3))
        UserDefaults.standard.set(menuSymbols, forKey: "menuSymbols")
        syncSubscriptionsIfMonitoring()
    }

    #if os(iOS)
    func prepareIOSBackgroundDelivery() async {
        do {
            let state = try await iosBackgroundService.enterForeground(
                rules: rules, monitoringEnabled: monitoringEnabled, primarySymbol: primarySymbol
            )
            rules = state.rules
            LocalPersistence.saveRules(rules)
            applyBackgroundEvents(state.events)
            try await iosBackgroundService.acknowledge(events: state.events)
            if let token = UserDefaults.standard.string(forKey: "apnsPushToken") {
                try await iosBackgroundService.registerPushToken(token)
            }
            iosBackgroundPrepared = true
            backgroundStatusMessage = "服务端后台监控已连接"
            startForegroundLeaseRenewal()
        } catch {
            iosBackgroundPrepared = false
            backgroundStatusMessage = "后台监控不可用：\(error.localizedDescription)"
        }
    }

    func registerAPNSToken(_ token: String) async {
        do {
            try await iosBackgroundService.registerPushToken(token)
            backgroundStatusMessage = "服务端后台监控与通知已就绪"
        } catch {
            backgroundStatusMessage = "APNs 注册失败：\(error.localizedDescription)"
        }
    }

    func fetchIOSBackgroundEvents() async {
        await prepareIOSBackgroundDelivery()
    }

    func startLiveActivity() async throws {
        let current = prices[primarySymbol]
        let content = ActivityContent(
            state: PriceActivityAttributes.ContentState(
                symbol: primarySymbol, price: current?.priceText ?? "--",
                direction: "flat", eventTime: current?.eventTime ?? Int64(Date().timeIntervalSince1970 * 1000)
            ),
            staleDate: Date().addingTimeInterval(30)
        )
        activity = try Activity.request(
            attributes: PriceActivityAttributes(startedAt: Date()),
            content: content,
            pushType: .token
        )
        guard let currentActivity = activity else { return }
        liveActivityTokenTask?.cancel()
        liveActivityTokenTask = Task {
            for await tokenData in currentActivity.pushTokenUpdates {
                guard !Task.isCancelled else { return }
                let token = tokenData.map { String(format: "%02x", $0) }.joined()
                let expiresAt = Int64(Date().addingTimeInterval(8 * 60 * 60).timeIntervalSince1970 * 1_000)
                try? await iosBackgroundService.registerLiveActivity(
                    id: currentActivity.id, token: token, symbol: primarySymbol, expiresAt: expiresAt
                )
            }
        }
    }

    func stopLiveActivity() async {
        guard let currentActivity = activity else { return }
        activity = nil
        liveActivityTokenTask?.cancel()
        liveActivityTokenTask = nil
        try? await iosBackgroundService.deleteLiveActivity(id: currentActivity.id)
        await currentActivity.end(nil, dismissalPolicy: .immediate)
    }

    func enterForeground() async {
        guard monitoringEnabled else { return }
        await prepareIOSBackgroundDelivery()
        if !iosBackgroundPrepared {
            initializeRuleStatesFromCurrentPrices()
        }
        connect()
    }

    func enterBackground() async {
        persistAllPrices()
        try? await iosBackgroundService.sync(
            rules: rules, monitoringEnabled: monitoringEnabled, primarySymbol: primarySymbol
        )
        foregroundLeaseTask?.cancel()
        foregroundLeaseTask = nil
        await iosBackgroundService.enterBackground()
        streamTask?.cancel()
        relayRetryTask?.cancel()
        watchdogTask?.cancel()
        priceFlushTask?.cancel()
        priceFlushTask = nil
        pendingPrices.removeAll()
        await stream.disconnect()
        monitorState = .disconnected
        statusMessage = "已进入后台；后台监控需要服务端"
    }
    #endif

    private func connect() {
        streamTask?.cancel()
        priceFlushTask?.cancel()
        priceFlushTask = nil
        pendingPrices.removeAll()
        streamTask = Task {
            var relayNext = false
            while !Task.isCancelled {
                do {
                    relayRetryTask?.cancel()
                    connectionStartedAt = Date()
                    lastReceivedAt = nil
                    monitorState = .connecting
                    usingServerRelay = relayNext
                    statusMessage = relayNext ? "正在连接服务端行情" : "正在直连币安"
                    let updates: AsyncThrowingStream<PriceStreamEvent, Error>
                    if relayNext {
                        #if os(iOS)
                        let platform = "ios"
                        let displayName = "iPhone"
                        #else
                        let platform = "macos"
                        let displayName = "Mac"
                        #endif
                        let relay = try await ServerConnectionService.shared.prepareStream(
                            symbols: desiredSymbols, platform: platform, displayName: displayName
                        )
                        updates = try await stream.connect(
                            serverURL: relay.url, token: relay.token,
                            lastEventTimes: Dictionary(uniqueKeysWithValues: desiredSymbols.map {
                                ($0, buffer.latest(symbol: $0)?.eventTime ?? 0)
                            })
                        )
                        relayRetryTask = Task {
                            try? await Task.sleep(for: .seconds(300))
                            guard !Task.isCancelled else { return }
                            await stream.disconnect()
                        }
                    } else {
                        updates = try await stream.connect(symbols: desiredSymbols)
                    }
                    for try await event in updates {
                        guard !Task.isCancelled else { return }
                        switch event {
                        case .price(let point): enqueue(point)
                        case .warmingUp:
                            updateReadiness()
                        case .ready:
                            updateReadiness()
                        }
                    }
                } catch {
                    relayRetryTask?.cancel()
                    guard !Task.isCancelled else { return }
                    relayNext.toggle()
                    monitorState = .disconnected
                    statusMessage = relayNext
                        ? "币安直连失败，正在切换服务端"
                        : "服务端连接中断，5 秒后重新检测币安"
                    if !relayNext {
                        try? await Task.sleep(for: .seconds(5))
                    }
                }
            }
        }
        watchdogTask?.cancel()
        watchdogTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                guard !Task.isCancelled, monitoringEnabled,
                      let reference = lastReceivedAt ?? connectionStartedAt else { continue }
                let elapsed = Date().timeIntervalSince(reference)
                if elapsed >= 3, !usingServerRelay {
                    statusMessage = "币安直连无有效行情，正在切换服务端"
                    await stream.disconnect()
                } else if elapsed >= 60, !connectionFailureNotified {
                    connectionFailureNotified = true
                    monitorState = .disconnected
                    statusMessage = "连续 60 秒未收到有效行情"
                    await sendHealthNotification(title: "价格监控已中断", body: "正在重新连接币安行情")
                    await stream.disconnect()
                } else if elapsed >= 30, monitorState != .disconnected {
                    monitorState = .stale
                    statusMessage = "连续 30 秒未收到有效行情"
                }
            }
        }
    }

    private func enqueue(_ point: PricePoint) {
        pendingPrices[point.symbol] = point
        guard priceFlushTask == nil else { return }
        priceFlushTask = Task {
            while !Task.isCancelled {
                try? await Task.sleep(for: .seconds(1))
                guard !Task.isCancelled else { return }
                let points = pendingPrices.values.sorted { $0.symbol < $1.symbol }
                pendingPrices.removeAll()
                points.forEach(receive)
                if pendingPrices.isEmpty {
                    priceFlushTask = nil
                    return
                }
            }
        }
    }

    private func receive(_ point: PricePoint) {
        guard buffer.add(point) else { return }
        prices[point.symbol] = point
        lastReceivedAt = Date()
        connectionFailureNotified = false
        if point.replay {
            monitorState = .warmingUp
            statusMessage = "正在补齐一小时价格"
            return
        }
        monitorState = .live
        evaluate(point)
        persistPriceIfDue(point)
        updateReadiness()
        #if os(iOS)
        if point.symbol == primarySymbol, Date().timeIntervalSince(lastSurfaceUpdate) >= 15 {
            lastSurfaceUpdate = Date()
            Task { await updateLiveActivity(point) }
        }
        #endif
    }

    private func persistPriceIfDue(_ point: PricePoint) {
        let last = lastPersistedEventTimes[point.symbol] ?? 0
        guard point.eventTime - last >= 15_000 else { return }
        LocalPersistence.savePrices(buffer.points(symbol: point.symbol), symbol: point.symbol)
        lastPersistedEventTimes[point.symbol] = point.eventTime
    }

    private func persistAllPrices() {
        for symbol in buffer.symbols {
            LocalPersistence.savePrices(buffer.points(symbol: symbol), symbol: symbol)
            lastPersistedEventTimes[symbol] = buffer.latest(symbol: symbol)?.eventTime
        }
    }

    private func updateReadiness() {
        let warming = rules.contains { rule in
            rule.isEnabled && !buffer.covers(
                symbol: rule.symbol,
                durationMilliseconds: Int64(rule.windowMinutes) * 60_000
            )
        }
        monitorState = warming ? .warmingUp : .live
        let notificationNote = notificationsAllowed ? "" : " · 系统通知未开启"
        let source = usingServerRelay ? "服务端币安行情" : "直连币安"
        statusMessage = warming
            ? "\(source) · 正在积累完整规则窗口\(notificationNote)"
            : "\(source) · 实时监控中\(notificationNote)"
    }

    private func evaluate(_ point: PricePoint) {
        var triggers: [AlertTrigger] = []
        for index in rules.indices where rules[index].symbol == point.symbol {
            triggers += RuleEngine.evaluate(rule: &rules[index], current: point, buffer: buffer)
        }
        LocalPersistence.saveRules(rules)
        guard !triggers.isEmpty else { return }
        let records = triggers.map {
            TriggerRecord(
                id: "\($0.ruleID.uuidString):\($0.direction.rawValue):\($0.eventTime)",
                symbol: $0.symbol, direction: $0.direction, changePercent: $0.changePercent,
                windowMinutes: $0.windowMinutes, thresholdText: $0.thresholdText,
                priceText: $0.priceText, eventTime: $0.eventTime
            )
        }
        let cutoff = Int64(Date().addingTimeInterval(-30 * 24 * 60 * 60).timeIntervalSince1970 * 1000)
        history = Array((records + history).filter { $0.eventTime >= cutoff }.uniqued(by: \.id).prefix(500))
        LocalPersistence.saveHistory(history)
        Task { await sendAlertNotification(symbol: point.symbol, triggers: triggers) }
    }

    private func rulesDidChange() {
        syncSubscriptionsIfMonitoring()
        #if os(iOS)
        Task { await syncIOSBackgroundConfiguration() }
        #endif
    }

    private var desiredSymbols: [String] {
        let enabledRuleSymbols = rules.filter(\.isEnabled).map(\.symbol)
        return Array(Set(enabledRuleSymbols + menuSymbols + [primarySymbol])).sorted().prefix(50).map { $0 }
    }

    private func syncSubscriptionsIfMonitoring() {
        guard monitoringEnabled else { return }
        statusMessage = "正在更新币安订阅"
        connect()
    }

    #if os(iOS)
    private func syncIOSBackgroundConfiguration() async {
        do {
            try await iosBackgroundService.sync(
                rules: rules, monitoringEnabled: monitoringEnabled, primarySymbol: primarySymbol
            )
            backgroundStatusMessage = "服务端后台监控已同步"
        } catch {
            backgroundStatusMessage = "后台规则同步失败：\(error.localizedDescription)"
        }
    }

    private func startForegroundLeaseRenewal() {
        foregroundLeaseTask?.cancel()
        foregroundLeaseTask = Task {
            while !Task.isCancelled {
                do {
                    try await iosBackgroundService.renewForegroundLease()
                } catch {
                    backgroundStatusMessage = "前后台交接异常：\(error.localizedDescription)"
                }
                try? await Task.sleep(for: .seconds(10))
            }
        }
    }

    private func applyBackgroundEvents(_ events: [IOSBackgroundEvent]) {
        guard !events.isEmpty else { return }
        let records = events.flatMap { event in
            event.triggers.compactMap { trigger -> TriggerRecord? in
                guard let change = Decimal(string: trigger.changePct, locale: Locale(identifier: "en_US_POSIX")) else { return nil }
                return TriggerRecord(
                    id: "\(event.id):\(trigger.ruleId):\(trigger.direction.rawValue)",
                    symbol: trigger.symbol, direction: trigger.direction, changePercent: change,
                    windowMinutes: trigger.windowMinutes, thresholdText: trigger.thresholdPct,
                    priceText: trigger.price, eventTime: trigger.eventTime
                )
            }
        }
        let cutoff = Int64(Date().addingTimeInterval(-30 * 24 * 60 * 60).timeIntervalSince1970 * 1_000)
        history = Array((records + history).filter { $0.eventTime >= cutoff }.uniqued(by: \.id).prefix(500))
        LocalPersistence.saveHistory(history)
    }

    private func initializeRuleStatesFromCurrentPrices() {
        for index in rules.indices {
            guard let current = buffer.latest(symbol: rules[index].symbol) else { continue }
            _ = RuleEngine.initialize(rule: &rules[index], current: current, buffer: buffer)
        }
        LocalPersistence.saveRules(rules)
    }

    #endif

    private func sendAlertNotification(symbol: String, triggers: [AlertTrigger]) async {
        guard notificationsAllowed else { return }
        let content = UNMutableNotificationContent()
        content.title = "\(symbol) 价格预警"
        content.body = triggers.map {
            let direction = $0.direction == .rise ? "上涨" : "下跌"
            return "\($0.windowMinutes)分钟\(direction) \($0.changePercent.formatted(.number.precision(.fractionLength(2))))%（阈值 \($0.thresholdText)%）"
        }.joined(separator: "；") + " · 最新价 \(triggers[0].priceText)"
        content.sound = .default
        try? await UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: "alert:\(symbol):\(triggers[0].eventTime)", content: content, trigger: nil))
    }

    private func sendHealthNotification(title: String, body: String) async {
        guard notificationsAllowed else { return }
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        try? await UNUserNotificationCenter.current().add(UNNotificationRequest(identifier: "monitor-health", content: content, trigger: nil))
    }

    #if os(iOS)
    private func updateLiveActivity(_ point: PricePoint) async {
        guard let currentActivity = activity else { return }
        let content = ActivityContent(
            state: PriceActivityAttributes.ContentState(symbol: point.symbol, price: point.priceText, direction: "flat", eventTime: point.eventTime),
            staleDate: Date().addingTimeInterval(30)
        )
        await currentActivity.update(content)
    }
    #endif

    private func refreshNotificationAuthorization() async {
        let center = UNUserNotificationCenter.current()
        var settings = await center.notificationSettings()
        if settings.authorizationStatus == .notDetermined {
            _ = try? await center.requestAuthorization(options: [.alert, .sound, .badge])
            settings = await center.notificationSettings()
        }
        notificationsAllowed = settings.authorizationStatus == .authorized || settings.authorizationStatus == .provisional
        if monitorState == .live || monitorState == .warmingUp {
            updateReadiness()
        }
    }

}

private extension Array {
    func uniqued<Key: Hashable>(by keyPath: KeyPath<Element, Key>) -> [Element] {
        var seen = Set<Key>()
        return filter { seen.insert($0[keyPath: keyPath]).inserted }
    }
}
