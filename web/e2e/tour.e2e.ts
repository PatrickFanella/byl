import { expect, test, type APIRequestContext } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function devToken(request: APIRequestContext, email: string): Promise<string> {
	const res = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email, display_name: 'Admin' }
	});
	expect(res.ok()).toBeTruthy();
	return (await res.json()).token as string;
}

test('a curator builds a tour, waits for routes, and publishes it', async ({ page, request }) => {
	await devToken(request, 'admin@example.com');
	await page.goto('/signin');
	await page.getByLabel('Email').fill('admin@example.com');
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();

	const title = `E2E Night Walk ${Date.now()}`;
	await page.goto('/tours');
	await page.getByLabel('Title').fill(title);
	await page.getByLabel('Topic').selectOption({ label: 'Downtown After Dark' });
	await page.getByRole('button', { name: 'Create draft' }).click();
	await expect(page.getByRole('heading', { name: title })).toBeVisible();

	const picker = page.locator('select').filter({ hasText: 'Add a published place' });
	await picker.selectOption({ label: 'Tivoli Theatre' });
	await page.getByRole('button', { name: 'Add stop' }).click();
	await picker.selectOption({ label: 'Fishel Park' });
	await page.getByRole('button', { name: 'Add stop' }).click();
	await page.getByRole('button', { name: 'Save stops' }).click();
	await expect(page.getByText(/stops saved/i)).toBeVisible();

	// The worker computes straight-line routes within a few seconds; the page loads data after reload.
	await expect(async () => {
		await page.reload();
		await expect(page.locator('aside li', { hasText: 'walk' }).getByText('ready')).toBeVisible({
			timeout: 4000
		});
	}).toPass({ timeout: 45_000, intervals: [2000] });

	await page.getByRole('button', { name: 'Publish' }).click();
	await expect(page.getByText('Published.')).toBeVisible();

	const pub = await request.get(
		`${apiUrl}/api/v1/cities/downers-grove/tours?topic=downtown-after-dark`
	);
	const items = (await pub.json()).items as { title: string }[];
	expect(items.some((t) => t.title === title)).toBeTruthy();
});
