import XCTest
@testable import InvisibleMuseumKit

/// Records what the player asked of the audio session and the synthesizer, in order.
private final class FakeAudio: AudioSessionConfiguring, SpeechSynthesizing {
    var log: [String] = []
    var speaking = false
    func configureForSpokenAudio() throws { log.append("configure") }
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
    func testSpeechConfiguresTheSessionBeforeSpeaking() {
        let fake = FakeAudio()
        let player = NarrationPlayer(session: fake, synthesizer: fake)
        player.play(.speech("Hello"))
        XCTAssertEqual(fake.log, ["configure", "speak:Hello"])
        XCTAssertTrue(player.isPlaying)
        player.stop()
        XCTAssertEqual(fake.log.last, "stopSpeech")
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
        XCTAssertEqual(fake.log, ["configure", "speak:One", "stopSpeech", "configure"])
        XCTAssertNotNil(player.currentPlayer)
    }
}
