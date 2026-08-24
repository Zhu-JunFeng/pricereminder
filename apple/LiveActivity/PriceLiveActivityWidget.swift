import ActivityKit
import SwiftUI
import WidgetKit

@main
struct PriceLiveActivityBundle: WidgetBundle {
    var body: some Widget { PriceLiveActivityWidget() }
}

struct PriceLiveActivityWidget: Widget {
    var body: some WidgetConfiguration {
        ActivityConfiguration(for: PriceActivityAttributes.self) { context in
            HStack(spacing: 12) {
                VStack(alignment: .leading, spacing: 3) {
                    Text(context.state.symbol).font(.headline)
                    Text("币安最新成交价").font(.caption).foregroundStyle(.secondary)
                }
                Spacer()
                Text(context.state.price)
                    .font(.title3.weight(.semibold)).monospacedDigit()
            }
            .padding(.horizontal, 16)
            .activityBackgroundTint(Color(uiColor: .systemBackground))
            .activitySystemActionForegroundColor(.primary)
        } dynamicIsland: { context in
            DynamicIsland {
                DynamicIslandExpandedRegion(.leading) {
                    Text(context.state.symbol).font(.headline)
                }
                DynamicIslandExpandedRegion(.trailing) {
                    Text(context.state.price).font(.headline).monospacedDigit()
                }
                DynamicIslandExpandedRegion(.bottom) {
                    HStack {
                        Text("币安最新成交价")
                        Spacer()
                        Text(Date(timeIntervalSince1970: Double(context.state.eventTime) / 1000), style: .time)
                    }
                    .font(.caption).foregroundStyle(.secondary)
                }
            } compactLeading: {
                Image(systemName: "waveform.path.ecg")
                    .foregroundStyle(Color(red: 0, green: 0.50, blue: 0.54))
            } compactTrailing: {
                Text(context.state.price).font(.caption2.weight(.semibold)).monospacedDigit()
            } minimal: {
                Image(systemName: "waveform.path.ecg")
                    .foregroundStyle(Color(red: 0, green: 0.50, blue: 0.54))
            }
        }
    }
}
