import AVFoundation
import InvisibleMuseumKit

/// Playback category with spoken-audio mode and ducking, set before anything plays so the
/// guide keeps talking with the screen locked and lowers music instead of fighting it.
final class SpokenAudioSession: AudioSessionConfiguring {
    func configureForSpokenAudio() throws {
        #if os(iOS)
        let session = AVAudioSession.sharedInstance()
        try session.setCategory(.playback, mode: .spokenAudio, options: [.duckOthers])
        try session.setActive(true)
        #endif
    }

    func deactivate() {
        #if os(iOS)
        try? AVAudioSession.sharedInstance().setActive(false, options: [.notifyOthersOnDeactivation])
        #endif
    }
}
