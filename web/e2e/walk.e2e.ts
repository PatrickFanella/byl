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
	// Review fix: the auto-play effect must not loop (fetch → play → stop → fetch …).
	const context = await browser.newContext({ geolocation: station, permissions: ['geolocation'] });
	await context.addInitScript(() => localStorage.setItem('invisible-museum.audio', '1'));
	const page = await context.newPage();
	let momentFetches = 0;
	page.on('request', (r) => {
		if (/\/api\/v1\/moments\//.test(r.url())) momentFetches++;
	});
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText(/you have arrived/i)).toBeVisible({ timeout: 15_000 });
	await page.waitForTimeout(3000);
	expect(momentFetches).toBe(1);
	await context.close();
});
