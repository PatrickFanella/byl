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

test('a computed development route stays a draft with actionable quality findings', async ({
	page,
	request
}) => {
	await devToken(request, 'admin@example.com');
	await page.goto('/signin');
	await page.getByLabel('Email').fill('admin@example.com');
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
	await page.goto('/admin');
	await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();

	const title = `E2E Night Walk ${Date.now()}`;
	await page.goto('/admin/tours');
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

	await expect(page.getByRole('button', { name: 'Publish', exact: true })).toBeDisabled();
	await expect(
		page.getByText(
			'Publication requires a current passing path assessment for every selected mode.'
		)
	).toBeVisible();
	await expect(page.getByText('Publication blocked', { exact: true })).toHaveCount(2);
	await page.getByRole('button', { name: 'Inspect retained evidence' }).first().click();
	await expect(
		page.getByText('No independently inspected source passages were retained.')
	).toBeVisible();
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)
	).toBeTruthy();
	const pub = await request.get(
		`${apiUrl}/api/v1/cities/downers-grove/tours?topic=downtown-after-dark`
	);
	const items = (await pub.json()).items as { title: string }[];
	expect(items.some((t) => t.title === title)).toBeFalsy();
});
