import { expect, test } from '@playwright/test';
import fixture from '../src/lib/happenings/fixtures/happenings-v1.json' with { type: 'json' };

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

test('notices are opt-in and a reviewed synthetic API notice keeps its source visible', async ({
	page
}) => {
	let reads = 0;
	page.on('request', (request) => {
		if (request.url().includes('/happenings')) reads++;
	});
	await page.goto('/places/main-street-station');
	await expect(
		page.getByRole('heading', { name: 'The Burlington Arrives', exact: true })
	).toBeVisible();
	const panel = page.getByRole('region', { name: 'Optional current notices' });
	await expect(panel.getByRole('button', { name: 'Show current notices' })).toBeVisible();
	expect(reads).toBe(0);
	const response = page.waitForResponse((r) => r.url().includes('/happenings'));
	await panel.getByRole('button', { name: 'Show current notices' }).click();
	const received = await response;
	expect(received.status()).toBe(200);
	expect(received.headers()['cache-control']).toBe('no-store');
	expect(received.request().headers()['authorization']).toBeUndefined();
	await expect(
		panel.getByRole('heading', { name: 'Synthetic browser history talk' })
	).toBeVisible();
	await expect(panel.getByText('Scheduled for these dates', { exact: true })).toBeVisible();
	await expect(
		panel.getByRole('link', { name: 'Synthetic organizer announcement' })
	).toHaveAttribute('href', 'https://organizer.example/browser-announcement');
	await expect(panel).toContainText('Last source check:');
	await expect(panel).toContainText('dates only');
	await panel.getByRole('button', { name: 'Hide current notices' }).click();
	await expect(panel.getByRole('heading')).toHaveCount(0);
	await expect(
		page.getByRole('heading', { name: 'The Burlington Arrives', exact: true })
	).toBeVisible();
});

test('empty and unavailable optional data leave the historical story readable', async ({
	page
}) => {
	await page.goto('/places/tivoli-theatre');
	const panel = page.getByRole('region', { name: 'Optional current notices' });
	await panel.getByRole('button', { name: 'Show current notices' }).click();
	await expect(panel.getByText('No current notices have been published here.')).toBeVisible();
	for (const body of [null, { items: [{ title: 'Malformed' }], next_cursor: '' }]) {
		await page.route('**/api/v1/places/*/happenings?*', (route) =>
			route.fulfill({
				status: body ? 200 : 404,
				json: body ?? { error: { code: 'not_found', message: 'Older API' } }
			})
		);
		await panel.getByRole('button', { name: 'Refresh current notices' }).click();
		await expect(
			panel.getByText('Current notices are unavailable. The historical story and tour still work.')
		).toBeVisible();
		await expect(
			page.getByRole('heading', { name: 'Built for the Talkies', exact: true })
		).toBeVisible();
		await page.unroute('**/api/v1/places/*/happenings?*');
	}
});

test('all shared status fixtures render clearly and reflow at phone width', async ({
	page
}, info) => {
	await page.clock.install({ time: new Date(fixture.now) });
	await page.setViewportSize({ width: 320, height: 700 });
	await page.goto('/places/main-street-station');
	const panel = page.getByRole('region', { name: 'Optional current notices' });
	const place = (
		await (await page.request.get(`${apiUrl}/api/v1/places/main-street-station`)).json()
	).id;
	const items = fixture.cases.flatMap((example, index) =>
		example.response.items.map((h) => ({
			...h,
			id: `fixture-${index}`,
			title: `Fixture ${example.name}`,
			place_id: place
		}))
	);
	await page.route('**/api/v1/places/*/happenings?*', (route) =>
		route.fulfill({ json: { items, next_cursor: '' } })
	);
	await panel.getByRole('button', { name: 'Show current notices' }).click();
	await expect(panel.locator('article')).toHaveCount(16);
	for (const label of [
		'Cancelled',
		'Expired notice',
		'Dates conflict',
		'Source unavailable',
		'Needs review',
		'Notice unavailable',
		'Upcoming'
	])
		await expect(panel.getByText(label, { exact: true }).first()).toBeVisible();
	await page.evaluate(() => {
		document.documentElement.style.fontSize = '200%';
	});
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1)
	).toBeTruthy();
	await panel
		.getByRole('heading', { name: 'Fixture active', exact: true })
		.scrollIntoViewIfNeeded();
	await page.screenshot({ path: info.outputPath('current-notices-phone.png'), fullPage: false });
});

test('opening current notices during arrival preserves the pinned trip and narration', async ({
	page
}) => {
	await page.addInitScript(() => {
		localStorage.setItem('invisible-museum.audio', '1');
		Object.defineProperty(speechSynthesis, 'speak', {
			value: (utterance: SpeechSynthesisUtterance) => {
				document.documentElement.dataset.narrationCalls = String(
					Number(document.documentElement.dataset.narrationCalls ?? 0) + 1
				);
				document.documentElement.dataset.narrationText = utterance.text;
			}
		});
	});
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	await expect(page.getByText(/you have arrived/i)).toBeVisible();
	await expect
		.poll(() => page.evaluate(() => document.documentElement.dataset.narrationCalls))
		.toBe('1');
	const before = await page.evaluate(() => ({
		storage: JSON.stringify(localStorage),
		narration: document.documentElement.dataset.narrationText
	}));
	const panel = page.getByRole('region', { name: 'Optional current notices' });
	await panel.getByRole('button', { name: 'Show current notices' }).click();
	await expect(
		panel.getByRole('heading', { name: 'Synthetic browser history talk' })
	).toBeVisible();
	expect(
		await page.evaluate(() => ({
			storage: JSON.stringify(localStorage),
			narration: document.documentElement.dataset.narrationText
		}))
	).toEqual(before);
	expect(await page.evaluate(() => document.documentElement.dataset.narrationCalls)).toBe('1');
	await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'Continue to the next stop' }).click();
	await expect(page.getByText('Next · stop 2 of 4', { exact: true })).toBeVisible();
});
