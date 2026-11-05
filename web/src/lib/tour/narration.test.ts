import { describe, expect, it } from 'vitest';
import { plainText, plan } from './narration';

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
