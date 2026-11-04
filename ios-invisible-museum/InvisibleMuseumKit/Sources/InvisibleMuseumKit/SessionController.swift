import Foundation

/// Where the session token and account live between launches. The app uses the Keychain;
/// tests use memory.
public protocol TokenStorage: AnyObject {
    func read(_ key: String) -> String?
    func write(_ key: String, value: String)
    func remove(_ key: String)
}

public final class MemoryTokenStorage: TokenStorage {
    private var values: [String: String] = [:]
    public init() {}
    public func read(_ key: String) -> String? { values[key] }
    public func write(_ key: String, value: String) { values[key] = value }
    public func remove(_ key: String) { values[key] = nil }
}

/// Holds the bearer token and the signed-in account and persists them.
public final class SessionController {
    public static let tokenKey = "session.token"
    public static let accountKey = "session.account"

    private let storage: TokenStorage
    public private(set) var token: String?
    public private(set) var account: Account?
    /// Called after any change so observers can refresh.
    public var onChange: (() -> Void)?

    public init(storage: TokenStorage) {
        self.storage = storage
    }

    public var isSignedIn: Bool { token != nil }

    /// Loads a previous session from storage. Returns whether one existed.
    @discardableResult
    public func restore() -> Bool {
        token = storage.read(Self.tokenKey)
        if let raw = storage.read(Self.accountKey), let data = raw.data(using: .utf8) {
            account = try? JSONDecoder().decode(Account.self, from: data)
        } else {
            account = nil
        }
        if token == nil { account = nil }
        onChange?()
        return token != nil
    }

    public func set(session: Session) {
        token = session.token
        account = session.account
        storage.write(Self.tokenKey, value: session.token)
        persistAccount()
        onChange?()
    }

    public func update(account: Account) {
        self.account = account
        persistAccount()
        onChange?()
    }

    public func clear() {
        token = nil
        account = nil
        storage.remove(Self.tokenKey)
        storage.remove(Self.accountKey)
        onChange?()
    }

    private func persistAccount() {
        guard let account, let data = try? JSONEncoder().encode(account), let raw = String(data: data, encoding: .utf8) else { return }
        storage.write(Self.accountKey, value: raw)
    }
}
