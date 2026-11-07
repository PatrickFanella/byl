import { expect, test } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

test.beforeEach(async ({ page }) => {
	// Check the packaged images against this test's API, not production.
	await page.route('https://byl.subcult.tv/media/files/library/**', async (route) => {
		const response = await route.fetch({
			url: `${apiUrl}${new URL(route.request().url()).pathname}`
		});
		await route.fulfill({ response });
	});
});

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
	await expect(page.getByRole('heading', { name: 'Built for the Talkies' })).toBeVisible();
	await expect(page.getByRole('img', { name: /Tivoli marquee/ })).toBeVisible();
	await expect
		.poll(() =>
			page
				.getByRole('img', { name: /Tivoli marquee/ })
				.evaluate((img) => (img as HTMLImageElement).naturalWidth)
		)
		.toBeGreaterThan(0);
	await expect(
		page
			.locator('figure')
			.filter({ has: page.getByRole('img', { name: /Tivoli marquee/ }) })
			.getByRole('link', { name: 'Image source', exact: true })
	).toBeVisible();
	await expect(page.locator('article p')).toHaveCount(5);
	await expect(
		page.getByRole('navigation', { name: 'Events and stories at this place' })
	).toHaveCount(0);
	await expect(page.getByRole('heading', { name: /sources/i })).toBeVisible();
});

test('a place lists its events and opens a selected history in place', async ({ page }) => {
	await page.goto('/places/main-street-station');
	const events = page.getByRole('navigation', { name: 'Events and stories at this place' });
	await expect(events.getByRole('link')).toHaveCount(3);
	await events.getByRole('link', { name: /Twin-City Zephyr Crash/ }).click();
	await expect(page).toHaveURL(/featured=.*#story/);
	await expect(
		page.getByRole('heading', { name: 'The Twin-City Zephyr Crash', exact: true })
	).toBeVisible();
	await expect(page.locator('article')).toContainText('May 26, 1947');
	await expect(events.getByRole('link', { name: /Twin-City Zephyr Crash/ })).toHaveAttribute(
		'aria-current',
		'true'
	);
	await page.reload();
	await expect(page.locator('article')).toContainText('May 26, 1947');
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

test('shared topics with the same slug filter tours by their distinct IDs', async ({
	page,
	request
}) => {
	const login = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	expect(login.ok()).toBeTruthy();
	const headers = { Authorization: `Bearer ${(await login.json()).token}` };
	const suffix = Date.now();
	const cityResponse = await request.post(`${apiUrl}/api/v1/admin/cities`, {
		headers,
		data: {
			name: `Lisle fixture ${suffix}`,
			center_lat: 41.8,
			center_lng: -88.1,
			bbox_min_lat: 41.7,
			bbox_max_lat: 41.9,
			bbox_min_lng: -88.2,
			bbox_max_lng: -88
		}
	});
	expect(cityResponse.ok()).toBeTruthy();
	const city = await cityResponse.json();
	const original = await request.get(`${apiUrl}/api/v1/cities/downers-grove`);
	const local = (await original.json()).topics.find(
		(t: { id: string }) => t.id === '00000000-0000-4000-8000-00000000a0a1'
	);
	const name = `Regional rail ${suffix}`;
	const topicResponse = await request.post(`${apiUrl}/api/v1/admin/topics`, {
		headers,
		data: {
			city_id: city.id,
			city_ids: ['00000000-0000-4000-8000-0000000000c1'],
			slug: local.slug,
			name
		}
	});
	expect(topicResponse.ok()).toBeTruthy();
	const shared = await topicResponse.json();
	const title = `Regional fixture ${suffix}`;
	const created = await request.post(`${apiUrl}/api/v1/admin/tours`, {
		headers,
		data: {
			city_id: '00000000-0000-4000-8000-0000000000c1',
			topic_id: shared.id,
			title,
			modes: ['walk'],
			stops: [
				{ place_id: '00000000-0000-4000-8000-00000000b0b1' },
				{ place_id: '00000000-0000-4000-8000-00000000b0b2' }
			]
		}
	});
	expect(created.ok()).toBeTruthy();
	const fixture = await created.json();
	try {
		await expect(async () => {
			const published = await request.post(`${apiUrl}/api/v1/admin/tours/${fixture.id}/publish`, {
				headers
			});
			expect(published.ok()).toBeTruthy();
		}).toPass({ timeout: 30_000, intervals: [1000] });
		await page.goto('/tours');
		const selected = page.waitForRequest((r) => {
			const url = new URL(r.url());
			return url.pathname.endsWith('/tours') && url.searchParams.has('topic');
		});
		await page.getByRole('button', { name, exact: true }).click();
		expect(new URL((await selected).url()).searchParams.get('topic')).toBe(shared.id);
		await expect(page.getByRole('link', { name: new RegExp(title) })).toBeVisible();
		await expect(page.getByRole('link', { name: /downtown memory loop/i })).toHaveCount(0);
		await page.getByRole('button', { name: local.name, exact: true }).click();
		await expect(page.getByRole('link', { name: /downtown memory loop/i })).toBeVisible();
		await expect(page.getByRole('link', { name: new RegExp(title) })).toHaveCount(0);
	} finally {
		await request.delete(`${apiUrl}/api/v1/admin/tours/${fixture.id}`, { headers });
	}
});
