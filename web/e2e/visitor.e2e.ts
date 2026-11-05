import { expect, test } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

test('a visitor browses from home to a tour, a place, and a moment', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: /downers grove as a museum/i })).toBeVisible();
	await page
		.getByRole('link', { name: /downtown memory loop/i })
		.first()
		.click();
	await expect(page).toHaveURL(/\/tours\/downtown-memory-loop$/);
	await expect(page.getByRole('heading', { name: 'Downtown Memory Loop' })).toBeVisible();
	await expect(page.getByTestId('stop-row')).toHaveCount(4);
	// Routes are computed by the worker shortly after the seed; wait for the estimate.
	await expect(async () => {
		await page.reload();
		await expect(page.getByText(/min walk/).first()).toBeVisible({ timeout: 2000 });
	}).toPass({ timeout: 60_000 });

	await page.getByTestId('stop-row').nth(1).click();
	await expect(page).toHaveURL(/\/places\/.+\?featured=/);
	await expect(page.getByText(/featured on this tour/i)).toBeVisible();
	await page.getByTestId('featured-moment').click();
	await expect(page).toHaveURL(/\/moments\//);
	await expect(page.getByRole('heading', { name: /sources/i })).toBeVisible();
});

test('the tours page filters by travel mode and topic', async ({ page }) => {
	await page.goto('/tours');
	await expect(page.getByRole('link', { name: /downtown memory loop/i })).toBeVisible();
	await page.getByRole('button', { name: /biking/i }).click();
	await expect(page.getByRole('link', { name: /downtown memory loop/i })).toBeVisible();
});
