import { describe, expect, it, vi } from 'vitest';
import { NarrationPlayer, plainText, plan } from './narration';

describe('plan', () => {
	it('prefers recorded narration', () => {
		expect(plan('https://media.test/n.mp3', 'Some body')).toEqual({
			kind: 'stream',
			url: 'https://media.test/n.mp3'
		});
	});
	it('falls back to speech from the plain body', () => {
		expect(plan(null, '## The Depot\n\nIt **opened** in [1864](https://x).')).toEqual({
			kind: 'speech',
			text: 'The Depot It opened in 1864.'
		});
	});
	it('is none without anything to say', () => {
		expect(plan(null, '   ')).toEqual({ kind: 'none' });
	});
});

describe('plainText', () => {
	it('preserves underscores inside words', () => {
		expect(plainText('museum_archive_story and _history_')).toBe(
			'museum_archive_story and history'
		);
	});
	it('strips Markdown', () => {
		const md = `# Title

Some *emphasis* and __strong__ words, \`code\`, an ![image](https://i/x.png) and a [link](https://example.org).

- a list item
1. numbered

> quoted

\`\`\`
fenced code
\`\`\`
<br>Trailing   spaces`;
		expect(plainText(md)).toBe(
			'Title Some emphasis and strong words, code, an and a link. a list item numbered quoted Trailing spaces'
		);
	});
});

describe('NarrationPlayer', () => {
	it('ignores the late end event of an utterance it cancelled', () => {
		type Utterance = { text: string; onend: (() => void) | null; onerror: (() => void) | null };
		const spoken: Utterance[] = [];
		vi.stubGlobal(
			'SpeechSynthesisUtterance',
			class {
				onend: (() => void) | null = null;
				onerror: (() => void) | null = null;
				constructor(public text: string) {}
			}
		);
		const synth = { speak: (u: Utterance) => spoken.push(u), cancel: vi.fn() };
		vi.stubGlobal('speechSynthesis', synth);
		try {
			const player = new NarrationPlayer();
			const ended = vi.fn();
			player.onEnd = ended;
			player.play({ kind: 'speech', text: 'Stop A' });
			player.play({ kind: 'speech', text: 'Stop B' });
			spoken[0].onerror?.(); // cancel() interrupts A after B has started
			expect(player.isPlaying).toBe(true);
			expect(ended).not.toHaveBeenCalled();
			player.stop();
			expect(synth.cancel).toHaveBeenCalledTimes(2);
			spoken[1].onend?.();
			expect(ended).not.toHaveBeenCalled();
		} finally {
			vi.unstubAllGlobals();
		}
	});
});
