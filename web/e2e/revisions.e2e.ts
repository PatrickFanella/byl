import { expect, test, type Page, type APIRequestContext } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function releases(page: Page, request: APIRequestContext) {
	const tour = await (await request.get(`${apiUrl}/api/v1/tours/downtown-memory-loop`)).json();
	const original = await (
		await request.get(`${apiUrl}/api/v1/tours/${tour.id}/revisions/${tour.revision_id}`)
	).json();
	const newer = structuredClone(original);
	newer.id = '00000000-0000-4000-8000-00000000ffff';
	newer.number++;
	newer.snapshot.tour.revision_id = newer.id;
	newer.snapshot.tour.revision_number = newer.number;
	const stops = newer.snapshot.tour.stops;
	newer.snapshot.tour.stops = [stops[1], stops[0], ...stops.slice(2)].map((s, i) => ({
		...s,
		position: i + 1
	}));
	newer.snapshot.tour.stops[0].note = 'Urgent reviewed access guidance: use the east entrance.';
	let published = false;
	// Synthetic publication transport isolates client adoption. Snapshot/approval
	// immutability and transaction races have separate actual PostgreSQL tests.
	await page.route(`**/api/v1/tours/${tour.id}/revisions?*`, (route) =>
		route.fulfill({ json: { items: published ? [newer, original] : [original] } })
	);
	await page.route(`**/api/v1/tours/${tour.id}/revisions/${newer.id}`, (route) =>
		route.fulfill({ json: newer })
	);
	return { tour, original, newer, publish: () => (published = true) };
}

test('an active trip stays pinned through publication and adopts a reordered revision explicitly', async ({
	page,
	request
}, info) => {
	const f = await releases(page, request);
	let mutableRoutes = 0;
	page.on('request', (r) => {
		if (/\/api\/v1\/tours\/[^/]+\/route\?/.test(r.url())) mutableRoutes++;
	});
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	await page.getByRole('button', { name: 'Continue to the next stop' }).click();
	f.publish();
	await page.getByRole('button', { name: 'Check for route updates' }).click();
	await expect(
		page.getByText(`Revision ${f.newer.number} is available. Your current trip has not changed.`)
	).toBeVisible();
	await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
	await expect(page.getByText('Next · stop 2 of 4', { exact: true })).toBeVisible();
	await expect(
		page.getByText('Urgent reviewed access guidance: use the east entrance.', { exact: false })
	).toBeVisible();
	await page
		.getByRole('button', { name: `Use revision ${f.newer.number} with its own progress` })
		.click();
	await expect(page.getByText('0 of 4 stops', { exact: true })).toBeVisible();
	await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
	await expect(page.getByText('Tivoli Theatre', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	await page.getByRole('button', { name: 'Continue to the next stop' }).click();
	await page.reload();
	await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
	await expect(page.getByText('Main Street Station', { exact: true })).toBeVisible();
	const visits = await page.evaluate(
		({ tour, revision }) =>
			JSON.parse(
				localStorage.getItem(`invisible-museum.revision-progress.anonymous.${tour}.${revision}`) ??
					'{}'
			).visited,
		{ tour: f.tour.id, revision: f.newer.id }
	);
	expect(visits).toEqual([f.newer.snapshot.tour.stops[0].id]);
	expect(mutableRoutes).toBe(0);
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('revision-trip-mobile.png') });
});

test('API outage resumes the cached revision and reconnect does not substitute a newer itinerary', async ({
	page,
	request
}) => {
	const f = await releases(page, request);
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	await page.getByRole('button', { name: 'Continue to the next stop' }).click();
	await page.route('**/api/v1/**', (route) => route.abort('internetdisconnected'));
	await page.reload();
	await expect(page.getByText('Offline: using the saved route')).toBeVisible();
	await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
	await page.unroute('**/api/v1/**');
	f.publish();
	await page.getByRole('button', { name: 'Check for route updates' }).click();
	await expect(
		page.getByText(`Revision ${f.newer.number} is available. Your current trip has not changed.`)
	).toBeVisible();
	await expect(
		page.getByText(`Following revision ${f.original.number}.`, { exact: false })
	).toBeVisible();
});

test('an unavailable pinned revision cannot silently fall forward to the latest release', async ({
	page,
	request
}) => {
	const f = await releases(page, request);
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
	f.publish();
	await page.route(`**/api/v1/tours/${f.tour.id}/revisions/${f.original.id}`, (route) =>
		route.fulfill({
			status: 404,
			json: { error: { code: 'not_found', message: 'Unavailable old release' } }
		})
	);
	await page.reload();
	await expect(
		page.getByText('This saved tour revision is no longer available.', { exact: false })
	).toBeVisible();
	await expect(page.getByRole('button', { name: "I'm here", exact: true })).toHaveCount(0);
});

test('an unsupported mixed tour cannot start an older unpinned cached walk', async ({
	page,
	request
}, info) => {
	const f = await releases(page, request);
	await page.goto('/');
	await page.evaluate(
		({ tour, revision }) => {
			localStorage.setItem('invisible-museum.tour.downtown-memory-loop', JSON.stringify(tour));
			localStorage.setItem(
				`invisible-museum.route.${tour.id}.walk`,
				JSON.stringify(revision.snapshot.routes.walk)
			);
			localStorage.setItem(
				`invisible-museum.revision.anonymous.${tour.id}.${revision.id}`,
				JSON.stringify(revision)
			);
		},
		{ tour: f.original.snapshot.tour, revision: f.original }
	);
	// Transport fixture isolates legacy fallback. The authoritative 403 response
	// and unchanged old-pin access are independently tested against PostgreSQL.
	await page.route('**/api/v1/tours/downtown-memory-loop', (route) =>
		route.fulfill({
			status: 403,
			json: { error: { code: 'unsupported_itinerary', message: 'Enabled client support required' } }
		})
	);
	let oldRevisionReads = 0;
	page.on('request', (request) => {
		if (request.url().endsWith(`/revisions/${f.original.id}`)) oldRevisionReads++;
	});
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(
		page.getByText('This tour is no longer available online.', { exact: true })
	).toBeVisible();
	await expect(page.getByRole('button', { name: "I'm here", exact: true })).toHaveCount(0);
	expect(oldRevisionReads).toBe(0);
	await page.setViewportSize({ width: 390, height: 844 });
	await page.screenshot({ path: info.outputPath('unsupported-transit-mobile.png') });
});

test('unsupported modes are refused, and signed-in visits use the exact revision and stable stop ID', async ({
	page,
	request
}) => {
	const f = await releases(page, request);
	const onlyWalk = structuredClone(f.original);
	onlyWalk.snapshot.tour.modes = ['walk'];
	await page.route(`**/api/v1/tours/${f.tour.id}/revisions/${f.original.id}`, (route) =>
		route.fulfill({ json: onlyWalk })
	);
	await page.goto('/tours/downtown-memory-loop/walk?mode=bike');
	await expect(
		page.getByText('This travel mode is not available in the pinned revision.', { exact: false })
	).toBeVisible();
	await page.goto('/signin');
	await page.getByLabel('Email').fill(`revision-progress-${Date.now()}@example.com`);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
	const update = page.waitForRequest(
		(r) =>
			r.method() === 'PUT' && r.url().endsWith(`/progress/${f.tour.id}/revisions/${f.original.id}`)
	);
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	expect((await update).postDataJSON()).toMatchObject({
		visited_stop_ids: [f.original.snapshot.tour.stops[0].id],
		mode: 'walk'
	});
	await page.getByRole('button', { name: 'Continue to the next stop' }).click();
	await page.reload();
	await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
	await page.getByRole('button', { name: 'End', exact: true }).click();
	await page.getByRole('button', { name: 'Keep my progress and end', exact: true }).click();
	await page.getByRole('button', { name: '♡ Save for later', exact: true }).click();
	await expect(page.getByRole('button', { name: '♥ Saved', exact: true })).toBeVisible();
	await page.goto('/saved');
	await page.getByRole('link', { name: 'Resume saved revision and its progress' }).click();
	await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
});

test('a visit in another tab updates the open pinned walk', async ({ page }) => {
	await page.goto('/signin');
	await page.getByLabel('Email').fill(`revision-tab-sync-${Date.now()}@example.com`);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
	await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
	await expect(page.getByText('0 of 4 stops', { exact: true })).toBeVisible();

	const other = await page.context().newPage();
	try {
		await other.goto('/tours/downtown-memory-loop/walk?mode=walk');
		await expect(other.getByText('0 of 4 stops', { exact: true })).toBeVisible();
		await other.getByRole('button', { name: "I'm here", exact: true }).click();
		await expect(other.getByText('1 of 4 stops', { exact: true })).toBeVisible();

		await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
		await expect(page.getByText('Next · stop 2 of 4', { exact: true })).toBeVisible();
	} finally {
		await other.close();
	}
});

test('a visit in another tab updates a walk without geolocation', async ({ browser }) => {
	const context = await browser.newContext();
	await context.addInitScript(() => {
		const nativeNavigator = window.navigator;
		Object.defineProperty(window, 'navigator', {
			configurable: true,
			value: new Proxy(nativeNavigator, {
				has(target, key) {
					return key === 'geolocation' ? false : Reflect.has(target, key);
				},
				get(target, key) {
					const value = Reflect.get(target, key, target);
					return typeof value === 'function' ? value.bind(target) : value;
				}
			})
		});
	});
	const page = await context.newPage();
	try {
		await page.goto('/signin');
		await page.getByLabel('Email').fill(`revision-no-geo-tab-sync-${Date.now()}@example.com`);
		await page.getByRole('button', { name: /sign in/i }).click();
		await expect(page).not.toHaveURL(/\/signin/);
		await page.goto('/tours/downtown-memory-loop/walk?mode=walk');
		await expect(
			page.getByText("Your location is unavailable right now. Tap I'm here at each stop.", {
				exact: true
			})
		).toBeVisible();
		await expect(page.getByText('0 of 4 stops', { exact: true })).toBeVisible();

		const other = await context.newPage();
		try {
			await other.goto('/tours/downtown-memory-loop/walk?mode=walk');
			await expect(other.getByText('0 of 4 stops', { exact: true })).toBeVisible();
			await other.getByRole('button', { name: "I'm here", exact: true }).click();
			await expect(other.getByText('1 of 4 stops', { exact: true })).toBeVisible();

			await expect(page.getByText('1 of 4 stops', { exact: true })).toBeVisible();
			await expect(page.getByText('Next · stop 2 of 4', { exact: true })).toBeVisible();
		} finally {
			await other.close();
		}
	} finally {
		await context.close();
	}
});
