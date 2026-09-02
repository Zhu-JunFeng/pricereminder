import PriceCore
import SwiftUI

struct RulesView: View {
    @EnvironmentObject private var model: AppModel
    @State private var showingAdd = false
    @State private var editingRule: AlertRule?
    @State private var editingMarketRule: MarketAlertRule?

    var body: some View {
        List {
            Section {
                if model.rules.isEmpty && model.marketRules.isEmpty {
                    ContentUnavailableView("还没有预警规则", systemImage: "bell.slash", description: Text("可添加单合约、全市场或目标价格提醒。"))
                } else {
                    ForEach(model.rules) { rule in
                        HStack(spacing: 12) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(rule.symbol).font(.headline)
                                Text(ruleDescription(rule))
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Toggle("", isOn: Binding(
                                get: { rule.isEnabled },
                                set: { model.setRuleEnabled(id: rule.id, enabled: $0) }
                            )).labelsHidden()
                            Button { editingRule = rule } label: {
                                Image(systemName: "pencil")
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel("编辑 \(rule.symbol) 提醒")
                        }
                        .swipeActions { Button("删除", role: .destructive) { model.deleteRule(id: rule.id) } }
                    }
                    ForEach(model.marketRules) { rule in
                        HStack(spacing: 12) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text("全部 USDT 永续").font(.headline)
                                Text("\(rule.windowMinutes) 分钟内涨跌 ≥ \(rule.thresholdText)%")
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Toggle("", isOn: Binding(
                                get: { rule.isEnabled },
                                set: { model.setMarketRuleEnabled(id: rule.id, enabled: $0) }
                            )).labelsHidden()
                            Button { editingMarketRule = rule } label: { Image(systemName: "pencil") }
                                .buttonStyle(.plain)
                                .accessibilityLabel("编辑全市场提醒")
                        }
                        .swipeActions { Button("删除", role: .destructive) { model.deleteMarketRule(id: rule.id) } }
                    }
                }
            } header: { Text("单合约、全市场与目标价格 · \(model.rules.count + model.marketRules.count)/50") }

            Section("最近触发") {
                if model.history.isEmpty {
                    Text("暂无触发记录。历史只保存在本机，并在 30 天后清理。")
                        .font(.caption).foregroundStyle(.secondary)
                } else {
                    ForEach(model.history.prefix(50)) { event in
                        HStack(spacing: 10) {
                            Image(systemName: event.direction == .rise ? "arrow.up.right" : "arrow.down.right")
                                .foregroundStyle(event.direction == .rise ? AppTheme.rise : AppTheme.fall)
                            VStack(alignment: .leading, spacing: 3) {
                                Text(eventTitle(event)).font(.headline)
                                Text(eventDescription(event))
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Text(Date(timeIntervalSince1970: Double(event.eventTime) / 1000), style: .relative)
                                .font(.caption2).foregroundStyle(.secondary)
                        }
                    }
                }
            }
        }
        .navigationTitle("价格预警")
        .toolbar { Button { showingAdd = true } label: { Label("添加", systemImage: "plus") } }
        .sheet(isPresented: $showingAdd) { RuleEditorView() }
        .sheet(item: $editingRule) { RuleEditorView(rule: $0) }
        .sheet(item: $editingMarketRule) { RuleEditorView(marketRule: $0) }
    }

    private func ruleDescription(_ rule: AlertRule) -> String {
        if rule.kind == .target {
            let direction = rule.targetDirection == .above ? "达到或高于" : "达到或低于"
            return "\(direction) \(rule.targetPriceText ?? "--")"
        }
        return "\(rule.windowMinutes) 分钟内上涨或下跌 ≥ \(rule.thresholdText)%"
    }

    private func eventTitle(_ event: TriggerRecord) -> String {
        event.kind == .target ? "\(event.symbol) · 目标价" : "\(event.symbol) · \(event.windowMinutes)分钟"
    }

    private func eventDescription(_ event: TriggerRecord) -> String {
        if event.kind == .target {
            let direction = event.direction == .rise ? "达到或高于" : "达到或低于"
            return "\(direction) \(event.targetPriceText ?? "--") · \(event.priceText)"
        }
        return "\((event.changePercent ?? .zero).formatted(.number.precision(.fractionLength(2))))% · \(event.priceText)"
    }
}

private struct RuleEditorView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var symbol = "BTCUSDT"
    @State private var window = 5
    @State private var threshold = "3"
    @State private var kind: AlertRuleKind = .percentage
    @State private var targetDirection: TargetDirection = .above
    @State private var targetPrice = ""
    @State private var error: String?
    @State private var showingContracts = false
    private let editingID: UUID?
    private let editingMarketID: UUID?

    init(rule: AlertRule? = nil) {
        editingID = rule?.id
        editingMarketID = nil
        _symbol = State(initialValue: rule?.symbol ?? "BTCUSDT")
        _window = State(initialValue: rule?.windowMinutes ?? 5)
        _threshold = State(initialValue: rule?.thresholdText ?? "3")
        _kind = State(initialValue: rule?.kind ?? .percentage)
        _targetDirection = State(initialValue: rule?.targetDirection ?? .above)
        _targetPrice = State(initialValue: rule?.targetPriceText ?? "")
    }

    init(marketRule: MarketAlertRule) {
        editingID = nil
        editingMarketID = marketRule.id
        _symbol = State(initialValue: "BTCUSDT")
        _window = State(initialValue: marketRule.windowMinutes)
        _threshold = State(initialValue: marketRule.thresholdText)
        _kind = State(initialValue: .marketPercentage)
        _targetDirection = State(initialValue: .above)
        _targetPrice = State(initialValue: "")
    }

    var body: some View {
        NavigationStack {
            Form {
                Picker("提醒类型", selection: $kind) {
                    if editingMarketID != nil {
                        Text("全市场").tag(AlertRuleKind.marketPercentage)
                    } else {
                        Text("单合约").tag(AlertRuleKind.percentage)
                        if editingID == nil {
                            Text("全市场").tag(AlertRuleKind.marketPercentage)
                        }
                        Text("目标价格").tag(AlertRuleKind.target)
                    }
                }
                .pickerStyle(.segmented)
                if kind == .marketPercentage {
                    LabeledContent("范围", value: "币安全部 USDT 永续")
                } else {
                    Button { showingContracts = true } label: {
                        LabeledContent("合约") {
                            HStack(spacing: 6) {
                                Text(symbol)
                                Image(systemName: "chevron.up.chevron.down")
                                    .font(.caption)
                                    .foregroundStyle(.secondary)
                            }
                        }
                    }
                    .foregroundStyle(.primary)
                }
                if kind != .target {
                    Stepper("时间窗口：\(window) 分钟", value: $window, in: 1...60)
                    TextField("变化阈值 (%)", text: $threshold).keyboardType(.decimalPad)
                } else {
                    Picker("触发条件", selection: $targetDirection) {
                        Text("达到或高于").tag(TargetDirection.above)
                        Text("达到或低于").tag(TargetDirection.below)
                    }
                    TextField("目标价格", text: $targetPrice).keyboardType(.decimalPad)
                }
                if let error { Text(error).font(.caption).foregroundStyle(AppTheme.fall) }
                Section {
                    Text(helpText)
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
            .navigationTitle(editingID == nil && editingMarketID == nil ? "添加预警" : "编辑预警")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("保存") {
                        do {
                            if let editingMarketID {
                                try model.updateMarketRule(
                                    id: editingMarketID, windowMinutes: window, threshold: threshold
                                )
                            } else if let editingID {
                                try model.updateRule(
                                    id: editingID, symbol: symbol, kind: kind,
                                    windowMinutes: window, threshold: threshold,
                                    direction: targetDirection, targetPrice: targetPrice
                                )
                            } else if kind == .marketPercentage {
                                try model.addMarketRule(windowMinutes: window, threshold: threshold)
                            } else if kind == .percentage {
                                try model.addRule(symbol: symbol, windowMinutes: window, threshold: threshold)
                            } else {
                                try model.addTargetRule(symbol: symbol, direction: targetDirection, targetPrice: targetPrice)
                            }
                            dismiss()
                        }
                        catch { self.error = error.localizedDescription }
                    }
                }
            }
            .onAppear {
                if editingID == nil && editingMarketID == nil { symbol = model.primarySymbol }
            }
            .sheet(isPresented: $showingContracts) {
                ContractPickerView(selected: symbol, contracts: model.contracts) { symbol = $0 }
            }
        }
    }

    private var helpText: String {
        switch kind {
        case .percentage:
            "当前价相对 N 分钟前上涨或下跌达到阈值时提醒；两个方向独立重新武装。"
        case .marketPercentage:
            "扫描全部 USDT 永续；窗口内相对最低价上涨或最高价下跌达到阈值时立即提醒。"
        case .target:
            "当前价进入目标区间时提醒一次；离开目标区间后自动重新武装。"
        }
    }
}
