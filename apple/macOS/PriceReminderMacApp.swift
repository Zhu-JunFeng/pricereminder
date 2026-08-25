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
        let values = model.menuSymbols.prefix(3).map { symbol in
            model.prices[symbol].map { "\(symbol.replacingOccurrences(of: "USDT", with: "")) \($0.priceText)" } ?? "\(symbol) --"
        }
        Text(values.joined(separator: "  ·  "))
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
                    Text(model.prices[symbol]?.priceText ?? "--")
                        .font(.title3.weight(.semibold)).numericPriceStyle()
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
                Text("同时支持滚动涨跌幅和目标价格提醒。")
                    .foregroundStyle(.secondary)

                Picker("提醒类型", selection: $ruleKind) {
                    Text("涨跌幅").tag(AlertRuleKind.percentage)
                    Text("目标价格").tag(AlertRuleKind.target)
                }
                .pickerStyle(.segmented)

                HStack(spacing: 10) {
                    Picker("合约", selection: $ruleSymbol) {
                        ForEach(model.contracts) { contract in
                            Text(contract.symbol).tag(contract.symbol)
                        }
                    }
                    .frame(maxWidth: 220)
                    if ruleKind == .percentage {
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
                    Button("添加") { addRule() }
                        .buttonStyle(.borderedProminent)
                }

                if let ruleError {
                    Text(ruleError).font(.caption).foregroundStyle(AppTheme.fall)
                }

                if model.rules.isEmpty {
                    ContentUnavailableView("还没有预警规则", systemImage: "bell.slash", description: Text("可添加涨跌幅或目标价格提醒。"))
                } else {
                    List(model.rules) { rule in
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
                            Button(role: .destructive) { model.deleteRule(id: rule.id) } label: {
                                Image(systemName: "trash")
                            }
                            .buttonStyle(.borderless)
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
        let query = contractQuery.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty else { return model.contracts }
        return model.contracts.filter {
            $0.symbol.localizedCaseInsensitiveContains(query)
                || $0.baseAsset.localizedCaseInsensitiveContains(query)
                || $0.quoteAsset.localizedCaseInsensitiveContains(query)
        }
    }

    private func addRule() {
        do {
            if ruleKind == .percentage {
                try model.addRule(symbol: ruleSymbol, windowMinutes: ruleWindow, threshold: ruleThreshold)
            } else {
                try model.addTargetRule(symbol: ruleSymbol, direction: targetDirection, targetPrice: targetPrice)
            }
            ruleError = nil
        } catch {
            ruleError = error.localizedDescription
        }
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
