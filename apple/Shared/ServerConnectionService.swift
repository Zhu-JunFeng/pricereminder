import Foundation
import PriceCore

enum ServerConnectionError: LocalizedError {
    case missingServerURL

    var errorDescription: String? { "未配置行情中继服务地址" }
}

actor ServerConnectionService {
    static let shared = ServerConnectionService()

    private var client: APIClient?

    func authenticatedClient(platform: String, displayName: String) async throws -> APIClient {
        if let client { return client }
        let baseURL = try Self.serverURL()
        if let token = KeychainStore.token() {
            let existing = APIClient(baseURL: baseURL, token: token)
            if try await existing.refreshDevice() {
                client = existing
                return existing
            }
            KeychainStore.deleteToken()
        }
        let created = APIClient(baseURL: baseURL)
        let registration = try await created.register(platform: platform, displayName: displayName)
        try KeychainStore.save(token: registration.token)
        await created.setToken(registration.token)
        client = created
        return created
    }

    func contracts(platform: String, displayName: String) async throws -> [Contract] {
        try await authenticatedClient(platform: platform, displayName: displayName).contracts()
    }

    func prepareStream(
        symbols: [String], platform: String, displayName: String
    ) async throws -> (url: URL, token: String) {
        let api = try await authenticatedClient(platform: platform, displayName: displayName)
        try await api.setSubscriptions(symbols)
        return (try await api.streamURL(), try await api.bearerToken())
    }

    static func serverURL() throws -> URL {
        guard let value = Bundle.main.object(forInfoDictionaryKey: "PriceReminderServerURL") as? String,
              !value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty,
              let url = URL(string: value), url.scheme == "https" else {
            throw ServerConnectionError.missingServerURL
        }
        return url
    }
}
