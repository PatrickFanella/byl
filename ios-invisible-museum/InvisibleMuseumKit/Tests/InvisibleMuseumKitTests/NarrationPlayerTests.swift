import XCTest
@testable import InvisibleMuseumKit

/// Records what the player asked of the audio session and the synthesizer, in order.
private final class FakeAudio: AudioSessionConfiguring, SpeechSynthesizing {
    var log: [String] = []
    var speaking = false
    var onFinish: (() -> Void)?
    func configureForSpokenAudio() throws { log.append("configure") }
    func deactivate() { log.append("deactivate") }
    func speak(_ text: String) { log.append("speak:\(text)"); speaking = true }
    func stopSpeaking() { log.append("stopSpeech"); speaking = false }
    var isSpeaking: Bool { speaking }
}

final class NarrationPlayerTests: XCTestCase {
    func testPlanPrefersRecordedNarration() {
        let url = URL(string: "https://media.test/narration/x.mp3")!
        XCTAssertEqual(NarrationPlayer.plan(narrationURL: url, body: "Some body"), .stream(url))
    }

    func testPlanFallsBackToSpeechFromPlainText() {
        XCTAssertEqual(NarrationPlayer.plan(narrationURL: nil, body: "## The Depot\n\nIt **opened** in [1864](https://x)."), .speech("The Depot It opened in 1864."))
    }

    func testPlanIsNoneWithoutAnything() {
        XCTAssertEqual(NarrationPlayer.plan(narrationURL: nil, body: "   "), .none)
    }

    func testPlainTextStripsMarkdown() {
        let md = """
        # Title

        Some *emphasis* and __strong__ words, `code`, an ![image](https://i/x.png) and a [link](https://example.org).

        - a list item
        1. numbered

        > quoted

        ```
        fenced code
        ```
        <br>Trailing   spaces
        """
        XCTAssertEqual(NarrationPlayer.plainText(md), "Title Some emphasis and strong words, code, an and a link. a list item numbered quoted Trailing spaces")
    }

    @MainActor
    func testSpeechConfiguresTheSessionBeforeSpeakingAndReleasesItOnStop() {
        let fake = FakeAudio()
        let player = NarrationPlayer(session: fake, synthesizer: fake)
        player.play(.speech("Hello"))
        XCTAssertEqual(fake.log, ["configure", "speak:Hello"])
        XCTAssertTrue(player.isPlaying)
        player.stop()
        XCTAssertEqual(fake.log, ["configure", "speak:Hello", "stopSpeech", "deactivate"])
        XCTAssertFalse(player.isPlaying)
    }

    @MainActor
    func testStreamConfiguresTheSessionAndKeepsAStrongPlayer() {
        // Review Focus 4: the AVPlayer must be retained or audio stops when the screen locks.
        let fake = FakeAudio()
        let player = NarrationPlayer(session: fake, synthesizer: fake)
        player.play(.stream(URL(string: "https://media.test/narration/x.mp3")!))
        XCTAssertEqual(fake.log, ["configure"])
        XCTAssertNotNil(player.currentPlayer)
        XCTAssertTrue(player.isPlaying)
        player.stop()
        XCTAssertNil(player.currentPlayer)
        XCTAssertEqual(fake.log, ["configure", "deactivate"])
    }

    @MainActor
    func testPlayingNoneDoesNothing() {
        let fake = FakeAudio()
        let player = NarrationPlayer(session: fake, synthesizer: fake)
        player.play(.none)
        XCTAssertTrue(fake.log.isEmpty)
        XCTAssertFalse(player.isPlaying)
    }

    @MainActor
    func testNewPlaybackStopsThePreviousOne() {
        let fake = FakeAudio()
        let player = NarrationPlayer(session: fake, synthesizer: fake)
        player.play(.speech("One"))
        player.play(.stream(URL(string: "https://media.test/n.mp3")!))
        XCTAssertEqual(fake.log, ["configure", "speak:One", "stopSpeech", "deactivate", "configure"])
        XCTAssertNotNil(player.currentPlayer)
    }

    @MainActor
    func testSpeechFinishingReleasesTheSession() async throws {
        // Review fix: ducked music must come back when the narrator stops talking.
        let fake = FakeAudio()
        let player = NarrationPlayer(session: fake, synthesizer: fake)
        player.play(.speech("Short"))
        fake.speaking = false
        fake.onFinish?()
        try await Task.sleep(nanoseconds: 100_000_000)
        XCTAssertFalse(player.isPlaying)
        XCTAssertEqual(fake.log.last, "deactivate")
        XCTAssertEqual(fake.log.filter { $0 == "deactivate" }.count, 1)
    }

    @MainActor
    func testStopIsIdempotent() {
        let fake = FakeAudio()
        let player = NarrationPlayer(session: fake, synthesizer: fake)
        player.stop()
        player.stop()
        XCTAssertTrue(fake.log.isEmpty, "nothing was playing, so the session is left alone")
    }
}
