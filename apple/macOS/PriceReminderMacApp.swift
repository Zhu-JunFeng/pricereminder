import PriceCore
import SwiftUI

private func compactPrice(_ text: String) -> String {
    guard text.contains("."), !text.lowercased().contains("e") else { return text }
    var result = text
    while result.last == "0" { result.removeLast() }
    if result.last == "." { result.removeLast() }
    return result
}

#if !UI_TEST_HARNESS
@main
struct PriceReminderMacApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        MenuBarExtra {
            MenuPanel()
                .environmentObject(model)
        } label: {
            MenuBarLabel()
                .environmentObject(model)
                .task {
                    await model.startMonitoring()
                    await model.bootstrap(platform: "macos")
                }
        }
        .menuBarExtraStyle(.window)

        Settings {
            MacSettingsView().environmentObject(model)
        }
    }
}
#endif

private struct MenuBarLabel: View {
    @EnvironmentObject private var model: AppModel

    var body: some View {
        if model.menuBarSymbols.isEmpty {
            Image(systemName: "chart.line.uptrend.xyaxis")
                .accessibilityLabel("PriceReminder")
        } else {
            // A MenuBarExtra status item needs one label to show all prices.
            Text(verbatim: model.menuBarSymbols.map { symbol in
                let name = symbol == GoldQuote.symbol ? "黄金" : (symbol.hasSuffix("USDT") ? String(symbol.dropLast(4)) : symbol)
                let price = compactPrice(model.prices[symbol]?.priceText ?? "--")
                let arrow = switch model.consecutivePriceTrends[symbol] {
                case .rise: " ↑"
                case .fall: " ↓"
                case nil: ""
                }
                return "\(name) \(price)\(arrow)"
            }.joined(separator: " · "))
            .monospaced()
        }
    }
}

private struct MenuContentHeightKey: PreferenceKey {
    static let defaultValue: CGFloat = 0
    static func reduce(value: inout CGFloat, nextValue: () -> CGFloat) {
        value = max(value, nextValue())
    }
}

struct MenuPanel: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.openSettings) private var openSettings
    @State private var contentHeight: CGFloat = 0
    @State private var panelVisible = false
    @AppStorage("showPanelDetails") private var showPanelDetails = false
    @AppStorage("showPanelCharts") private var showPanelCharts = true

    var body: some View {
        Group {
            if panelVisible {
                TimelineView(.periodic(from: .now, by: 1)) { context in
                    panel(at: context.date)
                }
            } else {
                panel(at: Date())
            }
        }
        .background(MenuPanelWindowStyle(isVisible: $panelVisible))
        .onDisappear { panelVisible = false }
    }

    private func panel(at now: Date) -> some View {
        VStack(alignment: .leading, spacing: 0) {
            let status = model.panelMonitoringStatus(at: now)
            if showPanelDetails || !model.monitoringEnabled || model.monitorState == .disconnected {
                HStack {
                    Text(showPanelDetails ? status.text : (model.monitoringEnabled ? "行情连接中断" : "监控已停止"))
                        .font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Circle().fill(status.live ? AppTheme.primary : Color.orange).frame(width: 8, height: 8)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 10)
                Divider()
            }
            if let error = model.positionError { Text(error).font(.caption).foregroundStyle(AppTheme.fall).padding(12) }
            if showPanelCharts && model.chartAuthorizationRequired {
                HStack {
                    Text("查看走势需要授权服务端连接")
                        .font(.caption).foregroundStyle(.secondary)
                    Spacer()
                    Button(model.authorizingServer ? "正在授权…" : "授权") {
                        Task { await model.authorizeServer() }
                    }
                    .disabled(model.authorizingServer)
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 8)
                Divider()
            }
            if model.panelSymbols.isEmpty {
                VStack(spacing: 8) {
                    Text("还没有选择行情品种").foregroundStyle(.secondary)
                    Button("选择品种") {
                        model.settingsTab = "display"
                        NSApplication.shared.activate(ignoringOtherApps: true)
                        openSettings()
                    }
                }
                .frame(maxWidth: .infinity)
                .padding(20)
            } else {
                ScrollView {
                    VStack(spacing: 0) {
                        ForEach(model.panelSymbols, id: \.self) { symbol in
                            MenuPriceRow(symbol: symbol, now: now, details: showPanelDetails,
                                         showChart: panelVisible && showPanelCharts)
                                .padding(.horizontal, 16).padding(.vertical, 12)
                            Divider()
                        }
                    }
                    .fixedSize(horizontal: false, vertical: true)
                    .background {
                        GeometryReader { geometry in
                            Color.clear.preference(key: MenuContentHeightKey.self, value: geometry.size.height)
                        }
                    }
                }
                .frame(height: min(380, contentHeight))
                .onPreferenceChange(MenuContentHeightKey.self) { contentHeight = $0 }
            }
            Divider()
            HStack {
                Button(model.monitoringEnabled ? "停止监控" : "开始监控") {
                    if model.monitoringEnabled { model.stopMonitoring() }
                    else { Task { await model.startMonitoring() } }
                }
                Spacer()
                Button("管理持仓") {
                    model.settingsTab = "positions"
                    NSApplication.shared.activate(ignoringOtherApps: true)
                    openSettings()
                }
                Button("设置…") {
                    NSApplication.shared.activate(ignoringOtherApps: true)
                    openSettings()
                }
                Button("退出") { NSApplication.shared.terminate(nil) }
            }
            .buttonStyle(.borderless)
            .padding(14)
        }
        .frame(width: 380)
        .background(Color(nsColor: .windowBackgroundColor))
        .clipShape(RoundedRectangle(cornerRadius: 14, style: .continuous))
    }
}

private struct MenuPanelWindowStyle: NSViewRepresentable {
    @Binding var isVisible: Bool

    final class View: NSView {
        var onVisibilityChange: ((Bool) -> Void)?

        override func viewDidMoveToWindow() {
            super.viewDidMoveToWindow()
            let notifications = NotificationCenter.default
            notifications.removeObserver(self)
            guard let window else {
                setVisible(false)
                return
            }
            window.isOpaque = false
            window.backgroundColor = .clear
            window.hasShadow = true
            window.contentView?.wantsLayer = true
            window.contentView?.layer?.cornerRadius = 14
            window.contentView?.layer?.masksToBounds = true
            notifications.addObserver(self, selector: #selector(didBecomeKey),
                                      name: NSWindow.didBecomeKeyNotification, object: window)
            notifications.addObserver(self, selector: #selector(didHide),
                                      name: NSWindow.didResignKeyNotification, object: window)
            notifications.addObserver(self, selector: #selector(didHide),
                                      name: NSWindow.willCloseNotification, object: window)
            setVisible(window.isVisible && window.isKeyWindow)
        }

        @objc private func didBecomeKey(_ notification: Notification) { setVisible(true) }
        @objc private func didHide(_ notification: Notification) { setVisible(false) }

        private func setVisible(_ visible: Bool) {
            DispatchQueue.main.async { [weak self] in self?.onVisibilityChange?(visible) }
        }
    }
    func makeNSView(context: Context) -> View {
        let view = View()
        updateNSView(view, context: context)
        return view
    }
    func updateNSView(_ view: View, context: Context) {
        view.onVisibilityChange = { isVisible = $0 }
    }
}

private struct MenuPriceRow: View {
    @EnvironmentObject private var model: AppModel
    let symbol: String
    let now: Date

    let details: Bool
    let showChart: Bool

    private var isGold: Bool { symbol == GoldQuote.symbol }

    var body: some View {
        let stale = model.prices[symbol].map { model.isPriceStale($0, now: now) } ?? false
        VStack(alignment: .leading, spacing: 5) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Text(model.displaySymbol(symbol)).font(.headline).lineLimit(1)
                Spacer(minLength: 2)
                ViewThatFits(in: .horizontal) {
                    HStack(alignment: .firstTextBaseline, spacing: 5) {
                        headlinePrice(stale: stale)
                        headlineChange(stale: stale)
                    }
                    VStack(alignment: .trailing, spacing: 3) {
                        headlinePrice(stale: stale)
                        headlineChange(stale: stale)
                    }
                }
            }
            if isGold, model.goldQuote?.state == "closed" {
                Text("休市").font(.caption2).foregroundStyle(.secondary)
            } else if stale {
                Text("报价已过期").font(.caption2).foregroundStyle(.secondary)
            } else if isGold, !model.goldFeedAvailable, model.monitoringEnabled {
                Text("黄金行情暂不可用").font(.caption2).foregroundStyle(.secondary)
            }
            if showChart {
                PanelDayChart(symbol: symbol, now: now)
            }
            if details {
                Text(isGold ? "今日涨跌 · 相对昨收" : "今日涨跌 · 北京时间 00:00 起")
                    .font(.caption).foregroundStyle(.secondary)
                if isGold {
                    Text(model.goldStatus).font(.caption2).foregroundStyle(.secondary)
                    if let quote = model.goldQuote {
                        Text("新浪财经 · 报价时间 \(quote.quoteTimeText)")
                            .font(.caption2).foregroundStyle(.secondary)
                    }
                }
                Group {
                    let point = model.prices[symbol]
                    let stale = point.map { model.isPriceStale($0, now: now) } ?? true
                    VStack(alignment: .leading, spacing: 5) {
                        ForEach(model.positions.filter { $0.setting.symbol == symbol }) { position in
                            let setting = position.setting
                            HStack(alignment: .firstTextBaseline) {
                                Text("\(setting.positionSide.displayName) · \(setting.leverage)倍 · 开仓 \(compactPrice(setting.priceText))")
                                    .lineLimit(1).truncationMode(.middle).help(setting.priceText)
                                Spacer(minLength: 8)
                                if let point {
                                    let change = EntryPriceCalculator.change(currentPrice: point.price, entryPrice: setting.price, positionSide: setting.positionSide, leverage: setting.leverage)
                                    Text(change.percentageText).foregroundStyle(stale ? Color.secondary : color(for: change.direction)).fixedSize()
                                } else { Text("--") }
                            }
                        }
                    }.font(.caption).foregroundStyle(.secondary).numericPriceStyle()
                }
            }
        }
        .task(id: BeijingDailyPrice.startMilliseconds(at: now)) {
            if !isGold {
                await model.loadDailyOpen(symbol: symbol, start: BeijingDailyPrice.startMilliseconds(at: now))
            }
        }
    }

    private func headlinePrice(stale: Bool) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 3) {
            Text(compactPrice(model.prices[symbol]?.priceText ?? "--"))
                .font(.title3.weight(.semibold))
            if isGold { Text("元/克").font(.caption).foregroundStyle(.secondary) }
            PriceTrendArrow(trend: stale ? nil : model.consecutivePriceTrends[symbol], size: 9)
        }
        .numericPriceStyle()
        .fixedSize()
    }

    private func headlineChange(stale: Bool) -> some View {
        let change = model.dailyChange(symbol: symbol, now: now)
        return HStack(alignment: .firstTextBaseline, spacing: 3) {
            Text("今日").font(.caption).foregroundStyle(.secondary)
            Text(change?.percentageText ?? "--")
                .font(.title3.weight(.semibold))
                .foregroundStyle(change.flatMap { stale ? nil : color(for: $0.direction) } ?? .secondary)
        }
        .numericPriceStyle()
        .fixedSize()
        .help(isGold ? "相对上一交易日收盘价" : "相对北京时间 00:00 开盘价")
    }

    private func color(for direction: EntryPriceDirection) -> Color {
        switch direction {
        case .rise: AppTheme.rise
        case .fall: AppTheme.fall
        case .flat: .secondary
        }
    }
}

private struct PanelDayChart: View {
    @EnvironmentObject private var model: AppModel
    @State private var series: PriceChart?
    @State private var seriesWindow: Int64?
    @State private var failed = false
    let symbol: String
    let now: Date

    private var window: Int64 {
        let reference = symbol == GoldQuote.symbol ? now.addingTimeInterval(-20 * 3600) : now
        return BeijingDailyPrice.startMilliseconds(at: reference)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            if let series, seriesWindow == window {
                Canvas { context, size in
                    draw(series, context: context, size: size)
                }
                .frame(height: 48)
                .accessibilityLabel("\(model.displaySymbol(symbol))走势，\(series.date)")
                HStack {
                    Text(series.date)
                    if series.symbol == GoldQuote.symbol || series.stale || failed {
                        Text("更新于 \(lastUpdate(series.updatedAt))")
                    }
                    Spacer()
                    if let close = series.finalClose {
                        Text("收盘 \(compactPrice(close))")
                    }
                }
                .font(.caption2)
                .foregroundStyle(.secondary)
                .help(series.source)
            } else {
                Text(failed ? "走势暂不可用" : "正在加载走势…")
                    .font(.caption2).foregroundStyle(.secondary)
                    .frame(height: 48)
            }
        }
        .task(id: "\(window):\(model.chartAuthorizationVersion)") {
            let requestedWindow = window
            series = model.cachedPanelChart(symbol: symbol, window: requestedWindow)
            seriesWindow = requestedWindow
            failed = false
            while !Task.isCancelled {
                do {
                    let history = try await model.loadPanelChart(symbol: symbol, window: requestedWindow)
                    guard !Task.isCancelled else { return }
                    series = history
                    failed = false
                } catch {
                    guard !Task.isCancelled else { return }
                    failed = true
                }
                do { try await Task.sleep(for: .seconds(60)) } catch { return }
            }
        }
    }

    private func lastUpdate(_ milliseconds: Int64) -> String {
        let date = Date(timeIntervalSince1970: Double(milliseconds) / 1000)
        var style = Date.FormatStyle().hour(.twoDigits(amPM: .omitted)).minute(.twoDigits)
        style.timeZone = TimeZone(secondsFromGMT: 8 * 3600)!
        return date.formatted(style)
    }

    private struct PlotPoint {
        let time: Int64
        let open: Double?
        var high: Double
        var low: Double
        var close: Double
    }

    private func draw(_ series: PriceChart, context: GraphicsContext, size: CGSize) {
        var points = series.points.compactMap { point -> PlotPoint? in
            guard let close = Double(point.close), close.isFinite else { return nil }
            return PlotPoint(time: point.time, open: point.open.flatMap(Double.init),
                             high: point.high.flatMap(Double.init) ?? close,
                             low: point.low.flatMap(Double.init) ?? close, close: close)
        }
        if let index = points.indices.last, points[index].open != nil,
           series.finalClose == nil, let live = model.prices[symbol], live.eventTime >= series.updatedAt,
           live.eventTime >= points[index].time, live.eventTime - points[index].time < series.intervalMS,
           let price = Double(live.priceText), price.isFinite {
            points[index].close = price
            points[index].high = max(price, points[index].high)
            points[index].low = min(price, points[index].low)
        }
        guard let first = points.first, let last = points.last,
              let high = points.map(\.high).max(), let low = points.map(\.low).min(),
              high.isFinite, low.isFinite else { return }
        let timeSpan = max(series.intervalMS, last.time - first.time)
        let width = max(1, size.width - 8)
        let scale = max(high - low, high * 0.000001, 0.000000000001)
        func x(_ time: Int64) -> CGFloat { 4 + CGFloat(time - first.time) / CGFloat(timeSpan) * width }
        func y(_ price: Double) -> CGFloat {
            size.height / 2 - CGFloat((price - low - (high - low) / 2) / scale) * max(1, size.height - 8)
        }
        if first.open != nil {
            let candleWidth = max(1, min(7, width * CGFloat(series.intervalMS) / CGFloat(timeSpan) * 0.65))
            for point in points {
                guard let open = point.open else { continue }
                let color = point.close >= open ? AppTheme.rise : AppTheme.fall
                let center = x(point.time)
                var wick = Path()
                wick.move(to: CGPoint(x: center, y: y(point.high)))
                wick.addLine(to: CGPoint(x: center, y: y(point.low)))
                context.stroke(wick, with: .color(color), lineWidth: 1)
                let body = CGRect(x: center - candleWidth / 2, y: min(y(open), y(point.close)),
                                  width: candleWidth, height: max(1, abs(y(open) - y(point.close))))
                context.fill(Path(body), with: .color(color))
            }
        } else {
            var line = Path()
            var previous: PlotPoint?
            for point in points {
                let location = CGPoint(x: x(point.time), y: y(point.close))
                if let previous, point.time - previous.time == series.intervalMS {
                    line.addLine(to: location)
                } else {
                    line.move(to: location)
                    context.fill(Path(ellipseIn: CGRect(x: location.x - 1, y: location.y - 1, width: 2, height: 2)),
                                 with: .color(AppTheme.primary))
                }
                previous = point
            }
            context.stroke(line, with: .color(AppTheme.primary), lineWidth: 1.5)
        }
    }
}

private struct PriceTrendArrow: View {
    let trend: ConsecutivePriceTrend?
    let size: CGFloat

    var body: some View {
        if let trend {
            Image(nsImage: symbolImage(for: trend))
                .renderingMode(.original)
                .frame(width: size, height: size)
                .accessibilityLabel(trend == .rise ? "连续上涨" : "连续下跌")
        }
    }

    private func symbolImage(for trend: ConsecutivePriceTrend) -> NSImage {
        let name = trend == .rise ? "arrow.up" : "arrow.down"
        let color = NSColor(trend == .rise ? AppTheme.rise : AppTheme.fall)
        let sizeConfiguration = NSImage.SymbolConfiguration(pointSize: size, weight: .bold)
        let colorConfiguration = NSImage.SymbolConfiguration(paletteColors: [color])
        let configuration = sizeConfiguration.applying(colorConfiguration)
        let image = NSImage(systemSymbolName: name, accessibilityDescription: nil)?
            .withSymbolConfiguration(configuration) ?? NSImage()
        image.isTemplate = false
        return image
    }
}

struct MacSettingsView: View {
    @EnvironmentObject private var model: AppModel
    @AppStorage("showPanelDetails") private var showPanelDetails = false
    @AppStorage("showPanelCharts") private var showPanelCharts = true
    @State private var menuSelectionError: String?
    @State private var contractQuery = ""
    @State private var ruleSymbol = "BTCUSDT"
    @State private var ruleWindow = 5
    @State private var ruleThreshold = "3"
    @State private var ruleKind: AlertRuleKind = .percentage
    @State private var targetDirection: TargetDirection = .above
    @State private var targetPrice = ""
    @State private var ruleError: String?
    @State private var ruleSuccess = ""
    @State private var showingRuleSuccess = false
    @State private var pendingDeletion: (id: UUID, market: Bool, description: String)?
    @State private var showingDeleteConfirmation = false
    @State private var deletionSuccess: String?
    @State private var testResult: String?
    @State private var showingRuleContracts = false
    @State private var editingRuleID: UUID?
    @State private var editingMarketRuleID: UUID?

    var body: some View {
        TabView(selection: $model.settingsTab) {
            VStack(alignment: .leading, spacing: 14) {
                Text("行情显示").font(.title2.weight(.semibold))
                Text("菜单栏按已选顺序显示价格，展开面板可查看全部。")
                    .foregroundStyle(.secondary)

                Picker("菜单栏显示数量", selection: Binding(
                    get: { model.menuBarPriceCount },
                    set: { model.setMenuBarPriceCount($0) }
                )) {
                    Text("仅图标").tag(0)
                    ForEach(AppModel.menuBarPriceCounts.dropFirst(), id: \.self) { count in
                        Text("\(count) 个").tag(count)
                    }
                }
                .pickerStyle(.menu)

                HStack(spacing: 18) {
                    Toggle("显示行情详情", isOn: $showPanelDetails)
                    Toggle("显示今日走势", isOn: $showPanelCharts)
                }
                .toggleStyle(.checkbox)

                if !model.menuSymbols.isEmpty {
                    selectedSymbolsOrder
                }

                TextField("搜索品种，如 BTC、黄金", text: $contractQuery)
                    .textFieldStyle(.roundedBorder)

                HStack {
                    Text("已选 \(model.menuSymbols.count) 个品种")
                        .font(.caption)
                        .foregroundStyle(.secondary)
                    if let error = menuSelectionError {
                        Text(error).font(.caption).foregroundStyle(AppTheme.fall)
                    }
                }

                List(filteredContracts) { contract in
                    Toggle(isOn: Binding(
                        get: { model.menuSymbols.contains(contract.symbol) },
                        set: { selected in
                            do {
                                try model.setMenuSymbol(contract.symbol, selected: selected)
                                menuSelectionError = nil
                            } catch { menuSelectionError = error.localizedDescription }
                        }
                    )) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(model.displaySymbol(contract.symbol))
                            Text("\(contract.baseAsset) / \(contract.quoteAsset)")
                                .font(.caption)
                                .foregroundStyle(.secondary)
                        }
                    }
                    .toggleStyle(.checkbox)
                }
                .overlay {
                    if filteredContracts.isEmpty {
                        contractListStatus
                    }
                }
                .onAppear {
                    if model.contracts.isEmpty {
                        Task { await model.bootstrap(platform: "macos") }
                    }
                }
            }
            .padding(20)
            .tabItem { Label("显示", systemImage: "menubar.rectangle") }.tag("display")
            PositionsView().tabItem { Label("持仓", systemImage: "list.bullet.rectangle") }.tag("positions")

            VStack(alignment: .leading, spacing: 12) {
                Text("预警规则").font(.title2.weight(.semibold))
                Text("支持单合约、全市场涨跌幅和目标价格提醒。")
                    .foregroundStyle(.secondary)

                Picker("提醒类型", selection: $ruleKind) {
                    if editingMarketRuleID != nil {
                        Text("全市场").tag(AlertRuleKind.marketPercentage)
                    } else {
                        Text("单合约").tag(AlertRuleKind.percentage)
                        if editingRuleID == nil {
                            Text("全市场").tag(AlertRuleKind.marketPercentage)
                        }
                        Text("目标价格").tag(AlertRuleKind.target)
                    }
                }
                .pickerStyle(.segmented)

                HStack(spacing: 10) {
                    if ruleKind == .marketPercentage {
                        Text("全部 USDT / USDC 永续").frame(maxWidth: 220, alignment: .leading)
                    } else {
                        Button { showingRuleContracts.toggle() } label: {
                            HStack(spacing: 6) {
                                Text(ruleSymbol)
                                Image(systemName: "chevron.up.chevron.down")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                        .popover(isPresented: $showingRuleContracts) {
                            MacContractPicker(selected: ruleSymbol) {
                                ruleSymbol = $0
                                showingRuleContracts = false
                            }
                        }
                        .frame(maxWidth: 220)
                    }
                    if ruleKind != .target {
                        Stepper("\(ruleWindow) 分钟", value: $ruleWindow, in: 1...60)
                            .frame(width: 135)
                        TextField("阈值 %", text: $ruleThreshold)
                            .frame(width: 72)
                    } else {
                        Picker("条件", selection: $targetDirection) {
                            Text("达到或高于").tag(TargetDirection.above)
                            Text("达到或低于").tag(TargetDirection.below)
                        }
                        .frame(width: 135)
                        TextField("目标价格", text: $targetPrice)
                            .frame(width: 105)
                    }
                    Button(editingRuleID == nil && editingMarketRuleID == nil ? "添加" : "保存修改") { saveRule() }
                        .buttonStyle(.borderedProminent)
                    if editingRuleID != nil || editingMarketRuleID != nil {
                        Button("取消") { cancelEditing() }
                    }
                }

                if let ruleError {
                    Text(ruleError).font(.caption).foregroundStyle(AppTheme.fall)
                }
                if let deletionSuccess {
                    Label(deletionSuccess, systemImage: "checkmark.circle.fill")
                        .font(.caption).foregroundStyle(AppTheme.rise)
                }

                if model.rules.isEmpty && model.marketRules.isEmpty {
                    ContentUnavailableView("还没有预警规则", systemImage: "bell.slash", description: Text("可添加单合约、全市场或目标价格提醒。"))
                } else {
                    List {
                        ForEach(model.rules) { rule in
                        HStack(spacing: 12) {
                            VStack(alignment: .leading, spacing: 3) {
                                Text(rule.symbol).font(.headline)
                                Text(ruleDescription(rule))
                                    .foregroundStyle(.secondary)
                            }
                            Spacer()
                            Toggle("", isOn: Binding(
                                get: { rule.isEnabled },
                                set: { model.setRuleEnabled(id: rule.id, enabled: $0) }
                            ))
                            .labelsHidden()
                            Button { beginEditing(rule) } label: {
                                Image(systemName: "pencil")
                            }
                            .buttonStyle(.borderless)
                            .help("编辑提醒")
                            Button(role: .destructive) {
                                deletionSuccess = nil
                                pendingDeletion = (rule.id, false, "\(model.displaySymbol(rule.symbol)) · \(ruleDescription(rule))")
                                showingDeleteConfirmation = true
                            } label: {
                                Image(systemName: "trash")
                            }
                            .buttonStyle(.borderless)
                        }
                        }
                        ForEach(model.marketRules) { rule in
                            HStack(spacing: 12) {
                                VStack(alignment: .leading, spacing: 3) {
                                    Text("全部 USDT / USDC 永续").font(.headline)
                                    Text("\(rule.windowMinutes)分钟内涨跌 ≥ \(rule.thresholdText)%")
                                        .foregroundStyle(.secondary)
                                }
                                Spacer()
                                Toggle("", isOn: Binding(
                                    get: { rule.isEnabled },
                                    set: { model.setMarketRuleEnabled(id: rule.id, enabled: $0) }
                                )).labelsHidden()
                                Button { beginEditing(rule) } label: { Image(systemName: "pencil") }
                                    .buttonStyle(.borderless)
                                Button(role: .destructive) {
                                    deletionSuccess = nil
                                    pendingDeletion = (rule.id, true, "全市场 · \(rule.windowMinutes)分钟 · \(rule.thresholdText)%")
                                    showingDeleteConfirmation = true
                                } label: {
                                    Image(systemName: "trash")
                                }.buttonStyle(.borderless)
                            }
                        }
                    }
                }
            }
            .padding(20)
            .tabItem { Label("预警", systemImage: "bell") }.tag("alerts")

            VStack(alignment: .leading, spacing: 12) {
                Text("连接自检").font(.title2.weight(.semibold))
                Text("查看实时链路、价格新鲜度和系统通知状态。")
                    .foregroundStyle(.secondary)

                Group {
                    DiagnosticRow(label: "监控状态", value: model.statusMessage)
                    DiagnosticRow(label: "连接路径", value: model.connectionSourceLabel)
                    DiagnosticRow(label: "订阅合约", value: "\(model.subscribedSymbolCount) 个")
                    DiagnosticRow(label: "行情延迟", value: model.latestPriceDelayText)
                    DiagnosticRow(label: "最后接收", value: lastReceivedText)
                    DiagnosticRow(label: "重连次数", value: "\(model.reconnectCount)")
                    DiagnosticRow(label: "通知权限", value: model.notificationStatusLabel)
                    DiagnosticRow(label: "全市场扫描", value: model.marketStatusMessage)
                    DiagnosticRow(label: "全市场覆盖", value: "\(model.marketContractCount) 个合约")
                    DiagnosticRow(
                        label: "全市场最后接收",
                        value: model.lastMarketReceivedAt?.formatted(date: .omitted, time: .standard) ?? "尚未收到"
                    )
                    if let error = model.lastConnectionError {
                        DiagnosticRow(label: "最近错误", value: error)
                    }
                }

                HStack {
                    Button("刷新状态") { Task { await model.refreshSelfCheck() } }
                    Button(model.authorizingServer ? "正在授权…" : "授权服务端连接") {
                        Task { await model.authorizeServer() }
                    }
                    .disabled(model.authorizingServer)
                    Button("发送测试通知") {
                        Task {
                            testResult = await model.sendTestNotification()
                                ? "测试通知已发送"
                                : "通知权限未开启"
                        }
                    }
                    if let testResult {
                        Text(testResult).font(.caption).foregroundStyle(.secondary)
                    }
                }
                if let message = model.serverAuthorizationMessage {
                    Text(message).font(.caption).foregroundStyle(.secondary)
                }
                Text("自动重连不会弹出钥匙串密码窗口；仅主动授权时可能需要系统确认。")
                    .font(.caption).foregroundStyle(.secondary)
                Spacer()
            }
            .padding(20)
            .tabItem { Label("自检", systemImage: "stethoscope") }.tag("diagnostics")
        }
        .frame(width: 620, height: 460)
        .alert("删除这条预警？", isPresented: $showingDeleteConfirmation) {
            Button("取消", role: .cancel) { pendingDeletion = nil }
            Button("删除预警", role: .destructive) { confirmRuleDeletion() }
        } message: {
            Text("\(pendingDeletion?.description ?? "")\n删除后将不再按此规则提醒。")
        }
        .alert("预警设置成功", isPresented: $showingRuleSuccess) {
            Button("知道了", role: .cancel) {}
        } message: {
            Text(ruleSuccess)
        }
        .onAppear {
            ruleSymbol = model.primarySymbol
        }
    }

    private var selectedSymbolsOrder: some View {
        GroupBox("已选品种顺序") {
            ScrollView {
                VStack(spacing: 4) {
                    ForEach(model.menuSymbols, id: \.self) { symbol in
                        HStack(spacing: 12) {
                            Text(model.displaySymbol(symbol))
                            Spacer()
                            Button {
                                model.moveMenuSymbol(symbol, up: true)
                            } label: {
                                Image(systemName: "chevron.up")
                            }
                            .disabled(symbol == model.menuSymbols.first)
                            .help("上移")
                            .accessibilityLabel("上移 \(model.displaySymbol(symbol))")
                            Button {
                                model.moveMenuSymbol(symbol, up: false)
                            } label: {
                                Image(systemName: "chevron.down")
                            }
                            .disabled(symbol == model.menuSymbols.last)
                            .help("下移")
                            .accessibilityLabel("下移 \(model.displaySymbol(symbol))")
                        }
                        .buttonStyle(.borderless)
                        .padding(.vertical, 3)
                    }
                }
                .padding(.trailing, 6)
            }
            .frame(maxHeight: 100)
        }
    }

    @ViewBuilder
    private var contractListStatus: some View {
        if model.loadingContracts {
            ProgressView("正在加载合约目录…")
        } else if let error = model.contractLoadError {
            ContentUnavailableView {
                Label("暂时无法加载合约目录", systemImage: "wifi.exclamationmark")
            } description: {
                Text(model.contractAuthorizationRequired ? model.serverAuthorizationMessage ?? error : error)
            } actions: {
                if model.contractAuthorizationRequired {
                    Button(model.authorizingServer ? "正在授权…" : "授权服务端连接") {
                        Task { await model.authorizeServer() }
                    }
                    .disabled(model.authorizingServer)
                }
                Button("重新加载") { Task { await model.bootstrap(platform: "macos") } }
                    .disabled(model.authorizingServer)
            }
        } else if model.contracts.isEmpty {
            Button("加载合约目录") { Task { await model.bootstrap(platform: "macos") } }
        } else {
            ContentUnavailableView("未找到匹配品种", systemImage: "magnifyingglass",
                                   description: Text("试试 ETH、BTC 或黄金。"))
        }
    }

    private var filteredContracts: [Contract] {
        model.orderedContracts(query: contractQuery)
    }

    private func confirmRuleDeletion() {
        guard let pending = pendingDeletion else { return }
        if pending.market {
            model.deleteMarketRule(id: pending.id)
        } else {
            model.deleteRule(id: pending.id)
        }
        if editingRuleID == pending.id || editingMarketRuleID == pending.id { cancelEditing() }
        ruleError = nil
        deletionSuccess = "预警已删除：\(pending.description)"
        pendingDeletion = nil
    }

    private func saveRule() {
        deletionSuccess = nil
        showingRuleSuccess = false
        let editing = editingRuleID != nil || editingMarketRuleID != nil
        let name = ruleKind == .marketPercentage ? "全市场" : model.displaySymbol(ruleSymbol)
        let detail = ruleKind == .target
            ? "\(targetDirection == .above ? "达到或高于" : "达到或低于") \(targetPrice)"
            : "\(ruleWindow)分钟 · 涨跌幅 \(ruleThreshold)%"
        do {
            if let editingRuleID {
                try model.updateRule(
                    id: editingRuleID, symbol: ruleSymbol, kind: ruleKind,
                    windowMinutes: ruleWindow, threshold: ruleThreshold,
                    direction: targetDirection, targetPrice: targetPrice
                )
                cancelEditing()
            } else if let editingMarketRuleID {
                try model.updateMarketRule(
                    id: editingMarketRuleID, windowMinutes: ruleWindow, threshold: ruleThreshold
                )
                cancelEditing()
            } else if ruleKind == .marketPercentage {
                try model.addMarketRule(windowMinutes: ruleWindow, threshold: ruleThreshold)
            } else if ruleKind == .percentage {
                try model.addRule(symbol: ruleSymbol, windowMinutes: ruleWindow, threshold: ruleThreshold)
            } else {
                try model.addTargetRule(symbol: ruleSymbol, direction: targetDirection, targetPrice: targetPrice)
            }
            ruleError = nil
            ruleSuccess = "\(editing ? "预警已更新" : "预警已添加")：\(name)\n\(detail)"
            showingRuleSuccess = true
        } catch {
            ruleError = error.localizedDescription
        }
    }

    private func beginEditing(_ rule: MarketAlertRule) {
        editingMarketRuleID = rule.id
        editingRuleID = nil
        ruleKind = .marketPercentage
        ruleWindow = rule.windowMinutes
        ruleThreshold = rule.thresholdText
        ruleError = nil
    }

    private func beginEditing(_ rule: AlertRule) {
        editingRuleID = rule.id
        ruleSymbol = rule.symbol
        ruleKind = rule.kind
        ruleWindow = rule.windowMinutes == 0 ? 5 : rule.windowMinutes
        ruleThreshold = rule.thresholdText
        targetDirection = rule.targetDirection ?? .above
        targetPrice = rule.targetPriceText ?? ""
        ruleError = nil
    }

    private func cancelEditing() {
        editingRuleID = nil
        editingMarketRuleID = nil
        ruleSymbol = model.primarySymbol
        ruleWindow = 5
        ruleThreshold = "3"
        ruleKind = .percentage
        targetDirection = .above
        targetPrice = ""
        ruleError = nil
    }

    private func ruleDescription(_ rule: AlertRule) -> String {
        if rule.kind == .target {
            let direction = rule.targetDirection == .above ? "达到或高于" : "达到或低于"
            return "\(direction) \(rule.targetPriceText ?? "--")"
        }
        return "\(rule.windowMinutes)分钟 · \(rule.thresholdText)%"
    }

    private var lastReceivedText: String {
        model.lastPriceReceivedAt?.formatted(date: .omitted, time: .standard) ?? "尚未收到"
    }
}

private struct MacContractPicker: View {
    @EnvironmentObject private var model: AppModel
    @State private var query = ""
    let selected: String
    let onSelect: (String) -> Void

    var body: some View {
        VStack(spacing: 10) {
            TextField("输入合约代码，如 BTC", text: $query)
                .textFieldStyle(.roundedBorder)
            ScrollView {
                LazyVStack(spacing: 2) {
                    ForEach(model.orderedContracts(query: query)) { contract in
                        Button {
                            model.recordRecentSymbol(contract.symbol)
                            onSelect(contract.symbol)
                        } label: {
                            HStack {
                                VStack(alignment: .leading, spacing: 2) {
                                    Text(model.displaySymbol(contract.symbol)).foregroundStyle(.primary)
                                    Text("\(contract.baseAsset) / \(contract.quoteAsset)")
                                        .font(.caption).foregroundStyle(.secondary)
                                }
                                Spacer()
                                if contract.symbol == selected {
                                    Image(systemName: "checkmark").foregroundStyle(AppTheme.primary)
                                }
                            }
                            .contentShape(Rectangle())
                            .padding(.horizontal, 8)
                            .padding(.vertical, 6)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .frame(height: 250)
        }
        .padding(12)
        .frame(width: 300)
    }
}

private struct DiagnosticRow: View {
    let label: String
    let value: String

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(label).frame(width: 84, alignment: .leading).foregroundStyle(.secondary)
            Text(value).textSelection(.enabled)
            Spacer()
        }
        Divider()
    }
}

private struct PositionsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var editing: PositionRecord?
    @State private var showingEditor = false
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("持仓记录").font(.title2.weight(.semibold))
                Spacer()
                Button("添加持仓") { editing = nil; showingEditor = true }
            }
            Text("手动记录参考收益，不含手续费和资金费。删除记录不会平仓。")
                .font(.caption).foregroundStyle(.secondary)
            if let error = model.positionError { Text(error).foregroundStyle(AppTheme.fall).font(.caption) }
            List {
                ForEach(model.positions) { position in
                    HStack {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(position.setting.symbol).fontWeight(.semibold)
                            Text("\(position.setting.positionSide.displayName) · \(position.setting.leverage)倍 · 开仓 \(compactPrice(position.setting.priceText))").font(.caption).foregroundStyle(.secondary)
                        }
                        Spacer()
                        Button("编辑") { editing = position; showingEditor = true }
                        Button("删除记录") { model.deletePosition(position.id) }
                    }.padding(.vertical, 4)
                }
            }
            .overlay { if model.positions.isEmpty { Text("暂无持仓，点击“添加持仓”开始记录。").foregroundStyle(.secondary) } }
        }.padding(20)
        .sheet(isPresented: $showingEditor) {
            PositionEditor(existing: editing).environmentObject(model)
        }
    }
}

private struct PositionEditor: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    let existing: PositionRecord?
    @State private var symbol = ""
    @State private var query = ""
    @State private var side = PositionSide.long
    @State private var price = ""
    @State private var leverage = "1"
    @State private var error: String?
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(existing == nil ? "添加持仓" : "编辑持仓").font(.title2.bold())
            TextField("搜索合约，如 ETH 或 USDC", text: $query).textFieldStyle(.roundedBorder)
            List(model.orderedContracts(query: query).filter { $0.symbol != GoldQuote.symbol }, selection: $symbol) { contract in
                Text(model.displaySymbol(contract.symbol)).tag(contract.symbol)
            }.frame(height: 140)
                .overlay {
                    if model.orderedContracts(query: query).filter({ $0.symbol != GoldQuote.symbol }).isEmpty {
                        Text(model.contracts.isEmpty ? "合约列表尚未加载，请稍后重试" : "没有匹配的合约")
                            .font(.caption).foregroundStyle(.secondary)
                    }
                }
            Text(symbol.isEmpty ? "请选择合约" : symbol).font(.headline)
            Picker("方向", selection: $side) {
                Text("多").tag(PositionSide.long)
                Text("空").tag(PositionSide.short)
            }.pickerStyle(.segmented)
            HStack {
                Text("开仓价")
                TextField("开仓价", text: $price).textFieldStyle(.roundedBorder)
                TimelineView(.periodic(from: .now, by: 1)) { context in
                    Button("使用当前价") {
                        if let point = model.prices[symbol], !model.isPriceStale(point, now: Date()) { price = point.priceText }
                    }.disabled(model.prices[symbol].map { model.isPriceStale($0, now: context.date) } ?? true)
                }
            }
            HStack {
                Text("杠杆")
                TextField("1", text: $leverage).textFieldStyle(.roundedBorder).frame(width: 70)
                Text("倍")
            }
            if let error { Text(error).font(.caption).foregroundStyle(AppTheme.fall) }
            HStack {
                Spacer()
                Button("取消") { dismiss() }.keyboardShortcut(.cancelAction)
                Button("保存") {
                    do {
                        guard let multiple = Int(leverage.trimmingCharacters(in: .whitespaces)), multiple > 0 else { throw PriceCoreError.invalidLeverage }
                        let setting = try EntryPriceSetting(symbol: symbol, priceText: price, positionSide: side, leverage: multiple)
                        try model.savePosition(PositionRecord(id: existing?.id ?? UUID(), setting: setting))
                        dismiss()
                    } catch { self.error = error.localizedDescription }
                }.keyboardShortcut(.defaultAction).disabled(symbol.isEmpty)
            }
        }.padding(20).frame(width: 440)
        .onAppear {
            if let setting = existing?.setting {
                symbol = setting.symbol; side = setting.positionSide; price = setting.priceText; leverage = String(setting.leverage)
            }
        }
    }
}
