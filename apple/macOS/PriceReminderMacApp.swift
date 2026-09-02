import PriceCore
import SwiftUI

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
        HStack(spacing: 5) {
            ForEach(Array(model.menuSymbols.prefix(3).enumerated()), id: \.element) { index, symbol in
                if index > 0 {
                    Text("·").foregroundStyle(.secondary)
                }
                HStack(spacing: 2) {
                    let shortSymbol = symbol.replacingOccurrences(of: "USDT", with: "")
                    Text("\(shortSymbol) \(model.prices[symbol]?.priceText ?? "--")")
                    PriceTrendArrow(trend: model.consecutivePriceTrends[symbol], size: 7)
                }
            }
        }
        .monospacedDigit()
    }
}

struct MenuPanel: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            HStack {
                VStack(alignment: .leading, spacing: 3) {
                    Text("币价提醒").font(.headline)
                    Text(model.statusMessage).font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Circle().fill(model.monitorState == .live ? AppTheme.primary : Color.orange).frame(width: 8, height: 8)
            }
            .padding(16)
            Divider()
            ForEach(model.menuSymbols.prefix(3), id: \.self) { symbol in
                HStack {
                    Text(symbol).font(.headline)
                    Spacer()
                    HStack(spacing: 5) {
                        Text(model.prices[symbol]?.priceText ?? "--")
                            .font(.title3.weight(.semibold)).numericPriceStyle()
                        PriceTrendArrow(trend: model.consecutivePriceTrends[symbol], size: 9)
                    }
                }
                .padding(.horizontal, 16).padding(.vertical, 12)
            }
            Divider()
            HStack {
                Button(model.monitoringEnabled ? "停止监控" : "开始监控") {
                    if model.monitoringEnabled { model.stopMonitoring() }
                    else { Task { await model.startMonitoring() } }
                }
                Spacer()
                Button("设置…") {
                    NSApplication.shared.activate(ignoringOtherApps: true)
                    openSettings()
                }
                Button("退出") { NSApplication.shared.terminate(nil) }
            }
            .buttonStyle(.borderless)
            .padding(14)
        }
        .frame(width: 340)
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
    @State private var selection = Set<String>()
    @State private var contractQuery = ""
    @State private var ruleSymbol = "BTCUSDT"
    @State private var ruleWindow = 5
    @State private var ruleThreshold = "3"
    @State private var ruleKind: AlertRuleKind = .percentage
    @State private var targetDirection: TargetDirection = .above
    @State private var targetPrice = ""
    @State private var ruleError: String?
    @State private var testResult: String?
    @State private var showingRuleContracts = false
    @State private var editingRuleID: UUID?
    @State private var editingMarketRuleID: UUID?

    var body: some View {
        TabView {
            VStack(alignment: .leading, spacing: 14) {
                Text("菜单栏合约").font(.title2.weight(.semibold))
                Text("最多选择三个合约，价格每秒更新。").foregroundStyle(.secondary)

                TextField("筛选合约代码，如 BTC", text: $contractQuery)
                    .textFieldStyle(.roundedBorder)

                List(filteredContracts, selection: $selection) { contract in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(contract.symbol)
                        Text("\(contract.baseAsset) / \(contract.quoteAsset)")
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                    .tag(contract.symbol)
                }
                .overlay {
                    if !contractQuery.isEmpty, filteredContracts.isEmpty {
                        ContentUnavailableView.search(text: contractQuery)
                    }
                }
                .onChange(of: selection) { _, value in
                    if value.count <= 3 {
                        model.setMenuSymbols(Array(value).sorted())
                    } else {
                        selection = Set(model.menuSymbols)
                    }
                }
            }
            .padding(20)
            .tabItem { Label("显示", systemImage: "menubar.rectangle") }

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
                        Text("全部 USDT 永续").frame(maxWidth: 220, alignment: .leading)
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
                            Button(role: .destructive) { model.deleteRule(id: rule.id) } label: {
                                Image(systemName: "trash")
                            }
                            .buttonStyle(.borderless)
                        }
                        }
                        ForEach(model.marketRules) { rule in
                            HStack(spacing: 12) {
                                VStack(alignment: .leading, spacing: 3) {
                                    Text("全部 USDT 永续").font(.headline)
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
                                Button(role: .destructive) { model.deleteMarketRule(id: rule.id) } label: {
                                    Image(systemName: "trash")
                                }.buttonStyle(.borderless)
                            }
                        }
                    }
                }
            }
            .padding(20)
            .tabItem { Label("预警", systemImage: "bell") }

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
                Spacer()
            }
            .padding(20)
            .tabItem { Label("自检", systemImage: "stethoscope") }
        }
        .frame(width: 620, height: 460)
        .onAppear {
            selection = Set(model.menuSymbols)
            ruleSymbol = model.primarySymbol
        }
    }

    private var filteredContracts: [Contract] {
        model.orderedContracts(query: contractQuery)
    }

    private func saveRule() {
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
                                    Text(contract.symbol).foregroundStyle(.primary)
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
