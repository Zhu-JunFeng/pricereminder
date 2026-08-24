import Foundation

public actor PriceStream {
    private final class CompletionGate: @unchecked Sendable {
        private let lock = NSLock()
        private var completed = false

        func claim() -> Bool {
            lock.lock()
            defer { lock.unlock() }
            guard !completed else { return false }
            completed = true
            return true
        }
    }

    private struct Message: Decodable {
        let eventType: String?
        let eventTime: Int64?
        let symbol: String?
        let price: String?
        let quantity: String?
        let code: Int?
        let message: String?

        enum CodingKeys: String, CodingKey {
            case eventType = "e"
            case eventTime = "E"
            case symbol = "s"
            case price = "p"
            case quantity = "q"
            case code
            case message = "msg"
        }
    }

    private struct CombinedMessage: Decodable {
        let data: Message
    }

    private struct ServerMessage: Decodable {
        let type: String
        let symbol: String?
        let price: String?
        let eventTime: Int64?
        let replay: Bool?
        let state: String?
        let reason: String?
        let serverTime: Int64?
    }

    private let streamURL: URL
    private let session: URLSession
    private var task: URLSessionWebSocketTask?

    public init(
        streamURL: URL = URL(string: "wss://fstream.binance.com/stream")!,
        session: URLSession = .shared
    ) {
        self.streamURL = streamURL
        self.session = session
    }

    public func connect(symbols: [String]) async throws -> AsyncThrowingStream<PriceStreamEvent, Error> {
        let symbols = Array(Set(symbols.map { $0.uppercased() })).sorted()
        guard !symbols.isEmpty, symbols.count <= 50,
              symbols.allSatisfy({ $0.range(of: "^[A-Z0-9]{2,30}$", options: .regularExpression) != nil }) else {
            throw PriceCoreError.invalidSubscription
        }

        guard var components = URLComponents(url: streamURL, resolvingAgainstBaseURL: false) else {
            throw PriceCoreError.invalidExchangeResponse
        }
        components.queryItems = [
            URLQueryItem(name: "streams", value: symbols.map { $0.lowercased() + "@trade" }.joined(separator: "/"))
        ]
        guard let url = components.url else { throw PriceCoreError.invalidExchangeResponse }

        task?.cancel(with: .goingAway, reason: nil)
        var request = URLRequest(url: url, timeoutInterval: 10)
        request.setValue("PriceReminder", forHTTPHeaderField: "User-Agent")
        let task = session.webSocketTask(with: request)
        self.task = task
        task.resume()

        try await ping(task)

        return eventStream(task: task) { data in
            try Self.decodeEvent(from: data).map(PriceStreamEvent.price)
        }
    }

    public func connect(
        serverURL: URL, token: String, lastEventTimes: [String: Int64]
    ) async throws -> AsyncThrowingStream<PriceStreamEvent, Error> {
        task?.cancel(with: .goingAway, reason: nil)
        var request = URLRequest(url: serverURL, timeoutInterval: 10)
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let task = session.webSocketTask(with: request)
        self.task = task
        task.resume()
        try await ping(task)
        let resume = try JSONEncoder().encode(ResumeMessage(type: "resume", lastEventTime: lastEventTimes))
        try await task.send(.data(resume))

        return eventStream(task: task, decoder: Self.decodeServerEvent)
    }

    private struct ResumeMessage: Encodable {
        let type: String
        let lastEventTime: [String: Int64]
    }

    private func eventStream(
        task: URLSessionWebSocketTask,
        decoder: @escaping @Sendable (Data) throws -> PriceStreamEvent?
    ) -> AsyncThrowingStream<PriceStreamEvent, Error> {
        return AsyncThrowingStream { continuation in
            Task {
                do {
                    while !Task.isCancelled {
                        let frame = try await task.receive()
                        let data: Data
                        switch frame {
                        case .data(let value): data = value
                        case .string(let value): data = Data(value.utf8)
                        @unknown default: continue
                        }
                        if let event = try decoder(data) {
                            continuation.yield(event)
                        }
                    }
                } catch {
                    continuation.finish(throwing: error)
                }
            }
        }
    }

    private func ping(_ task: URLSessionWebSocketTask) async throws {
        let gate = CompletionGate()
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            task.sendPing { error in
                guard gate.claim() else { return }
                if let error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume()
                }
            }
        }
    }

    public func disconnect() {
        task?.cancel(with: .normalClosure, reason: nil)
        task = nil
    }

    package static func decodeEvent(from data: Data) throws -> PricePoint? {
        let decoder = JSONDecoder()
        let message: Message
        if let combined = try? decoder.decode(CombinedMessage.self, from: data) {
            message = combined.data
        } else {
            message = try decoder.decode(Message.self, from: data)
        }
        if message.code != nil {
            throw PriceCoreError.invalidExchangeResponse
        }
        guard message.eventType == "trade",
              let symbol = message.symbol,
              let price = message.price,
              price != "0",
              message.quantity != "0",
              let eventTime = message.eventTime else {
            return nil
        }
        return try PricePoint(symbol: symbol, priceText: price, eventTime: eventTime)
    }

    package static func decodeServerEvent(from data: Data) throws -> PriceStreamEvent? {
        let message = try JSONDecoder().decode(ServerMessage.self, from: data)
        switch message.type {
        case "price":
            guard let symbol = message.symbol, let price = message.price, let eventTime = message.eventTime else {
                throw PriceCoreError.invalidServerResponse
            }
            return .price(try PricePoint(
                symbol: symbol, priceText: price, eventTime: eventTime, replay: message.replay ?? false
            ))
        case "status":
            return .warmingUp(symbol: message.symbol, reason: message.reason ?? message.state)
        case "ready":
            return .ready(serverTime: message.serverTime)
        default:
            return nil
        }
    }
}
