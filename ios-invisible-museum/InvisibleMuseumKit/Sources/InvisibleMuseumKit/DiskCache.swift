import Foundation

/// JSON files in a directory, keyed by string. Used for the last city, tours, the active tour's
/// detail and route, and local tour progress, so an active tour survives losing signal.
public final class DiskCache: @unchecked Sendable {
    public let directory: URL
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    /// Application Support/InvisibleMuseum, created on first use.
    public static func applicationSupport() throws -> DiskCache {
        let base = try FileManager.default.url(for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true)
        return try DiskCache(directory: base.appendingPathComponent("InvisibleMuseum", isDirectory: true))
    }

    public init(directory: URL) throws {
        self.directory = directory
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
    }

    public func save<T: Encodable>(_ value: T, key: String) {
        guard let data = try? encoder.encode(value) else { return }
        try? data.write(to: url(for: key), options: .atomic)
    }

    public func load<T: Decodable>(_ type: T.Type, key: String) -> T? {
        guard let data = try? Data(contentsOf: url(for: key)) else { return nil }
        return try? decoder.decode(type, from: data)
    }

    public func has(_ key: String) -> Bool {
        FileManager.default.fileExists(atPath: url(for: key).path)
    }

    public func remove(_ key: String) {
        try? FileManager.default.removeItem(at: url(for: key))
    }

    private func url(for key: String) -> URL {
        let safe = key.map { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" || $0 == "." ? String($0) : "_" }.joined()
        return directory.appendingPathComponent(safe + ".json")
    }
}
