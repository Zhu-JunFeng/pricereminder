import Foundation

public actor APIClient {
    public struct IOSRuleSnapshot: Codable, Sendable {
        public let version: Int64
        public let monitoringEnabled: Bool
        public let rules: [AlertRule]

        public init(version: Int64, monitoringEnabled: Bool, rules: [AlertRule]) {
            self.version = version
            self.monitoringEnabled = monitoringEnabled
            self.rules = rules
        }
    }

    public struct Registration: Codable, Sendable {
        public let deviceId: UUID
        public let token: String
        public let expiresAt: Int64
    }

    private struct ContractsResponse: Codable { let contracts: [Contract] }
    private struct SubscriptionsResponse: Codable { let symbols: [String] }
    private struct IOSRulesRequest: Encodable {
        let version: Int64
        let monitoringEnabled: Bool
        let rules: [AlertRule]
    }
    private struct IOSLeaseRequest: Encodable { let version: Int64 }
    private struct IOSEventsResponse: Decodable { let events: [IOSBackgroundEvent] }
    private struct ExpiryResponse: Decodable { let expiresAt: Int64 }

    private let baseURL: URL
    private let session: URLSession
    private var token: String?

    public init(baseURL: URL, token: String? = nil, session: URLSession = .shared) {
        self.baseURL = baseURL
        self.token = token
        self.session = session
    }

    public func setToken(_ token: String) { self.token = token }

    public func refreshDevice() async throws -> Bool {
        let url = try endpoint("/v1/devices/refresh")
        var request = URLRequest(url: url)
        request.httpMethod = "POST"
        request.setValue("Bearer \(try bearerToken())", forHTTPHeaderField: "Authorization")
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw PriceCoreError.invalidServerResponse }
        if http.statusCode == 401 { return false }
        guard 200..<300 ~= http.statusCode else { throw PriceCoreError.invalidServerResponse }
        _ = try JSONDecoder().decode(ExpiryResponse.self, from: data)
        return true
    }

    public func register(platform: String, displayName: String) async throws -> Registration {
        try await request(
            path: "/v1/devices/register", method: "POST",
            body: try JSONEncoder().encode(["platform": platform, "displayName": displayName]), authenticated: false
        )
    }

    public func contracts() async throws -> [Contract] {
        let response: ContractsResponse = try await request(path: "/v1/contracts")
        return response.contracts
    }

    public func subscriptions() async throws -> [String] {
        let response: SubscriptionsResponse = try await request(path: "/v1/subscriptions")
        return response.symbols
    }

    public func setSubscriptions(_ symbols: [String]) async throws {
        let _: SubscriptionsResponse = try await request(
            path: "/v1/subscriptions", method: "PUT",
            body: try JSONEncoder().encode(["symbols": symbols])
        )
    }

    public func setIOSRules(version: Int64, monitoringEnabled: Bool, rules: [AlertRule]) async throws {
        let _: VersionResponse = try await request(
            path: "/v1/ios/rules", method: "PUT",
            body: try JSONEncoder().encode(IOSRulesRequest(version: version, monitoringEnabled: monitoringEnabled, rules: rules))
        )
    }

    public func iosRules() async throws -> IOSRuleSnapshot? {
        let url = try endpoint("/v1/ios/rules")
        var request = URLRequest(url: url)
        request.setValue("Bearer \(try bearerToken())", forHTTPHeaderField: "Authorization")
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse else { throw PriceCoreError.invalidServerResponse }
        if http.statusCode == 404 { return nil }
        guard 200..<300 ~= http.statusCode else { throw PriceCoreError.invalidServerResponse }
        return try JSONDecoder().decode(IOSRuleSnapshot.self, from: data)
    }

    public func renewIOSLease(version: Int64) async throws {
        let _: LeaseResponse = try await request(
            path: "/v1/ios/lease", method: "POST",
            body: try JSONEncoder().encode(IOSLeaseRequest(version: version))
        )
    }

    public func setIOSPushToken(_ token: String, environment: String) async throws {
        let _: SavedResponse = try await request(
            path: "/v1/ios/push-token", method: "PUT",
            body: try JSONEncoder().encode(["token": token, "environment": environment])
        )
    }

    public func setIOSLiveActivity(
        activityID: String, pushToken: String, symbol: String, environment: String, expiresAt: Int64
    ) async throws {
        struct Body: Encodable {
            let activityId: String
            let pushToken: String
            let symbol: String
            let environment: String
            let expiresAt: Int64
        }
        let _: SavedResponse = try await request(
            path: "/v1/ios/live-activities", method: "PUT",
            body: try JSONEncoder().encode(Body(activityId: activityID, pushToken: pushToken, symbol: symbol, environment: environment, expiresAt: expiresAt))
        )
    }

    public func deleteIOSLiveActivity(activityID: String) async throws {
        try await requestWithoutResponse(path: "/v1/ios/live-activities/\(activityID)", method: "DELETE")
    }

    public func iosEvents(after eventID: String? = nil) async throws -> [IOSBackgroundEvent] {
        var path = "/v1/ios/events"
        if let eventID, let encoded = eventID.addingPercentEncoding(withAllowedCharacters: .urlQueryAllowed) {
            path += "?after=\(encoded)"
        }
        let response: IOSEventsResponse = try await request(path: path)
        return response.events
    }

    public func acknowledgeIOSEvent(_ eventID: String) async throws {
        try await requestWithoutResponse(path: "/v1/ios/events/\(eventID)/ack", method: "POST")
    }

    public func streamURL() throws -> URL {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false) else {
            throw PriceCoreError.invalidServerResponse
        }
        components.scheme = components.scheme == "https" ? "wss" : "ws"
        components.path = joinedPath("/v1/stream")
        guard let url = components.url else { throw PriceCoreError.invalidServerResponse }
        return url
    }

    public func bearerToken() throws -> String {
        guard let token else { throw PriceCoreError.invalidServerResponse }
        return token
    }

    private func request<Response: Decodable>(
        path: String, method: String = "GET", body: Data? = nil,
        authenticated: Bool = true
    ) async throws -> Response {
        let url = try endpoint(path)
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if authenticated {
            request.setValue("Bearer \(try bearerToken())", forHTTPHeaderField: "Authorization")
        }
        request.httpBody = body
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw PriceCoreError.invalidServerResponse
        }
        return try JSONDecoder().decode(Response.self, from: data)
    }

    private func requestWithoutResponse(path: String, method: String) async throws {
        let url = try endpoint(path)
        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("Bearer \(try bearerToken())", forHTTPHeaderField: "Authorization")
        let (_, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, 200..<300 ~= http.statusCode else {
            throw PriceCoreError.invalidServerResponse
        }
    }

    private struct VersionResponse: Decodable { let version: Int64 }
    private struct LeaseResponse: Decodable { let leaseUntil: Int64 }
    private struct SavedResponse: Decodable { let saved: Bool }

    private func endpoint(_ path: String) throws -> URL {
        guard var components = URLComponents(url: baseURL, resolvingAgainstBaseURL: false),
              let requested = URLComponents(string: path) else {
            throw PriceCoreError.invalidServerResponse
        }
        components.path = joinedPath(requested.path)
        components.percentEncodedQuery = requested.percentEncodedQuery
        guard let url = components.url else { throw PriceCoreError.invalidServerResponse }
        return url
    }

    private func joinedPath(_ path: String) -> String {
        baseURL.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
            .split(separator: "/")
            .map(String.init)
            .reduce("") { $0 + "/" + $1 } + "/" + path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
    }
}
