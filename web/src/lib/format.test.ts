import { describe, expect, it } from 'vitest';
import { safeHref } from './format';

describe('safeHref', () => {
	it('keeps http(s) links and drops script, data and relative targets', () => {
		expect(safeHref('https://archive.org/details/x')).toBe('https://archive.org/details/x');
		expect(safeHref(' HTTP://example.org ')).toBe('HTTP://example.org');
		for (const bad of [
			'javascript:alert(1)',
			' javascript:alert(1)',
			'data:text/html,x',
			'/admin',
			'',
			null,
			undefined,
			7
		])
			expect(safeHref(bad)).toBeUndefined();
	});
});
