import XCTest
@testable import InvisibleMuseumKit

final class SessionControllerTests: XCTestCase {
    private func session(role: String = "visitor") throws -> Session {
        let json = """
        {"token":"tok-1","expires_at":"2026-12-01T00:00:00Z","account":{"id":"a1","provider":"dev","email":"a@b.c","display_name":"A","role":"\(role)","created_at":"2026-11-04T00:00:00Z"}}
        """
        return try JSONDecoder().decode(Session.self, from: Data(json.utf8))
    }

    func testSetPersistsAndRestoreReloads() throws {
        let storage = MemoryTokenStorage()
        let first = SessionController(storage: storage)
        first.set(session: try session())
        XCTAssertTrue(first.isSignedIn)

        let second = SessionController(storage: storage)
        XCTAssertTrue(second.restore())
        XCTAssertEqual(second.token, "tok-1")
        XCTAssertEqual(second.account?.email, "a@b.c")
    }

    func testClearRemovesEverythingAndNotifies() throws {
        let storage = MemoryTokenStorage()
        let controller = SessionController(storage: storage)
        var changes = 0
        controller.onChange = { changes += 1 }
        controller.set(session: try session())
        controller.clear()
        XCTAssertNil(controller.token)
        XCTAssertNil(controller.account)
        XCTAssertNil(storage.read(SessionController.tokenKey))
        XCTAssertEqual(changes, 2)
        XCTAssertFalse(SessionController(storage: storage).restore())
    }

    func testUpdateAccountKeepsToken() throws {
        let controller = SessionController(storage: MemoryTokenStorage())
        controller.set(session: try session())
        controller.update(account: Account(id: "a1", provider: "dev", email: "a@b.c", displayName: "A", role: "curator"))
        XCTAssertEqual(controller.token, "tok-1")
        XCTAssertEqual(controller.account?.role, "curator")
    }

    func testRestoreWithoutTokenDropsStaleAccount() {
        let storage = MemoryTokenStorage()
        storage.write(SessionController.accountKey, value: "{\"id\":\"x\",\"provider\":\"dev\",\"email\":\"\",\"display_name\":\"\",\"role\":\"visitor\"}")
        let controller = SessionController(storage: storage)
        XCTAssertFalse(controller.restore())
        XCTAssertNil(controller.account)
    }
}
