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

/// The audio session side effects, abstracted so tests can check that the session is
/// configured before playback and released when it ends (other apps are ducked meanwhile).
public protocol AudioSessionConfiguring: AnyObject {
    func configureForSpokenAudio() throws
    func deactivate()
}

/// The speech synthesizer, abstracted so tests do not speak.
public protocol SpeechSynthesizing: AnyObject {
    func speak(_ text: String)
    func stopSpeaking()
    var isSpeaking: Bool { get }
    /// Called when an utterance finishes on its own.
    var onFinish: (() -> Void)? { get set }
}

/// Plays a moment's narration: the recorded file when there is one, otherwise the body read
/// aloud. Keeps a strong reference to the AVPlayer so audio continues with the screen locked,
/// and releases the audio session when playback ends so ducked audio comes back.
@MainActor
@Observable
public final class NarrationPlayer {
    public private(set) var currentPlayer: AVPlayer?
    public private(set) var current: NarrationSource = .none
    public private(set) var isPlaying = false
    public private(set) var lastError: String?

    private let session: AudioSessionConfiguring
    private let synthesizer: SpeechSynthesizing
    private var endObserver: NSObjectProtocol?

    public init(session: AudioSessionConfiguring, synthesizer: SpeechSynthesizing) {
        self.session = session
        self.synthesizer = synthesizer
        synthesizer.onFinish = { [weak self] in
            Task { @MainActor [weak self] in self?.playbackFinished() }
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
        isPlaying = true
        switch source {
        case .stream(let url):
            let player = AVPlayer(url: url)
            currentPlayer = player
            endObserver = NotificationCenter.default.addObserver(forName: .AVPlayerItemDidPlayToEndTime, object: player.currentItem, queue: .main) { [weak self] _ in
                Task { @MainActor [weak self] in self?.playbackFinished() }
            }
            player.play()
        case .speech(let text):
            synthesizer.speak(text)
        case .none:
            break
        }
    }

    public func stop() {
        guard isPlaying || currentPlayer != nil else { return }
        if let endObserver {
            NotificationCenter.default.removeObserver(endObserver)
            self.endObserver = nil
        }
        let wasSpeech: Bool
        if case .speech = current { wasSpeech = true } else { wasSpeech = false }
        isPlaying = false
        current = .none
        currentPlayer?.pause()
        currentPlayer = nil
        if wasSpeech { synthesizer.stopSpeaking() }
        session.deactivate()
    }

    private func playbackFinished() {
        guard isPlaying else { return }
        stop()
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

/// The real synthesizer behind SpeechSynthesizing; reports when an utterance ends.
public final class SystemSpeechSynthesizer: NSObject, SpeechSynthesizing, AVSpeechSynthesizerDelegate {
    private let synthesizer = AVSpeechSynthesizer()
    public var voiceLanguage = "en-US"
    public var rate: Float = AVSpeechUtteranceDefaultSpeechRate
    public var onFinish: (() -> Void)?

    public override init() {
        super.init()
        synthesizer.delegate = self
    }

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

    public func speechSynthesizer(_ synthesizer: AVSpeechSynthesizer, didFinish utterance: AVSpeechUtterance) {
        onFinish?()
    }
}
