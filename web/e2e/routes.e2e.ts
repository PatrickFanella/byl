import { expect, test, type Page } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function signIn(page: Page, email: string) {
	await page.goto('/signin');
	await page.getByLabel('Email').fill(email);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
}

test('a signed-in visitor builds a route and a stranger opens the share link', async ({
	page,
	browser
}) => {
	await signIn(page, `builder-${Date.now()}@example.com`);
	await page.goto('/routes/new');
	await page.getByLabel('Name').fill('Two stops');
	await page.getByRole('button', { name: /^Add Main Street Station$/ }).click();
	await expect(page.getByRole('button', { name: /create route/i })).toBeDisabled();
	await expect(page.getByText(/add at least 2 stops/i)).toBeVisible();
	await page.getByRole('button', { name: /^Add Tivoli Theatre$/ }).click();
	await page.getByRole('button', { name: /create route/i }).click();
	await expect(page.getByText(/route created/i)).toBeVisible();
	const link = await page.getByLabel('Share link').inputValue();
	expect(link).toMatch(/\/t\/[A-Za-z0-9_-]{10}$/);

	// Review Focus 4: a signed-out stranger lands on the unlisted tour, never on sign-in.
	const stranger = await browser.newContext();
	const other = await stranger.newPage();
	await other.goto(link);
	await expect(other).toHaveURL(/\/tours\//);
	await expect(other.getByRole('heading', { name: 'Two stops' })).toBeVisible();
	await expect(other.getByRole('link', { name: /open in the app/i })).toBeVisible();
	await stranger.close();

	await page.goto('/routes');
	await expect(page.getByRole('link', { name: /two stops/i })).toBeVisible();
});

test('saving a tour lists it under Saved', async ({ page }) => {
	await signIn(page, `saver-${Date.now()}@example.com`);
	await page.goto('/tours/downtown-memory-loop');
	await page.getByRole('button', { name: /save for later/i }).click();
	await expect(page.getByRole('button', { name: /saved/i })).toBeVisible();
	await page.goto('/saved');
	await expect(page.getByRole('link', { name: /downtown memory loop/i })).toBeVisible();
});

test('the builder asks a signed-out visitor to sign in instead of failing on Create', async ({
	page
}) => {
	await page.goto('/routes/new');
	await expect(page.getByText(/sign in to build routes/i)).toBeVisible();
	await expect(page.getByRole('button', { name: /create route/i })).toHaveCount(0);
});
