import Foundation

/// Error returned for any non-2xx response, carrying the API's error envelope.
public struct APIError: Error, Equatable, LocalizedError {
    public let status: Int
    public let code: String
    public let message: String
    public let fields: [String: String]

    public init(status: Int, code: String, message: String, fields: [String: String] = [:]) {
        self.status = status
        self.code = code
        self.message = message
        self.fields = fields
    }

    public var errorDescription: String? { message }
    public var isUnauthorized: Bool { status == 401 }
    public var isNotFound: Bool { status == 404 }
}

/// Minimal URLSession wrapper: base URL, bearer token, JSON encode and decode, error envelope.
public final class APIClient: @unchecked Sendable {
    public var baseURL: URL
    public var tokenProvider: () -> String?
    /// Called on every 401 so the app can clear its session.
    public var onUnauthorized: (() -> Void)?

    private let session: URLSession
    private let decoder = JSONDecoder()
    private let encoder = JSONEncoder()

    public init(baseURL: URL, session: URLSession = .shared, tokenProvider: @escaping () -> String? = { nil }) {
        self.baseURL = baseURL
        self.session = session
        self.tokenProvider = tokenProvider
    }

    public func get<T: Decodable>(_ path: String, query: [String: String] = [:]) async throws -> T {
        try await request("GET", path, query: query, body: Optional<Int>.none)
    }

    public func post<T: Decodable, B: Encodable>(_ path: String, body: B?) async throws -> T {
        try await request("POST", path, query: [:], body: body)
    }

    public func put<T: Decodable, B: Encodable>(_ path: String, body: B?) async throws -> T {
        try await request("PUT", path, query: [:], body: body)
    }

    public func patch<T: Decodable, B: Encodable>(_ path: String, body: B?) async throws -> T {
        try await request("PATCH", path, query: [:], body: body)
    }

    /// Sends a request whose success response has no body (204).
    public func send<B: Encodable>(_ method: String, _ path: String, body: B?) async throws {
        let _: Empty = try await request(method, path, query: [:], body: body)
    }

    public func delete(_ path: String) async throws {
        try await send("DELETE", path, body: Optional<Int>.none)
    }

    public struct Empty: Decodable { public init() {} }

    private func request<T: Decodable, B: Encodable>(_ method: String, _ path: String, query: [String: String], body: B?) async throws -> T {
        var components = URLComponents(url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false)
        if !query.isEmpty {
            components?.queryItems = query.map { URLQueryItem(name: $0.key, value: $0.value) }
        }
        guard let url = components?.url else {
            throw APIError(status: 0, code: "bad_url", message: "Could not build the request URL.")
        }
        var req = URLRequest(url: url)
        req.httpMethod = method
        req.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token = tokenProvider() {
            req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let body = body {
            req.setValue("application/json", forHTTPHeaderField: "Content-Type")
            req.httpBody = try encoder.encode(body)
        }
        let (data, response) = try await session.data(for: req)
        guard let http = response as? HTTPURLResponse else {
            throw APIError(status: 0, code: "no_response", message: "No response from the server.")
        }
        if http.statusCode == 401 {
            onUnauthorized?()
        }
        guard (200..<300).contains(http.statusCode) else {
            throw Self.parseError(status: http.statusCode, data: data)
        }
        if T.self == Empty.self || data.isEmpty {
            if let empty = Empty() as? T { return empty }
        }
        do {
            return try decoder.decode(T.self, from: data)
        } catch {
            throw APIError(status: http.statusCode, code: "decode_failed", message: "The server's response could not be read: \(error.localizedDescription)")
        }
    }

    /// Maps the error envelope ({"error": {code, message, fields}}) to APIError.
    public static func parseError(status: Int, data: Data) -> APIError {
        if let body = try? JSONDecoder().decode(APIErrorBody.self, from: data) {
            return APIError(status: status, code: body.error.code, message: body.error.message, fields: body.error.fields ?? [:])
        }
        return APIError(status: status, code: "http_error", message: "The request failed with status \(status).")
    }
}
