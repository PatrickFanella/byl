import { expect, test } from '@playwright/test';

test('appearance follows the system, persists an override, and survives navigation', async ({
	page
}) => {
	await page.emulateMedia({ colorScheme: 'dark' });
	await page.goto('/profile');
	const picker = page.getByRole('combobox', { name: 'Appearance' });
	await expect(picker).toHaveValue('system');
	await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(45, 31, 43)');
	await picker.selectOption('light');
	await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(241, 237, 240)');
	await page.reload();
	await expect(picker).toHaveValue('light');
	await page.getByRole('link', { name: 'Saved', exact: true }).click();
	await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(241, 237, 240)');
	await page.getByRole('link', { name: 'Profile', exact: true }).click();
	await picker.selectOption('dark');
	await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(45, 31, 43)');
	await picker.selectOption('system');
	await page.emulateMedia({ colorScheme: 'light' });
	await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(241, 237, 240)');
});

for (const theme of ['light', 'dark'] as const) {
	for (const width of [320, 1440]) {
		test(`${theme} profile reflows at ${width}px with local fonts and visible navigation`, async ({
			page
		}, testInfo) => {
			await page.emulateMedia({ colorScheme: theme });
			await page.setViewportSize({ width, height: 900 });
			await page.goto('/profile');
			await expect(page.getByRole('heading', { name: 'Visitor', exact: true })).toBeVisible();
			await page.evaluate(() => document.fonts.ready);
			expect(
				await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
			).toBeTruthy();
			for (const name of ['Home', 'Tours', 'Routes', 'Saved', 'Profile']) {
				await expect(
					page.getByRole('navigation').getByRole('link', { name, exact: true })
				).toBeVisible();
			}
			expect(await page.evaluate(() => document.fonts.check('16px "Public Sans"'))).toBeTruthy();
			expect(
				await page.evaluate(() => document.fonts.check('32px "Libre Baskerville"'))
			).toBeTruthy();
			await page.screenshot({ path: testInfo.outputPath('profile.png'), fullPage: true });
			await page.evaluate(() => {
				document.documentElement.style.fontSize = '200%';
			});
			expect(
				await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
			).toBeTruthy();
			await page.evaluate(() => {
				document.documentElement.style.fontSize = '';
			});
			await page.getByRole('combobox', { name: 'Appearance' }).focus();
			await expect(page.getByRole('combobox', { name: 'Appearance' })).toHaveCSS(
				'outline-width',
				'3px'
			);
		});
	}
}

test('system appearance changes retain manual tour progress and arrival controls', async ({
	page
}) => {
	test.skip(!process.env.E2E_API_URL, 'E2E_API_URL not set');
	await page.emulateMedia({ colorScheme: 'light' });
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText(/next · stop 1/i)).toBeVisible();
	await page.getByRole('button', { name: /i'm here/i }).click();
	await page.getByRole('button', { name: /continue/i }).click();
	await expect(page.getByText(/next · stop 2/i)).toBeVisible();
	await page.emulateMedia({ colorScheme: 'dark' });
	await expect(page.locator('html')).toHaveCSS('background-color', 'rgb(45, 31, 43)');
	await expect(page.getByText(/next · stop 2/i)).toBeVisible();
	await page.getByRole('button', { name: /i'm here/i }).click();
	await expect(page.getByRole('dialog', { name: 'You have arrived' })).toBeVisible();
});
