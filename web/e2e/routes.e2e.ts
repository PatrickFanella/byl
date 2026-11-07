import { expect, test, type Page } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function signIn(page: Page, email: string) {
	await page.goto('/signin');
	await page.getByLabel('Email').fill(email);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
}

test('a visitor saves an owner-only draft until its path is reviewed', async ({
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
	await expect(page.getByText(/saved as a draft that only you can open/i)).toBeVisible();
	await expect(page.getByLabel('Share link')).toHaveCount(0);
	await page.getByRole('link', { name: 'Open the route' }).click();
	await expect(page.getByRole('heading', { name: 'Two stops', exact: true })).toBeVisible();
	await expect(
		page.getByText(
			'Draft · Only you can open this route. Its path needs review before publication or sharing.',
			{ exact: true }
		)
	).toBeVisible({
		timeout: 15_000
	});
	await expect(
		page.getByRole('link', { name: /start walking|start riding|open in the app/i })
	).toHaveCount(0);
	const draftURL = page.url();
	const stranger = await browser.newContext();
	const other = await stranger.newPage();
	await other.goto(draftURL);
	await expect(other.getByRole('heading', { name: 'Two stops' })).toHaveCount(0);
	await expect(other.getByText(/not found|could not|couldn't/i).first()).toBeVisible();
	await stranger.close();

	await page.goto('/routes');
	await expect(page.getByRole('link', { name: /two stops/i })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Share', exact: true })).toHaveCount(0);
	await expect(page.getByText(/draft · path review required before sharing/i)).toBeVisible();
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
