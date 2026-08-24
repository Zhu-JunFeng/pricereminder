import SwiftUI

struct RootView: View {
    var body: some View {
        TabView {
            NavigationStack { MarketView() }
                .tabItem { Label("行情", systemImage: "chart.line.uptrend.xyaxis") }
            NavigationStack { RulesView() }
                .tabItem { Label("预警", systemImage: "bell.badge") }
            NavigationStack { SettingsView() }
                .tabItem { Label("设置", systemImage: "gearshape") }
        }
        .tint(AppTheme.primary)
    }
}
