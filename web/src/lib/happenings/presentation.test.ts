import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import fixture from './fixtures/happenings-v1.json';
import type { Happening } from '#lib/visitor.ts';
import { noticeStatus, noticeSourceURL, noticeDates, cityDay } from './presentation';

describe('shared current-notice responses', () => {
	it('keeps web, Swift and Kotlin on identical recorded response bytes', () => {
		const canonical = readFileSync(
			new URL('../../../../backend/api/fixtures/happenings-v1/responses.json', import.meta.url)
		);
		for (const path of [
			'./fixtures/happenings-v1.json',
			'../../../../ios-invisible-museum/InvisibleMuseumKit/Tests/InvisibleMuseumKitTests/Fixtures/happenings-v1.json',
			'../../../../android/core/src/test/resources/fixtures/happenings-v1.json'
		])
			expect(readFileSync(new URL(path, import.meta.url))).toEqual(canonical);
	});
	for (const example of fixture.cases)
		it(example.name, () => {
			expect(
				example.response.items.map((h) => noticeStatus(h as Happening, Date.parse(fixture.now)))
			).toEqual(example.expected_statuses);
		});
	it('uses the city calendar instead of the device day and never invents an occurrence time', () => {
		expect(cityDay(Date.parse('2026-11-09T03:00:00Z'), 'America/Chicago')).toBe('2026-11-08');
		expect(noticeDates(fixture.cases[0].response.items[0] as Happening)).toBe(
			'Nov 8, 2026 · America/Chicago · dates only'
		);
	});
	it('permits only ordinary source links without embedded credentials', () => {
		for (const link of [
			'javascript:alert(1)',
			'data:text/plain,source',
			'https://user:secret@example.org/'
		])
			expect(noticeSourceURL(link)).toBeNull();
		expect(noticeSourceURL('https://organizer.example/notice')).toBe(
			'https://organizer.example/notice'
		);
	});
});
