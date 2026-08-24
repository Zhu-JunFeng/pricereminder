import PriceCore
import SwiftUI

struct RulesView: View {
    @EnvironmentObject private var model: AppModel
    @State private var showingAdd = false

    var body: some View {
        List {
            Section {
                if model.rules.isEmpty {
                    ContentUnavailableView("还没有预警规则", systemImage: "bell.slash", description: Text("添加规则后，监控会先积累完整时间窗口。"))
                } else {
                    ForEach(model.rules) { rule in
                        HStack(spacing: 12) {
                            VStack(alignment: .leading, spacing: 4) {
                                Text(rule.symbol).font(.headline)
                                Text("\(rule.windowMinutes) 分钟内上涨或下跌 ≥ \(rule.thresholdText)%")
                                    .font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            Toggle("", isOn: Binding(
                                get: { rule.isEnabled },
                                set: { model.setRuleEnabled(id: rule.id, enabled: $0) }
                            )).labelsHidden()
                        }
                        .swipeActions { Button("删除", role: .destructive) { model.deleteRule(id: rule.id) } }
                    }
                }
            } header: { Text("滚动窗口双向提醒 · \(model.rules.count)/50") }

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
                                Text("\(event.symbol) · \(event.windowMinutes)分钟").font(.headline)
                                Text("\(event.changePercent.formatted(.number.precision(.fractionLength(2))))% · \(event.priceText)")
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
        .sheet(isPresented: $showingAdd) { AddRuleView() }
    }
}

private struct AddRuleView: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    @State private var symbol = "BTCUSDT"
    @State private var window = 5
    @State private var threshold = "3"
    @State private var error: String?
    @State private var showingContracts = false

    var body: some View {
        NavigationStack {
            Form {
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
                Stepper("时间窗口：\(window) 分钟", value: $window, in: 1...60)
                TextField("变化阈值", text: $threshold).keyboardType(.decimalPad)
                if let error { Text(error).font(.caption).foregroundStyle(AppTheme.fall) }
                Section {
                    Text("当前价相对 N 分钟前上涨或下跌达到阈值时提醒；两个方向独立重新武装。")
                        .font(.caption).foregroundStyle(.secondary)
                }
            }
            .navigationTitle("添加预警")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("保存") {
                        do { try model.addRule(symbol: symbol, windowMinutes: window, threshold: threshold); dismiss() }
                        catch { self.error = error.localizedDescription }
                    }
                }
            }
            .onAppear { symbol = model.primarySymbol }
            .sheet(isPresented: $showingContracts) {
                ContractPickerView(selected: symbol, contracts: model.contracts) { symbol = $0 }
            }
        }
    }
}
