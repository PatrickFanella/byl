import { expect, test } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

test('a visitor browses from home to a tour, a place, and a moment', async ({ page }) => {
	await page.goto('/');
	await expect(page.getByRole('heading', { name: /downers grove on foot/i })).toBeVisible();
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

test('the tours page removes tours that do not match the selected travel mode and topic', async ({
	page,
	request
}) => {
	const login = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	expect(login.ok()).toBeTruthy();
	const token = (await login.json()).token as string;
	const headers = { Authorization: `Bearer ${token}` };
	const title = `Walk-only fixture ${Date.now()}`;
	const created = await request.post(`${apiUrl}/api/v1/admin/tours`, {
		headers,
		data: {
			city_id: '00000000-0000-4000-8000-0000000000c1',
			title,
			topic_id: '00000000-0000-4000-8000-00000000a0a1',
			modes: ['walk'],
			stops: [
				{ place_id: '00000000-0000-4000-8000-00000000b0b1' },
				{ place_id: '00000000-0000-4000-8000-00000000b0b2' }
			]
		}
	});
	expect(created.ok()).toBeTruthy();
	const fixture = (await created.json()) as { id: string };
	try {
		await expect(async () => {
			const published = await request.post(`${apiUrl}/api/v1/admin/tours/${fixture.id}/publish`, {
				headers
			});
			expect(published.ok()).toBeTruthy();
		}).toPass({ timeout: 30_000, intervals: [1000] });
		await page.goto('/tours');
		await expect(page.getByRole('link', { name: new RegExp(title) })).toBeVisible();
		await page.getByRole('button', { name: /biking/i }).click();
		await expect(page.getByRole('link', { name: new RegExp(title) })).toHaveCount(0);
		await expect(page.getByRole('link', { name: /downtown memory loop/i })).toBeVisible();
		await page.getByRole('button', { name: 'Any', exact: true }).click();
		await expect(page.getByRole('link', { name: new RegExp(title) })).toBeVisible();
		await page.getByRole('button', { name: 'Parks and Public Life', exact: true }).click();
		await expect(page.getByText('No tours match')).toBeVisible();
		await expect(page.getByRole('link', { name: new RegExp(title) })).toHaveCount(0);
	} finally {
		await request.delete(`${apiUrl}/api/v1/admin/tours/${fixture.id}`, { headers });
	}
});
