import SwiftUI

@main
struct PriceReminderMacHarnessApp: App {
    @StateObject private var model = AppModel()

    var body: some Scene {
        WindowGroup("币价提醒验收") {
            MenuPanel()
                .environmentObject(model)
                .task {
                    await model.bootstrap(platform: "macos")
                }
        }
        .defaultSize(width: 340, height: 300)

        Settings {
            MacSettingsView().environmentObject(model)
        }
    }
}
