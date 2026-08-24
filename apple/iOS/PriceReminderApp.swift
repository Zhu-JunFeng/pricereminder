import SwiftUI

@main
struct PriceReminderApp: App {
    @UIApplicationDelegateAdaptor(AppDelegate.self) private var appDelegate
    @StateObject private var model = AppModel()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(model)
                .task {
                    await model.prepareIOSBackgroundDelivery()
                    await model.startMonitoring()
                    await model.bootstrap(platform: "ios")
                }
                .onReceive(NotificationCenter.default.publisher(for: .apnsTokenUpdated)) { notification in
                    guard let token = notification.object as? String else { return }
                    Task { await model.registerAPNSToken(token) }
                }
                .onReceive(NotificationCenter.default.publisher(for: .iosBackgroundEventReceived)) { _ in
                    Task { await model.fetchIOSBackgroundEvents() }
                }
        }
        .onChange(of: scenePhase) { _, phase in
            Task {
                if phase == .active { await model.enterForeground() }
                if phase == .background { await model.enterBackground() }
            }
        }
    }
}
