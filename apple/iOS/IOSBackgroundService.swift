import Foundation
import PriceCore

enum IOSBackgroundServiceError: LocalizedError {
    case missingServerURL

    var errorDescription: String? {
        switch self {
        case .missingServerURL:
            "未配置 iPhone 后台监控服务地址"
        }
    }
}

actor IOSBackgroundService {
    struct ForegroundState: Sendable {
        let rules: [AlertRule]
        let events: [IOSBackgroundEvent]
    }

    private static let versionKey = "iosBackgroundRuleVersion"
    private var version = Int64(UserDefaults.standard.integer(forKey: versionKey))
    private var foreground = false

    func enterForeground(rules: [AlertRule], monitoringEnabled: Bool, primarySymbol: String) async throws -> ForegroundState {
        foreground = true
        let api = try await authenticatedClient()
        let remote = try await api.iosRules()
        if let remote {
            version = max(version, remote.version)
        }
        let reconciled = reconcile(local: rules, remote: remote?.rules ?? [])
        try await sync(rules: reconciled, monitoringEnabled: monitoringEnabled, primarySymbol: primarySymbol)
        let events = try await api.iosEvents()
        return ForegroundState(rules: reconciled, events: events)
    }

    func enterBackground() {
        foreground = false
    }

    func sync(rules: [AlertRule], monitoringEnabled: Bool, primarySymbol: String) async throws {
        let api = try await authenticatedClient()
        let symbols = Array(Set(rules.filter(\.isEnabled).map(\.symbol) + [primarySymbol])).sorted()
        try await api.setSubscriptions(symbols)
        version = max(version + 1, Int64(Date().timeIntervalSince1970 * 1_000))
        try await api.setIOSRules(version: version, monitoringEnabled: monitoringEnabled, rules: rules)
        UserDefaults.standard.set(version, forKey: Self.versionKey)
        if foreground {
            try await api.renewIOSLease(version: version)
        }
    }

    func renewForegroundLease() async throws {
        guard foreground, version > 0 else { return }
        try await authenticatedClient().renewIOSLease(version: version)
    }

    func registerPushToken(_ token: String) async throws {
        try await authenticatedClient().setIOSPushToken(token, environment: pushEnvironment)
    }

    func registerLiveActivity(id: String, token: String, symbol: String, expiresAt: Int64) async throws {
        try await authenticatedClient().setIOSLiveActivity(
            activityID: id, pushToken: token, symbol: symbol,
            environment: pushEnvironment, expiresAt: expiresAt
        )
    }

    func deleteLiveActivity(id: String) async throws {
        try await authenticatedClient().deleteIOSLiveActivity(activityID: id)
    }

    func acknowledge(events: [IOSBackgroundEvent]) async throws {
        let api = try await authenticatedClient()
        for event in events {
            try await api.acknowledgeIOSEvent(event.id)
        }
    }

    private func authenticatedClient() async throws -> APIClient {
        try await ServerConnectionService.shared.authenticatedClient(platform: "ios", displayName: "iPhone")
    }

    private func reconcile(local: [AlertRule], remote: [AlertRule]) -> [AlertRule] {
        let remoteByID = Dictionary(uniqueKeysWithValues: remote.map { ($0.id, $0) })
        return local.map { item in
            guard let server = remoteByID[item.id],
                  server.symbol == item.symbol,
                  server.windowMinutes == item.windowMinutes,
                  server.threshold == item.threshold else { return item }
            var result = item
            result.riseTriggered = server.riseTriggered
            result.fallTriggered = server.fallTriggered
            return result
        }
    }

    private var pushEnvironment: String {
        #if DEBUG
        "sandbox"
        #else
        "production"
        #endif
    }
}
