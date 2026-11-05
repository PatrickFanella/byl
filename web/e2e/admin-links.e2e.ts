import { expect, test, type Page } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function signIn(page: Page, email: string) {
	await page.goto('/signin');
	await page.getByLabel('Email').fill(email);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
}

// Review Focus 5: after the move every admin link must resolve under /admin.
test('every admin nav link resolves under /admin', async ({ page }) => {
	await signIn(page, 'admin@example.com');
	await page.goto('/admin');
	await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
	const hrefs = await page
		.locator('header nav a')
		.evaluateAll((as) => as.map((a) => a.getAttribute('href')));
	expect(hrefs.length).toBeGreaterThanOrEqual(7);
	for (const href of hrefs) {
		expect(href).toMatch(/^\/admin/);
		await page.goto(href!);
		await expect(page.getByRole('heading').first()).toBeVisible();
		await expect(page.getByText(/not found/i)).toHaveCount(0);
	}
});
