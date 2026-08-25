import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel
    @State private var testResult: String?

    var body: some View {
        List {
            Section("行情") {
                LabeledContent("来源", value: "币安 U 本位永续")
                LabeledContent("连接", value: "终端直连")
                LabeledContent("价格", value: "最新成交价 · 每秒")
                LabeledContent("保留", value: "最近 1 小时")
            }
            Section("运行状态") {
                LabeledContent("状态", value: model.statusMessage)
                LabeledContent("连接路径", value: model.connectionSourceLabel)
                LabeledContent("订阅合约", value: "\(model.subscribedSymbolCount) 个")
                LabeledContent("行情延迟", value: model.latestPriceDelayText)
                LabeledContent("最后接收", value: lastReceivedText)
                LabeledContent("重连次数", value: "\(model.reconnectCount)")
                LabeledContent("通知权限", value: model.notificationStatusLabel)
                LabeledContent("后台提醒", value: model.backgroundStatusMessage)
                if let error = model.lastConnectionError {
                    LabeledContent("最近错误", value: error)
                }
            }
            Section("自检") {
                Button("刷新状态") { Task { await model.refreshSelfCheck() } }
                Button("发送测试通知") {
                    Task {
                        testResult = await model.sendTestNotification()
                            ? "测试通知已发送"
                            : "通知权限未开启，请前往系统设置允许通知"
                    }
                }
                if let testResult {
                    Text(testResult).font(.caption).foregroundStyle(.secondary)
                }
            }
            Section {
                Text("前台由本机直连币安；进入后台后由服务端继续计算规则，并通过 APNs 发送提醒。规则、价格缓存和触发历史仍保存在此设备。")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
        .navigationTitle("设置")
        .task { await model.refreshSelfCheck() }
    }

    private var lastReceivedText: String {
        guard let date = model.lastPriceReceivedAt else { return "尚未收到" }
        return date.formatted(date: .omitted, time: .standard)
    }
}
