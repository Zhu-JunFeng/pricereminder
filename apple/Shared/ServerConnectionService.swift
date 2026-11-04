import Foundation
import PriceCore

enum ServerConnectionError: LocalizedError {
    case missingServerURL

    var errorDescription: String? { "未配置行情中继服务地址" }
}

actor ServerConnectionService {
    static let shared = ServerConnectionService()

    private var client: APIClient?
    private var authentication: Task<APIClient, Error>?
    private var authenticationID: UUID?
    private var credential: Task<String?, Error>?

    func authorize(platform: String, displayName: String) async throws -> APIClient {
        if let client { return client }
        if let authentication {
            do { return try await authentication.value } catch { }
            self.authentication = nil
            authenticationID = nil
        }
        credential = nil
        return try await authenticatedClient(platform: platform, displayName: displayName, allowInteraction: true)
    }

    func authenticatedClient(platform: String, displayName: String, allowInteraction: Bool = false) async throws -> APIClient {
        if let client { return client }
        if let authentication { return try await authentication.value }
        let baseURL = try Self.serverURL()
        let tokenTask: Task<String?, Error>
        if let credential { tokenTask = credential }
        else {
            tokenTask = Task.detached { try KeychainStore.token(allowInteraction: allowInteraction) }
            credential = tokenTask
        }
        let attempt = Task {
            if let token = try await tokenTask.value {
                let existing = APIClient(baseURL: baseURL, token: token)
                if try await existing.refreshDevice() { return existing }
            }
            let created = APIClient(baseURL: baseURL)
            let registration = try await created.register(platform: platform, displayName: displayName)
            try await Task.detached {
                try KeychainStore.save(token: registration.token, allowInteraction: allowInteraction)
            }.value
            await created.setToken(registration.token)
            return created
        }
        let id = UUID()
        authenticationID = id
        authentication = attempt
        defer {
            if authenticationID == id { authentication = nil; authenticationID = nil }
        }
        do {
            let result = try await attempt.value
            client = result
            return result
        } catch {
            // A denied/locked keychain must not create devices on every retry.
            if error is KeychainStore.Failure {
                credential = Task { throw error }
            }
            throw error
        }
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
