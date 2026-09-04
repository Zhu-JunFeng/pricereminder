import PriceCore
import SwiftUI

struct MarketView: View {
    @EnvironmentObject private var model: AppModel
    @State private var showingContracts = false
    @State private var showingEntryPriceEditor = false

    private var price: PricePoint? { model.prices[model.primarySymbol] }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 0) {
                Button { showingContracts = true } label: {
                    HStack {
                        VStack(alignment: .leading, spacing: 4) {
                            Text("主合约").font(.caption).foregroundStyle(.secondary)
                            Text(model.primarySymbol).font(.headline).foregroundStyle(.primary)
                        }
                        Spacer()
                        Image(systemName: "chevron.up.chevron.down").foregroundStyle(.secondary)
                    }
                    .padding(14)
                    .background(AppTheme.surface, in: RoundedRectangle(cornerRadius: 8))
                }
                .buttonStyle(.plain)
                .padding(.bottom, 28)

                Text(model.primarySymbol)
                    .font(.subheadline.weight(.medium)).foregroundStyle(.secondary)
                Text(price?.priceText ?? "--")
                    .font(.system(size: 42, weight: .semibold, design: .rounded))
                    .numericPriceStyle()
                    .contentTransition(.numericText())
                Text(price.map { "币安最新成交价 · \(Date(timeIntervalSince1970: Double($0.eventTime) / 1000).formatted(date: .omitted, time: .standard))" } ?? "等待第一条价格")
                    .font(.caption).foregroundStyle(.secondary).padding(.top, 3)

                EntryPriceSummary(symbol: model.primarySymbol) {
                    showingEntryPriceEditor = true
                }
                .padding(.top, 16)

                Divider().padding(.vertical, 24)
                HStack(alignment: .center, spacing: 14) {
                    Circle().fill(stateColor).frame(width: 9, height: 9)
                    VStack(alignment: .leading, spacing: 3) {
                        Text(stateTitle).font(.headline)
                        Text(model.statusMessage).font(.caption).foregroundStyle(.secondary)
                    }
                    Spacer()
                    if model.monitoringEnabled {
                        Button("停止") { model.stopMonitoring() }.buttonStyle(.bordered)
                    } else {
                        Button("开始监控") { Task { await model.startMonitoring() } }.buttonStyle(.borderedProminent)
                    }
                }

                #if os(iOS)
                Divider().padding(.vertical, 24)
                VStack(alignment: .leading, spacing: 10) {
                    Text("灵动岛与锁屏").font(.headline)
                    Text("App 活跃时每 15 秒本地更新；进入后台后停止更新，后台实时活动需要服务端。")
                        .font(.caption).foregroundStyle(.secondary)
                    HStack {
                        Button("显示实时价格") { Task { await model.startLiveActivity() } }
                            .buttonStyle(.borderedProminent)
                        Button("结束") { Task { await model.stopLiveActivity() } }
                            .buttonStyle(.bordered)
                    }
                    if let status = model.liveActivityStatusMessage {
                        Text(status).font(.caption).foregroundStyle(.secondary)
                    }
                }
                #endif
            }
            .padding(20)
        }
        .navigationTitle("实时价格")
        .sheet(isPresented: $showingContracts) { ContractPickerView(selected: model.primarySymbol, contracts: model.contracts, onSelect: model.selectPrimary) }
        .sheet(isPresented: $showingEntryPriceEditor) {
            EntryPriceEditor(symbol: model.primarySymbol)
        }
    }

    private var stateColor: Color {
        switch model.monitorState {
        case .live: AppTheme.primary
        case .warmingUp, .connecting: .orange
        case .stale, .notificationUnavailable: AppTheme.fall
        case .disconnected: .secondary
        }
    }

    private var stateTitle: String {
        switch model.monitorState {
        case .live: "实时监控中"
        case .warmingUp: "数据积累中"
        case .connecting: "正在连接"
        case .stale: "价格已陈旧"
        case .notificationUnavailable: "通知不可用"
        case .disconnected: "监控未连接"
        }
    }
}

private struct EntryPriceSummary: View {
    @EnvironmentObject private var model: AppModel
    let symbol: String
    let edit: () -> Void

    var body: some View {
        Button(action: edit) {
            HStack(spacing: 8) {
                if let setting = model.entryPrices[symbol] {
                    Text("\(setting.positionSide.displayName) · 开仓 \(setting.priceText)")
                        .foregroundStyle(.secondary)
                        .numericPriceStyle()
                    if let change = model.entryPriceChange(for: symbol) {
                        TimelineView(.periodic(from: .now, by: 1)) { context in
                            let stale = model.prices[symbol].map {
                                model.isPriceStale($0, now: context.date)
                            } ?? true
                            Text(stale ? "\(change.percentageText) · 价格已陈旧" : change.percentageText)
                                .foregroundStyle(stale ? Color.secondary : color(for: change.direction))
                                .numericPriceStyle()
                        }
                    }
                } else {
                    Text("设置开仓价").foregroundStyle(AppTheme.primary)
                }
                Spacer()
                Image(systemName: "pencil").font(.caption).foregroundStyle(.secondary)
            }
            .font(.subheadline.weight(.medium))
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(model.entryPrices[symbol] == nil ? "设置开仓价" : "编辑开仓价")
    }

    private func color(for direction: EntryPriceDirection) -> Color {
        switch direction {
        case .rise: AppTheme.rise
        case .fall: AppTheme.fall
        case .flat: .secondary
        }
    }
}

private struct EntryPriceEditor: View {
    @EnvironmentObject private var model: AppModel
    @Environment(\.dismiss) private var dismiss
    let symbol: String
    @State private var priceText = ""
    @State private var positionSide: PositionSide = .long
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Picker("仓位方向", selection: $positionSide) {
                        Text("多单").tag(PositionSide.long)
                        Text("空单").tag(PositionSide.short)
                    }
                    .pickerStyle(.segmented)
                    TextField("开仓价格", text: $priceText)
                        .keyboardType(.decimalPad)
                        .numericPriceStyle()
                    Button("使用当前价") { useCurrentPrice() }
                        .disabled(!canUseCurrentPrice)
                } header: {
                    Text(symbol)
                } footer: {
                    Text("按所选多空方向计算价格收益率，不包含杠杆和仓位数量。")
                }

                if let errorMessage {
                    Section { Text(errorMessage).foregroundStyle(AppTheme.fall) }
                }

                if model.entryPrices[symbol] != nil {
                    Section {
                        Button("清除开仓价", role: .destructive) {
                            model.clearEntryPrice(symbol: symbol)
                            dismiss()
                        }
                    }
                }
            }
            .navigationTitle("开仓参考价")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("保存") { save() } }
            }
            .onAppear {
                priceText = model.entryPrices[symbol]?.priceText ?? ""
                positionSide = model.entryPrices[symbol]?.positionSide ?? .long
            }
        }
        .presentationDetents([.medium])
    }

    private var canUseCurrentPrice: Bool {
        guard let price = model.prices[symbol] else { return false }
        return !model.isPriceStale(price)
    }

    private func useCurrentPrice() {
        do {
            try model.useCurrentPriceAsEntry(symbol: symbol, positionSide: positionSide)
            priceText = model.entryPrices[symbol]?.priceText ?? ""
            errorMessage = nil
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    private func save() {
        do {
            try model.setEntryPrice(
                symbol: symbol, priceText: priceText, positionSide: positionSide
            )
            dismiss()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

struct ContractPickerView: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var model: AppModel
    @State private var query = ""
    let selected: String
    let contracts: [Contract]
    let onSelect: (String) -> Void

    private var filtered: [Contract] {
        ContractOrdering.ordered(contracts, recentSymbols: model.recentSymbols, query: query)
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                HStack(spacing: 10) {
                    Image(systemName: "magnifyingglass")
                        .foregroundStyle(.secondary)
                    TextField("输入合约代码，如 BTC", text: $query)
                        .textInputAutocapitalization(.characters)
                        .autocorrectionDisabled()
                    if !query.isEmpty {
                        Button { query = "" } label: {
                            Image(systemName: "xmark.circle.fill")
                                .foregroundStyle(.secondary)
                        }
                        .buttonStyle(.plain)
                    }
                }
                .padding(.horizontal, 14)
                .frame(minHeight: 46)
                .background(AppTheme.surface, in: RoundedRectangle(cornerRadius: 8))
                .padding(.horizontal, 16)
                .padding(.vertical, 10)

                List(filtered) { contract in
                    Button {
                        model.recordRecentSymbol(contract.symbol)
                        onSelect(contract.symbol)
                        dismiss()
                    } label: {
                        HStack {
                            VStack(alignment: .leading) {
                                Text(contract.symbol).foregroundStyle(.primary)
                                Text("\(contract.baseAsset) / \(contract.quoteAsset)").font(.caption).foregroundStyle(.secondary)
                            }
                            Spacer()
                            if contract.symbol == selected { Image(systemName: "checkmark").foregroundStyle(AppTheme.primary) }
                        }
                    }
                }
                .overlay {
                    if !query.isEmpty, filtered.isEmpty {
                        ContentUnavailableView.search(text: query)
                    }
                }
            }
            .navigationTitle("选择 U 本位永续")
            .toolbar { ToolbarItem(placement: .cancellationAction) { Button("取消") { dismiss() } } }
        }
    }
}
