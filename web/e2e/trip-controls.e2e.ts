import { expect, test } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

for (const width of [320, 375]) {
	for (const scale of [1, 2]) {
		test(`long trip title keeps End and Audio usable at ${width}px and ${scale * 100}% text`, async ({
			page,
			request
		}, info) => {
			const tour = await (await request.get(`${apiUrl}/api/v1/tours/downtown-memory-loop`)).json();
			const revision = await (
				await request.get(`${apiUrl}/api/v1/tours/${tour.id}/revisions/${tour.revision_id}`)
			).json();
			const title =
				'A very long historical itinerary title through the railway station, theatre, park and museum campus';
			tour.title = title;
			revision.snapshot.tour.title = title;
			await page.route('**/api/v1/tours/downtown-memory-loop', (route) =>
				route.fulfill({ json: tour })
			);
			await page.route(`**/api/v1/tours/${tour.id}/revisions/${revision.id}`, (route) =>
				route.fulfill({ json: revision })
			);
			await page.route('https://tiles.openfreemap.org/styles/**', (route) =>
				route.fulfill({ json: { version: 8, sources: {}, layers: [] } })
			);
			await page.setViewportSize({ width, height: 800 });
			await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
			await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
			await expect(page.locator('.byl-walk > header')).toContainText(title);
			await page.evaluate((scale) => {
				document.documentElement.style.fontSize = `${scale * 100}%`;
			}, scale);
			await page.evaluate(() => document.fonts.ready);
			for (const button of [
				page.getByRole('button', { name: 'End', exact: true }),
				page.getByRole('button', { name: 'Audio guide off', exact: true })
			]) {
				const box = await button.boundingBox();
				expect(box).not.toBeNull();
				expect(box!.x).toBeGreaterThanOrEqual(0);
				expect(box!.x + box!.width).toBeLessThanOrEqual(width);
			}
			await page.getByRole('button', { name: 'Audio guide off', exact: true }).focus();
			await expect(page.getByRole('button', { name: 'Audio guide off', exact: true })).toHaveCSS(
				'outline-width',
				'3px'
			);
			await page.getByRole('button', { name: 'Audio guide off', exact: true }).click();
			await expect(page.getByRole('button', { name: 'Audio guide on', exact: true })).toBeVisible();
			await page.getByRole('button', { name: 'Audio guide on', exact: true }).click();
			await page.screenshot({ path: info.outputPath('trip-controls.png') });
			await page.getByRole('button', { name: 'End', exact: true }).click();
			await expect(page.getByRole('dialog', { name: 'End this tour?' })).toBeVisible();
			await page.getByRole('button', { name: 'Cancel', exact: true }).click();
			await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
		});
	}
}
