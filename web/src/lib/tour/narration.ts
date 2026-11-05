// What the audio guide does at a stop: the recorded file, or the body read aloud.

export type NarrationPlan =
	{ kind: 'stream'; url: string } | { kind: 'speech'; text: string } | { kind: 'none' };

export function plan(narrationURL: string | null | undefined, body: string): NarrationPlan {
	if (narrationURL) return { kind: 'stream', url: narrationURL };
	const text = plainText(body);
	return text ? { kind: 'speech', text } : { kind: 'none' };
}

/** Strips the Markdown a narrator should not read aloud and collapses whitespace. */
export function plainText(markdown: string): string {
	return markdown
		.replace(/```[\s\S]*?```/g, ' ')
		.replace(/!\[[^\]]*\]\([^)]*\)/g, ' ')
		.replace(/\[([^\]]*)\]\([^)]*\)/g, '$1')
		.replace(/<[^>]+>/g, ' ')
		.replace(/^\s{0,3}#{1,6}\s*/gm, '')
		.replace(/^\s*(?:[-*+]|\d+\.)\s+/gm, '')
		.replace(/^\s*>\s?/gm, '')
		.replace(/`([^`]*)`/g, '$1')
		.replace(/(\*\*|__)(.*?)\1/g, '$2')
		.replace(/(\*|_)(.*?)\1/g, '$2')
		.split(/\s+/)
		.filter(Boolean)
		.join(' ');
}

/** Plays a plan in the browser: <audio> for a stream, speechSynthesis for speech. */
export class NarrationPlayer {
	private audio: HTMLAudioElement | null = null;
	private speaking = false;
	onEnd: (() => void) | null = null;

	get isPlaying(): boolean {
		return !!this.audio || this.speaking;
	}

	play(p: NarrationPlan): void {
		this.stop();
		if (p.kind === 'stream') {
			const a = new Audio(p.url);
			a.onended = () => this.finished();
			a.onerror = () => this.finished();
			this.audio = a;
			void a.play().catch(() => this.finished());
		} else if (p.kind === 'speech' && typeof speechSynthesis !== 'undefined') {
			const u = new SpeechSynthesisUtterance(p.text);
			u.onend = () => this.finished();
			u.onerror = () => this.finished();
			this.speaking = true;
			speechSynthesis.speak(u);
		}
	}

	stop(): void {
		if (this.audio) {
			this.audio.pause();
			this.audio = null;
		}
		if (this.speaking && typeof speechSynthesis !== 'undefined') speechSynthesis.cancel();
		this.speaking = false;
	}

	private finished(): void {
		this.audio = null;
		this.speaking = false;
		this.onEnd?.();
	}
}
