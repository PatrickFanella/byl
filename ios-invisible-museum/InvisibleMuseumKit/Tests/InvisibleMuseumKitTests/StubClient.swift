import Foundation
@testable import InvisibleMuseumKit

/// URLProtocol stub: tests register a handler that answers by method and path, and read back
/// every request the client made.
final class StubURLProtocol: URLProtocol {
    struct Recorded {
        let method: String
        let path: String
        let host: String?
        let query: String?
        let body: Data?
        let headers: [String: String]
    }

    nonisolated(unsafe) static var handler: ((String, String) -> (Int, Data))?
    nonisolated(unsafe) static var requests: [Recorded] = []

    static func reset() {
        handler = nil
        requests = []
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let method = request.httpMethod ?? "GET"
        let path = request.url?.path ?? ""
        Self.requests.append(Recorded(method: method, path: path, host: request.url?.host, query: request.url?.query, body: Self.readBody(request), headers: request.allHTTPHeaderFields ?? [:]))
        let (status, data) = Self.handler?(method, path) ?? (404, Data("{\"error\":{\"code\":\"not_found\",\"message\":\"no stub\"}}".utf8))
        let response = HTTPURLResponse(url: request.url!, statusCode: status, httpVersion: "HTTP/1.1", headerFields: ["Content-Type": "application/json"])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: data)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}

    /// URLSession hands a body to a URLProtocol as a stream, not as httpBody.
    private static func readBody(_ request: URLRequest) -> Data? {
        if let body = request.httpBody { return body }
        guard let stream = request.httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }
        var data = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let n = stream.read(&buffer, maxLength: buffer.count)
            if n <= 0 { break }
            data.append(buffer, count: n)
        }
        return data
    }
}

/// Builds an APIClient whose every request goes to StubURLProtocol.
func stubbedClient(token: String? = nil) -> APIClient {
    let config = URLSessionConfiguration.ephemeral
    config.protocolClasses = [StubURLProtocol.self]
    return APIClient(baseURL: URL(string: "https://stub.test")!, session: URLSession(configuration: config), tokenProvider: { token })
}

func fixtureData(_ name: String) -> Data {
    guard let url = Bundle.module.url(forResource: name, withExtension: "json", subdirectory: "Fixtures") else {
        fatalError("missing fixture \(name)")
    }
    return try! Data(contentsOf: url)
}
