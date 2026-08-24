import SwiftUI

struct SettingsView: View {
    @EnvironmentObject private var model: AppModel

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
                LabeledContent("后台提醒", value: model.backgroundStatusMessage)
            }
            Section {
                Text("前台由本机直连币安；进入后台后由服务端继续计算规则，并通过 APNs 发送提醒。规则、价格缓存和触发历史仍保存在此设备。")
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
        .navigationTitle("设置")
    }
}
