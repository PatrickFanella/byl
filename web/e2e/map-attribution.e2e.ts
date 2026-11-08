import { expect, test } from '@playwright/test';

test.skip(!process.env.E2E_API_URL, 'E2E_API_URL not set');

test('third-party map attribution strips adjacent event handlers and loads the bundled worker', async ({
	page
}) => {
	const workerUrls: string[] = [];
	page.on('worker', (worker) => workerUrls.push(worker.url()));
	await page.route('https://tiles.openfreemap.org/styles/**', (route) =>
		route.fulfill({
			json: {
				version: 8,
				sources: {
					probe: {
						type: 'geojson',
						data: {
							type: 'FeatureCollection',
							features: [
								{
									type: 'Feature',
									properties: {},
									geometry: { type: 'Point', coordinates: [-88.01, 41.8] }
								}
							]
						},
						attribution: `<details open onload="void 0" ontoggle="document.documentElement.dataset.mapAttributionExecuted='yes'">Attribution regression probe</details>`
					}
				},
				layers: [{ id: 'probe', type: 'circle', source: 'probe' }]
			}
		})
	);
	await page.goto('/tours/downtown-memory-loop');
	const attribution = page.locator('.maplibregl-ctrl-attrib-inner');
	await expect(attribution).toContainText('Attribution regression probe');
	// GHSA-jrc7-96c5-q579 removed one handler but skipped the adjacent one.
	await expect(attribution.locator('[onload], [ontoggle]')).toHaveCount(0);
	expect(
		await page.evaluate(() => document.documentElement.dataset.mapAttributionExecuted)
	).toBeUndefined();
	await expect(page.locator('.maplibregl-marker').first()).toBeVisible();
	expect(workerUrls.length).toBeGreaterThan(0);
	for (const worker of page.workers()) {
		await expect.poll(() => worker.evaluate(() => Reflect.has(globalThis, 'worker'))).toBe(true);
	}
	expect(new URL(workerUrls[0]).origin).toBe(new URL(page.url()).origin);
	expect((await page.request.get(workerUrls[0])).status()).toBe(200);
});
