import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function token(request: APIRequestContext) {
	const res = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	expect(res.ok()).toBeTruthy();
	return (await res.json()).token as string;
}

async function signIn(page: Page, email = 'admin@example.com') {
	await page.goto('/signin');
	await page.getByLabel('Email').fill(email);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
}

test('a curator saves a minimal brief, resumes all preferences, and uses the keyboard', async ({
	page
}, info) => {
	await signIn(page);
	await page.goto('/admin/briefs');
	await page.getByRole('link', { name: 'New route brief' }).click();
	await page.getByRole('button', { name: 'Save brief', exact: true }).click();
	await expect(page.getByLabel('Topic (required)')).toHaveAttribute('aria-invalid', 'true');
	await expect(page.getByLabel('Topic (required)')).toBeFocused();
	await page.getByLabel('Topic (required)').fill('Workers and the railroad');
	await page.keyboard.press('Tab');
	await expect(page.getByLabel('Area (required)')).toBeFocused();
	expect(
		await page.getByLabel('Area (required)').evaluate((el) => getComputedStyle(el).outlineWidth)
	).toBe('3px');
	await page.keyboard.type('Downers Grove');
	await page.getByRole('button', { name: 'Save brief', exact: true }).click();
	await expect(page).toHaveURL(/\/admin\/briefs\/[0-9a-f-]+\?saved=1$/);
	await expect(page.getByRole('status')).toHaveText('Brief saved.');
	const savedUrl = page.url().split('?')[0];
	await page.getByLabel('Submitted notes').fill('Find out how workers used the station.');
	await page.getByLabel('Place leads').fill('Old depot, address unknown\nMain Street Station');
	await page.getByLabel('Event leads').fill('The opening of the depot');
	await page.getByLabel('Source links').fill('https://example.org/archive');
	await page.getByLabel('Start preference').fill('Rail station');
	await page.getByLabel('End preference').fill('Return to the station');
	await page.getByLabel('Pilot identifier').fill('downers-labor-01');
	await page.getByLabel('Cycling', { exact: true }).uncheck();
	await page.getByLabel('Trip shape').selectOption('loop');
	await page.getByLabel('Duration in minutes (optional)').fill('90');
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect(page.getByText('Version 2', { exact: false })).toBeVisible();
	await page.goto('/admin/briefs');
	await page.getByRole('link', { name: 'Workers and the railroad', exact: true }).click();
	await expect(page).toHaveURL(savedUrl);
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Find out how workers used the station.'
	);
	await expect(page.getByLabel('Place leads')).toHaveValue(
		'Old depot, address unknown\nMain Street Station'
	);
	await expect(page.getByLabel('Event leads')).toHaveValue('The opening of the depot');
	await expect(page.getByLabel('Source links')).toHaveValue('https://example.org/archive');
	await expect(page.getByLabel('Start preference')).toHaveValue('Rail station');
	await expect(page.getByLabel('End preference')).toHaveValue('Return to the station');
	await expect(page.getByLabel('Pilot identifier')).toHaveValue('downers-labor-01');
	await expect(page.getByLabel('Cycling', { exact: true })).not.toBeChecked();
	await expect(page.getByLabel('Walking', { exact: true })).toBeChecked();
	await expect(page.getByLabel('Trip shape')).toHaveValue('loop');
	await expect(page.getByLabel('Duration in minutes (optional)')).toHaveValue('90');
	await page.screenshot({ path: info.outputPath('brief-desktop.png'), fullPage: true });
	await page.setViewportSize({ width: 390, height: 844 });
	await expect(page.getByLabel('Area (required)')).toBeVisible();
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('brief-mobile.png'), fullPage: true });
});

test('field errors and stale saves retain unsaved edits', async ({ page, request }) => {
	const auth = await token(request);
	const res = await request.post(`${apiUrl}/api/v1/admin/route-briefs`, {
		headers: { Authorization: `Bearer ${auth}` },
		data: { topic: 'Architecture', area: 'Chicagoland' }
	});
	expect(res.status()).toBe(201);
	const brief = await res.json();
	await signIn(page);
	await page.goto(`/admin/briefs/${brief.id}`);
	await page.getByLabel('Submitted notes').fill('Keep this unsaved research question.');
	await page.getByLabel('Source links').fill('not-a-link');
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect(page.getByLabel('Source links')).toHaveAttribute('aria-invalid', 'true');
	await expect(page.getByLabel('Source links')).toBeFocused();
	await expect(page.getByLabel('Source links')).toHaveAccessibleDescription(
		/HTTP or HTTPS links without credentials/
	);
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Keep this unsaved research question.'
	);
	await page.getByLabel('Source links').fill('https://example.org/architecture');
	const changed = await request.put(`${apiUrl}/api/v1/admin/route-briefs/${brief.id}`, {
		headers: { Authorization: `Bearer ${auth}` },
		data: { version: 1, submitted: { ...brief.submitted, notes: 'Saved in another tab' } }
	});
	expect(changed.ok()).toBeTruthy();
	await page.getByRole('button', { name: 'Save changes' }).click();
	await expect(page.getByRole('alert')).toContainText('changed since you opened it');
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Keep this unsaved research question.'
	);
	const saved = await request.get(`${apiUrl}/api/v1/admin/route-briefs/${brief.id}`, {
		headers: { Authorization: `Bearer ${auth}` }
	});
	expect((await saved.json()).submitted.notes).toBe('Saved in another tab');
});

test('retrying after a lost creation response resumes the same saved brief', async ({
	page,
	request
}) => {
	const auth = await token(request);
	await signIn(page);
	let lost = false;
	await page.route('**/api/v1/admin/route-briefs', async (route) => {
		if (route.request().method() === 'POST' && !lost) {
			lost = true;
			const response = await route.fetch();
			expect(response.status()).toBe(201);
			await route.abort();
		} else await route.continue();
	});
	await page.goto('/admin/briefs/new');
	await page.getByLabel('Topic (required)').fill('Retry after response loss');
	await page.getByLabel('Area (required)').fill('Downers Grove');
	await page.getByRole('button', { name: 'Save brief', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('Your edits are still here');
	await page.getByRole('button', { name: 'Save brief', exact: true }).click();
	await expect(page).toHaveURL(/\/admin\/briefs\/[0-9a-f-]+\?saved=1$/);
	const res = await request.get(`${apiUrl}/api/v1/admin/route-briefs`, {
		headers: { Authorization: `Bearer ${auth}` }
	});
	expect(
		(await res.json()).items.filter(
			(brief: { submitted: { topic: string } }) =>
				brief.submitted.topic === 'Retry after response loss'
		)
	).toHaveLength(1);
});

test('brief progress shows research reasons and processing disables edits', async ({
	page,
	request
}) => {
	const auth = await token(request);
	const headers = { Authorization: `Bearer ${auth}` };
	const res = await request.post(`${apiUrl}/api/v1/admin/route-briefs`, {
		headers,
		data: { topic: 'The river', area: 'DuPage County' }
	});
	expect(res.status()).toBe(201);
	const brief = await res.json();
	const path = `${apiUrl}/api/v1/admin/route-briefs/${brief.id}/state`;
	expect(
		(
			await request.post(path, {
				headers,
				data: { version: 1, status: 'processing', reason: 'Research queued' }
			})
		).ok()
	).toBeTruthy();
	await signIn(page);
	await page.goto(`/admin/briefs/${brief.id}`);
	await expect(page.getByText('State: processing', { exact: true })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save changes' })).toBeDisabled();
	expect(
		(
			await request.post(path, {
				headers,
				data: {
					version: 2,
					status: 'needs_research',
					reason: 'No verified source for the proposed crossing.'
				}
			})
		).ok()
	).toBeTruthy();
	await page.getByRole('button', { name: 'Reload saved brief' }).click();
	await expect(page.getByText('State: needs research', { exact: true })).toBeVisible();
	await expect(
		page.getByText('No verified source for the proposed crossing.', { exact: false })
	).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save changes' })).toBeEnabled();
});

test('signed-out and visitor accounts cannot load the curator workspace or its records', async ({
	page
}) => {
	const adminCalls: string[] = [];
	page.on('request', (req) => {
		if (req.url().includes('/api/v1/admin/route-briefs')) adminCalls.push(req.url());
	});
	await page.goto('/admin/briefs');
	await expect(page).toHaveURL(/\/signin\?next=/);
	await signIn(page, `brief-visitor-${Date.now()}@example.com`);
	await page.goto('/admin/briefs');
	await expect(page.getByRole('heading', { name: /needs the curator role/i })).toBeVisible();
	await page.goto('/admin/briefs/new');
	await expect(page.getByRole('heading', { name: /needs the curator role/i })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Save brief', exact: true })).toHaveCount(0);
	await page.goto('/admin/briefs/00000000-0000-4000-8000-000000000000');
	await expect(page.getByRole('heading', { name: /needs the curator role/i })).toBeVisible();
	expect(adminCalls).toEqual([]);
});

test('a curator inspects a private itinerary without losing unsaved brief edits', async ({
	page,
	request
}, info) => {
	const auth = await token(request);
	const created = await request.post(`${apiUrl}/api/v1/admin/route-briefs`, {
		headers: { Authorization: `Bearer ${auth}` },
		data: {
			topic: 'Rail workers',
			area: 'Downers Grove',
			trip_shape: 'out_and_back',
			duration_minutes: 30
		}
	});
	expect(created.ok()).toBeTruthy();
	const brief = await created.json();
	const seed = await request.get(`${apiUrl}/api/v1/tours/downtown-memory-loop`);
	const tour = await seed.json();
	const stops = tour.stops.slice(0, 2).map(
		(
			stop: {
				place: { id: string; name: string; city_id: string; latitude: number; longitude: number };
				featured_moment: { id: string; title: string };
			},
			i: number
		) => ({
			material: {
				record_id: `00000000-0000-4000-8000-00000000000${i}`,
				review_version: 0,
				place_id: stop.place.id,
				place_name: stop.place.name,
				city_id: stop.place.city_id,
				latitude: stop.place.latitude,
				longitude: stop.place.longitude,
				moment_id: stop.featured_moment.id,
				moment_title: stop.featured_moment.title,
				narrative: 'Retained source-linked historical paragraph for this browser fixture.',
				evidence_kind: 'curated_record',
				content_sha256: 'a'.repeat(64),
				source_urls: ['https://example.org/fixture'],
				evidence_ids: []
			},
			theme_reason: 'Matched requested theme: rail',
			what_to_notice: 'Curator research question: which traces remain visible today?',
			transition:
				i === 0 ? 'Continue the theme at the next researched place; the path needs review.' : '',
			planned_dwell_minutes: 3,
			planned_narration_minutes: 1
		})
	);
	// Only proposal transport is a fixture. Worker source capture, B5 validation,
	// no-publication and replay behavior run independently against PostgreSQL.
	await page.route(`**/api/v1/admin/route-briefs/${brief.id}/itinerary`, (route) =>
		route.fulfill({
			json: {
				run_id: '00000000-0000-4000-8000-000000000011',
				source_version: 1,
				brief_version: 2,
				proposal: {
					version: 1,
					status: 'draft',
					created_at: new Date().toISOString(),
					title: 'Private rail proposal',
					introduction: 'A private selection of researched histories.',
					requested_shape: 'out_and_back',
					trip_shape: 'out_and_back',
					modes: ['walk', 'bike'],
					target_duration_minutes: 30,
					travel_minutes: null,
					travel_state: 'not_computed',
					planned_dwell_minutes: 6,
					planned_narration_minutes: 2,
					stops,
					travel_sequence: [0, 1, 0],
					excluded: [],
					research_gaps: [
						{
							code: 'viewing_guidance',
							detail: 'Current visible features still need source review.'
						}
					],
					assumptions: ['Actual travel and access require separate review.']
				}
			}
		})
	);
	await page.route(`**/api/v1/admin/route-briefs/${brief.id}/research`, (route) =>
		route.fulfill({
			status: 202,
			json: { id: '00000000-0000-4000-8000-000000000011', status: 'active' }
		})
	);
	await signIn(page);
	await page.goto(`/admin/briefs/${brief.id}`);
	await expect(page.getByLabel('Trip shape')).toHaveValue('out_and_back');
	await page
		.getByLabel('Submitted notes')
		.fill('Unsaved curator wording must survive research and readback.');
	const queued = page.waitForRequest(
		(r) => r.url().endsWith(`/route-briefs/${brief.id}/research`) && r.method() === 'POST'
	);
	await page.getByRole('button', { name: 'Start research from saved brief', exact: true }).click();
	expect((await queued).postDataJSON()).toEqual({ version: 1 });
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Unsaved curator wording must survive research and readback.'
	);
	await expect(page.getByText('State: research requested', { exact: true })).toBeVisible();
	await expect(
		page.getByRole('button', { name: 'Inspect itinerary proposal', exact: true })
	).toBeEnabled();
	await page.getByRole('button', { name: 'Inspect itinerary proposal', exact: true }).focus();
	await page.keyboard.press('Enter');
	await expect(page.getByRole('heading', { name: 'Private rail proposal' })).toBeVisible();
	await expect(page.getByText('Not computed', { exact: true })).toBeVisible();
	await expect(page.getByText('Travel sequence:', { exact: true })).toBeVisible();
	await expect(page.getByText('Current visible features still need source review.')).toBeVisible();
	await expect(page.getByRole('link', { name: 'Source 1', exact: true })).toHaveCount(2);
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Unsaved curator wording must survive research and readback.'
	);
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('itinerary-mobile.png'), fullPage: true });
});
