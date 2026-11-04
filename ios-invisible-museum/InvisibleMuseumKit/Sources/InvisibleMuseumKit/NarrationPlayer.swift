import Foundation
import AVFoundation
import Observation

/// What the audio guide should do at a stop.
public enum NarrationSource: Equatable {
    /// A recorded narration file (`moment.narration.public_url`).
    case stream(URL)
    /// Read the moment body with the on-device synthesizer.
    case speech(String)
    case none
}

/// The audio session side effect, abstracted so tests can check it happens before playback.
public protocol AudioSessionConfiguring: AnyObject {
    func configureForSpokenAudio() throws
}

/// The speech synthesizer, abstracted so tests do not speak.
public protocol SpeechSynthesizing: AnyObject {
    func speak(_ text: String)
    func stopSpeaking()
    var isSpeaking: Bool { get }
}

/// Plays a moment's narration: the recorded file when there is one, otherwise the body read
/// aloud. Keeps a strong reference to the AVPlayer so audio continues with the screen locked.
@MainActor
@Observable
public final class NarrationPlayer {
    public private(set) var currentPlayer: AVPlayer?
    public private(set) var current: NarrationSource = .none
    public private(set) var lastError: String?

    private let session: AudioSessionConfiguring
    private let synthesizer: SpeechSynthesizing
    private var endObserver: NSObjectProtocol?

    public init(session: AudioSessionConfiguring, synthesizer: SpeechSynthesizing) {
        self.session = session
        self.synthesizer = synthesizer
    }

    public var isPlaying: Bool {
        switch current {
        case .stream: return currentPlayer != nil
        case .speech: return synthesizer.isSpeaking
        case .none: return false
        }
    }

    /// Picks the source for a moment: recorded narration first, then the body as speech.
    public static func plan(narrationURL: URL?, body: String) -> NarrationSource {
        if let narrationURL { return .stream(narrationURL) }
        let text = plainText(body)
        return text.isEmpty ? .none : .speech(text)
    }

    public func play(_ source: NarrationSource) {
        stop()
        guard source != .none else { return }
        do {
            try session.configureForSpokenAudio()
        } catch {
            lastError = error.localizedDescription
        }
        current = source
        switch source {
        case .stream(let url):
            let player = AVPlayer(url: url)
            currentPlayer = player
            endObserver = NotificationCenter.default.addObserver(forName: .AVPlayerItemDidPlayToEndTime, object: player.currentItem, queue: .main) { [weak self] _ in
                Task { @MainActor in self?.stop() }
            }
            player.play()
        case .speech(let text):
            synthesizer.speak(text)
        case .none:
            break
        }
    }

    public func stop() {
        if let endObserver {
            NotificationCenter.default.removeObserver(endObserver)
            self.endObserver = nil
        }
        currentPlayer?.pause()
        currentPlayer = nil
        if case .speech = current { synthesizer.stopSpeaking() }
        current = .none
    }

    /// Strips the Markdown a narrator should not read aloud and collapses whitespace.
    public static func plainText(_ markdown: String) -> String {
        var text = markdown
        let patterns: [(String, String)] = [
            ("(?s)```.*?```", " "),                         // fenced code
            ("!\\[[^\\]]*\\]\\([^)]*\\)", " "),             // images
            ("\\[([^\\]]*)\\]\\([^)]*\\)", "$1"),            // links → text
            ("<[^>]+>", " "),                                // html tags
            ("(?m)^\\s{0,3}#{1,6}\\s*", ""),                  // headings
            ("(?m)^\\s*(?:[-*+]|\\d+\\.)\\s+", ""),           // list markers
            ("(?m)^\\s*>\\s?", ""),                           // block quotes
            ("`([^`]*)`", "$1"),                              // inline code
            ("(\\*\\*|__)(.*?)\\1", "$2"),                    // strong
            ("(\\*|_)(.*?)\\1", "$2")                         // emphasis
        ]
        for (pattern, replacement) in patterns {
            if let re = try? NSRegularExpression(pattern: pattern) {
                text = re.stringByReplacingMatches(in: text, range: NSRange(text.startIndex..., in: text), withTemplate: replacement)
            }
        }
        return text.split(whereSeparator: { $0.isWhitespace || $0.isNewline }).joined(separator: " ")
    }
}

/// The real synthesizer behind SpeechSynthesizing.
public final class SystemSpeechSynthesizer: SpeechSynthesizing {
    private let synthesizer = AVSpeechSynthesizer()
    public var voiceLanguage = "en-US"
    public var rate: Float = AVSpeechUtteranceDefaultSpeechRate

    public init() {}

    public func speak(_ text: String) {
        let utterance = AVSpeechUtterance(string: text)
        utterance.voice = AVSpeechSynthesisVoice(language: voiceLanguage)
        utterance.rate = rate
        synthesizer.speak(utterance)
    }

    public func stopSpeaking() {
        synthesizer.stopSpeaking(at: .immediate)
    }

    public var isSpeaking: Bool { synthesizer.isSpeaking }
}
