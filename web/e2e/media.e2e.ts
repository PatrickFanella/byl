import { expect, test } from '@playwright/test';
const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

test('a curator adds a hosted document and external video to a place and event', async ({
	page,
	request
}, testInfo) => {
	await page.route('https://byl.subcult.tv/media/files/library/**', async (route) => {
		const response = await route.fetch({
			url: `${apiUrl}${new URL(route.request().url()).pathname}`
		});
		await route.fulfill({ response });
	});
	const login = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	const headers = { Authorization: `Bearer ${(await login.json()).token}` };
	const placeID = '00000000-0000-4000-8000-00000000b0b2';
	const momentID = '00000000-0000-4000-8000-00000000d0d3';
	const originalPlace = await (
		await request.get(`${apiUrl}/api/v1/admin/places/${placeID}`, { headers })
	).json();
	const originalMoment = await (
		await request.get(`${apiUrl}/api/v1/admin/moments/${momentID}`, { headers })
	).json();
	const outbound: string[] = [];
	page.on('request', (req) => {
		if (req.url().startsWith('https://video.example.org')) outbound.push(req.url());
	});
	try {
		await page.goto('/signin');
		await page.getByLabel('Email', { exact: true }).fill('admin@example.com');
		await page.getByRole('button', { name: /sign in/i }).click();
		await expect(page).not.toHaveURL(/\/signin/);
		await page.goto('/admin/media');
		const upload = page
			.locator('form')
			.filter({ has: page.getByRole('button', { name: 'Upload file', exact: true }) });
		await upload.getByLabel('File', { exact: true }).setInputFiles({
			name: 'archive.pdf',
			mimeType: 'application/pdf',
			buffer: Buffer.from('%PDF-1.4\nA public archive record')
		});
		await upload.getByLabel('Title or caption', { exact: true }).fill('Theatre archive record');
		await upload.getByLabel('Creator or publisher', { exact: true }).fill('Local archive');
		await upload
			.getByLabel('Original source URL', { exact: true })
			.fill('https://archive.example.org/record');
		await upload.getByLabel('License', { exact: true }).fill('PD');
		await upload
			.getByLabel('License URL', { exact: true })
			.fill('https://archive.example.org/rights');
		const uploaded = page.waitForResponse(
			(r) => r.request().method() === 'POST' && r.url().endsWith('/api/v1/admin/media')
		);
		await upload.getByRole('button', { name: 'Upload file', exact: true }).click();
		const document = await (await uploaded).json();
		expect(document.kind).toBe('document');
		const link = page
			.locator('form')
			.filter({ has: page.getByRole('button', { name: 'Add link', exact: true }) });
		await link.getByLabel('Destination URL').fill('https://video.example.org/watch/theatre');
		await link.getByLabel('Title and relevance').fill('Theatre history documentary');
		await link.getByLabel('Creator or publisher').fill('Historical society');
		const linked = page.waitForResponse(
			(r) => r.request().method() === 'POST' && r.url().endsWith('/api/v1/admin/media/links')
		);
		await link.getByRole('button', { name: 'Add link', exact: true }).click();
		const video = await (await linked).json();
		expect(video.external).toBe(true);
		expect(video.storage_key).toBeUndefined();
		await page.screenshot({ path: testInfo.outputPath('library.png'), fullPage: true });
		await page.goto(`/admin/places/${placeID}`);
		for (const id of [document.id, video.id]) {
			await page.getByLabel('Attach media from library').selectOption(id);
			await page.getByRole('button', { name: 'Attach', exact: true }).click();
		}
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect(page.getByText('Saved.', { exact: true })).toBeVisible();
		await page.goto(`/admin/moments/${momentID}`);
		await page.getByLabel('Attach media from library').selectOption(video.id);
		await page.getByRole('button', { name: 'Attach', exact: true }).click();
		await page.getByRole('button', { name: 'Save', exact: true }).click();
		await expect(page.getByText('Saved.', { exact: true })).toBeVisible();
		await page.goto('/places/tivoli-theatre');
		const collection = page.getByRole('region', { name: 'From this place', exact: true });
		await expect(collection.getByRole('link', { name: /Theatre archive record/ })).toHaveAttribute(
			'href',
			document.public_url
		);
		await expect(
			collection.getByRole('link', { name: /Theatre history documentary/ })
		).toHaveAttribute('href', video.public_url);
		await expect(page.locator('video, iframe')).toHaveCount(0);
		expect(outbound).toEqual([]);
		await page.reload();
		await expect(collection.getByRole('heading', { name: 'Videos', exact: true })).toBeVisible();
		const downloaded = await request.get(document.public_url);
		expect(downloaded.headers()['content-disposition']).toBe('attachment');
		expect((await downloaded.body()).toString()).toContain('%PDF-1.4');
		await page.screenshot({ path: testInfo.outputPath('collection-desktop.png'), fullPage: true });
		await page.setViewportSize({ width: 390, height: 844 });
		expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
			false
		);
		await page.screenshot({ path: testInfo.outputPath('collection-mobile.png'), fullPage: true });
	} finally {
		await request.put(`${apiUrl}/api/v1/admin/places/${placeID}/media`, {
			headers,
			data: { media_ids: originalPlace.media.map((asset: { id: string }) => asset.id) }
		});
		await request.put(`${apiUrl}/api/v1/admin/moments/${momentID}/media`, {
			headers,
			data: { media_ids: originalMoment.media.map((asset: { id: string }) => asset.id) }
		});
	}
});
