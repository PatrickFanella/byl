import XCTest
@testable import InvisibleMuseumKit

final class DiskCacheTests: XCTestCase {
    private struct Note: Codable, Equatable { let text: String; let n: Int }

    private func temporaryCache() throws -> DiskCache {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("imk-cache-\(UUID().uuidString)")
        return try DiskCache(directory: dir)
    }

    func testRoundTripsCodableValues() throws {
        let cache = try temporaryCache()
        cache.save(Note(text: "hi", n: 3), key: "note")
        XCTAssertEqual(cache.load(Note.self, key: "note"), Note(text: "hi", n: 3))
        XCTAssertTrue(cache.has("note"))
    }

    func testMissingKeyIsNil() throws {
        let cache = try temporaryCache()
        XCTAssertNil(cache.load(Note.self, key: "nope"))
        XCTAssertFalse(cache.has("nope"))
    }

    func testRemoveDeletesTheFile() throws {
        let cache = try temporaryCache()
        cache.save(Note(text: "x", n: 1), key: "note")
        cache.remove("note")
        XCTAssertNil(cache.load(Note.self, key: "note"))
    }

    func testKeysWithSlashesAndSpacesAreSafeFileNames() throws {
        let cache = try temporaryCache()
        cache.save(Note(text: "x", n: 1), key: "route/00000000-0000-4000-8000-00000000e0e1 walk")
        XCTAssertEqual(cache.load(Note.self, key: "route/00000000-0000-4000-8000-00000000e0e1 walk")?.n, 1)
    }

    func testSurvivesANewInstanceOnTheSameDirectory() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("imk-cache-\(UUID().uuidString)")
        try DiskCache(directory: dir).save(Note(text: "kept", n: 9), key: "note")
        XCTAssertEqual(try DiskCache(directory: dir).load(Note.self, key: "note")?.text, "kept")
    }
}
