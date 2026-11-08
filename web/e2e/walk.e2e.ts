import { expect, test } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

const station = { latitude: 41.796, longitude: -88.0093, accuracy: 10 };

test('arriving at the first stop by GPS shows its moment and advances', async ({ browser }) => {
	const context = await browser.newContext({ geolocation: station, permissions: ['geolocation'] });
	const page = await context.newPage();
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText(/you have arrived/i)).toBeVisible({ timeout: 15_000 });
	await expect(page.getByText('The Burlington Arrives')).toBeVisible();
	await expect(page.getByText('1 of 4 stops')).toBeVisible();
	await page.getByRole('button', { name: /continue/i }).click();
	await expect(page.getByText(/next · stop 2/i)).toBeVisible();
	await context.close();
});

test('with location denied the walk still works through "I\'m here"', async ({ browser }) => {
	// Review Focus 1: no geolocation permission at all.
	const context = await browser.newContext();
	const page = await context.newPage();
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText(/next · stop 1/i)).toBeVisible({ timeout: 15_000 });
	await expect(page.getByText(/location is off/i)).toBeVisible({ timeout: 15_000 });
	await page.getByRole('button', { name: /i'm here/i }).click();
	await expect(page.getByText(/you have arrived/i)).toBeVisible();
	await page.getByRole('button', { name: /continue/i }).click();
	await expect(page.getByText(/next · stop 2/i)).toBeVisible();
	await context.close();
});

test('the audio guide plays the featured moment once on arrival', async ({ browser }) => {
	// Revision-aware trips narrate retained text without fetching mutable moments.
	const context = await browser.newContext({ geolocation: station, permissions: ['geolocation'] });
	await context.addInitScript(() => {
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
	const page = await context.newPage();
	let momentFetches = 0;
	page.on('request', (r) => {
		if (/\/api\/v1\/moments\//.test(r.url())) momentFetches++;
	});
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText(/you have arrived/i)).toBeVisible({ timeout: 15_000 });
	await expect
		.poll(() => page.evaluate(() => document.documentElement.dataset.narrationCalls))
		.toBe('1');
	const tour = await (
		await context.request.get(`${apiUrl}/api/v1/tours/downtown-memory-loop`)
	).json();
	const revision = await (
		await context.request.get(`${apiUrl}/api/v1/tours/${tour.id}/revisions/${tour.revision_id}`)
	).json();
	const id = revision.snapshot.tour.stops[0].featured_moment.id;
	expect(await page.evaluate(() => document.documentElement.dataset.narrationText)).toBe(
		revision.snapshot.moments[id].body.replace(/\s+/g, ' ').trim()
	);
	await page.waitForTimeout(3000);
	expect(await page.evaluate(() => document.documentElement.dataset.narrationCalls)).toBe('1');
	expect(momentFetches).toBe(0);
	await context.close();
});

test('the end dialog traps focus, closes with Escape, and restores the opener', async ({
	page
}) => {
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText(/next · stop 1/i)).toBeVisible();
	const opener = page.getByRole('button', { name: 'End', exact: true });
	await opener.click();
	const dialog = page.getByRole('dialog', { name: 'End this tour?' });
	await expect(dialog).toHaveAttribute('aria-modal', 'true');
	await expect(dialog.getByRole('button', { name: 'Keep my progress and end' })).toBeFocused();
	await page.keyboard.press('Shift+Tab');
	await expect(dialog.getByRole('button', { name: 'Cancel' })).toBeFocused();
	await page.keyboard.press('Escape');
	await expect(dialog).not.toBeVisible();
	await expect(opener).toBeFocused();
});
