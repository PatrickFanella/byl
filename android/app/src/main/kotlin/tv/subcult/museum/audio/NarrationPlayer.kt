package tv.subcult.museum.audio

import android.content.Context
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.media.MediaPlayer
import android.speech.tts.TextToSpeech
import android.speech.tts.UtteranceProgressListener
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import tv.subcult.museum.core.NarrationPlan
import java.util.Locale

/**
 * Plays a moment's narration: MediaPlayer for the recorded file, TextToSpeech for the body.
 * Requests transient-may-duck audio focus before playing and abandons it when playback ends.
 */
class NarrationPlayer(context: Context) {
    private val app = context.applicationContext
    private val audioManager = app.getSystemService(Context.AUDIO_SERVICE) as AudioManager
    private val attributes = AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_ASSISTANCE_NAVIGATION_GUIDANCE).setContentType(AudioAttributes.CONTENT_TYPE_SPEECH).build()
    private val focusRequest = AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN_TRANSIENT_MAY_DUCK).setAudioAttributes(attributes).build()
    private var player: MediaPlayer? = null
    private var tts: TextToSpeech? = null
    private var ttsReady = false
    private var pendingSpeech: String? = null
    private val _isPlaying = MutableStateFlow(false)
    val isPlaying: StateFlow<Boolean> = _isPlaying

    fun play(plan: NarrationPlan) {
        stop()
        when (plan) {
            is NarrationPlan.Stream -> {
                audioManager.requestAudioFocus(focusRequest)
                player = MediaPlayer().apply {
                    setAudioAttributes(attributes)
                    setOnCompletionListener { finished() }
                    setOnErrorListener { _, _, _ -> finished(); true }
                    setOnPreparedListener { it.start() }
                    runCatching { setDataSource(plan.url); prepareAsync() }.onFailure { finished() }
                }
                _isPlaying.value = true
            }
            is NarrationPlan.Speech -> {
                audioManager.requestAudioFocus(focusRequest)
                _isPlaying.value = true
                speak(plan.text)
            }
            NarrationPlan.None -> Unit
        }
    }

    private fun speak(text: String) {
        val engine = tts
        if (engine != null && ttsReady) {
            engine.speak(text, TextToSpeech.QUEUE_FLUSH, null, "narration")
            return
        }
        pendingSpeech = text
        if (engine == null) {
            tts = TextToSpeech(app) { status ->
                ttsReady = status == TextToSpeech.SUCCESS
                if (ttsReady) {
                    tts?.language = Locale.getDefault()
                    tts?.setAudioAttributes(attributes)
                    tts?.setOnUtteranceProgressListener(object : UtteranceProgressListener() {
                        override fun onStart(utteranceId: String?) {}
                        override fun onDone(utteranceId: String?) { finished() }
                        @Deprecated("Deprecated in Java")
                        override fun onError(utteranceId: String?) { finished() }
                    })
                    pendingSpeech?.let { tts?.speak(it, TextToSpeech.QUEUE_FLUSH, null, "narration") }
                } else finished()
                pendingSpeech = null
            }
        }
    }

    fun stop() {
        pendingSpeech = null
        player?.let { runCatching { it.stop() }; it.release() }
        player = null
        if (ttsReady) tts?.stop()
        if (_isPlaying.value) audioManager.abandonAudioFocusRequest(focusRequest)
        _isPlaying.value = false
    }

    private fun finished() {
        player?.release()
        player = null
        audioManager.abandonAudioFocusRequest(focusRequest)
        _isPlaying.value = false
    }

    fun release() {
        stop()
        tts?.shutdown()
        tts = null
        ttsReady = false
    }
}
