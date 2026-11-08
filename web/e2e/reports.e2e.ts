import { expect, test, type Page, type APIRequestContext } from '@playwright/test';

import type { TourDetail, TourRevision, RouteReportInput } from '#lib/visitor.ts';
import type { Schemas } from '#lib/api/index.ts';

const apiURL = process.env.E2E_API_URL;
test.skip(!apiURL, 'E2E_API_URL not set');
// A deterministic basemap keeps route/marker rendering independent of tile service availability.
test.beforeEach(async ({ page }) => {
	await page.emulateMedia({ reducedMotion: 'reduce' });
	await page.route('https://tiles.openfreemap.org/styles/**', (route) =>
		route.fulfill({
			json: {
				version: 8,
				sources: {},
				layers: [{ id: 'background', type: 'background', paint: { 'background-color': '#f7f4ef' } }]
			}
		})
	);
});

async function signIn(page: Page, email = `report-${crypto.randomUUID()}@example.com`) {
	await page.goto('/signin');
	await page.getByLabel('Email', { exact: true }).fill(email);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
}
async function retained(request: APIRequestContext) {
	const tour = (await (
		await request.get(`${apiURL}/api/v1/tours/e2e-route-report-fixture`)
	).json()) as TourDetail;
	const revision = (await (
		await request.get(`${apiURL}/api/v1/tours/${tour.id}/revisions/${tour.revision_id}`)
	).json()) as TourRevision;
	expect(revision.snapshot).toBeTruthy();
	return { tour, revision: { ...revision, snapshot: revision.snapshot! } };
}
async function reportPath(request: APIRequestContext) {
	const f = await retained(request);
	return `/tours/${f.tour.id}/report?revision=${f.revision.id}&mode=walk`;
}
async function adminToken(request: APIRequestContext) {
	const r = await request.post(`${apiURL}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	expect(r.ok()).toBeTruthy();
	return (await r.json()).token as string;
}

test('report-only submission keeps the exact seen revision and is keyboard accessible', async ({
	page,
	request
}, info) => {
	await signIn(page);
	const f = await retained(request);
	await page.goto('/tours/e2e-route-report-fixture');
	await page.getByRole('link', { name: 'Suggest a route change', exact: true }).click();
	await expect(
		page.getByRole('heading', { name: 'Suggest a route change', exact: true })
	).toBeVisible();
	const target = page.getByLabel('Affected stop, entrance or path', { exact: true });
	await target.focus();
	await target.press('End');
	await expect(target).toBeFocused();
	expect(await target.inputValue()).toMatch(/^segment:/);
	await page
		.getByLabel('What did you observe?', { exact: true })
		.fill('The selected crossing was blocked. No replacement known.');
	let body!: RouteReportInput;
	page.on('request', (r) => {
		if (r.method() === 'POST' && r.url().endsWith('/api/v1/me/route-reports'))
			body = r.postDataJSON();
	});
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/reports\/[0-9a-f-]+\?submitted=1$/);
	expect(body.tour_id).toBe(f.tour.id);
	expect(body.revision_id).toBe(f.revision.id);
	expect(body.mode).toBe('walk');
	expect(body.affected.kind).toBe('segment');
	expect(body.alternative).toBeNull();
	expect(body.annotation).toBeNull();
	expect(body.observed_on).toBeNull();
	expect(body).not.toHaveProperty('visited');
	expect(body).not.toHaveProperty('trace');
	await expect(
		page.getByText('The selected crossing was blocked. No replacement known.', { exact: true })
	).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Status: Curator review' })).toBeVisible();
	await page.setViewportSize({ width: 320, height: 740 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('report-status-320.png'), fullPage: true });
	await page.goto('/routes/reports');
	await expect(page.getByRole('link', { name: f.tour.title, exact: true })).toHaveCount(1);
});

test('an optional alternative captures entered points without collecting a journey', async ({
	page,
	request
}, info) => {
	await signIn(page);
	const f = await retained(request);
	const leg = f.revision.snapshot.routes.walk.legs![0];
	const from = f.revision.snapshot.tour.stops.find((s) => s.position === leg.from_position)!;
	const to = f.revision.snapshot.tour.stops.find((s) => s.position === leg.to_position)!;
	await page.goto(await reportPath(request));
	await page
		.getByLabel('Affected stop, entrance or path', { exact: true })
		.selectOption(`segment:${from.id}:${to.id}`);
	await page.getByRole('button', { name: 'Draw an alternative', exact: true }).click();
	const start = leg.geometry[0];
	const end = leg.geometry[leg.geometry.length - 1];
	const coords = [
		start,
		[(start[0] + end[0]) / 2 + 0.00015, (start[1] + end[1]) / 2 + 0.00015],
		end
	];
	for (const point of coords) {
		await page.getByLabel('Latitude', { exact: true }).fill(String(point[1]));
		await page.getByLabel('Longitude', { exact: true }).fill(String(point[0]));
		await page.getByRole('button', { name: 'Add coordinate', exact: true }).click();
	}
	await expect(page.getByText(/Alternative point 3:/)).toBeVisible();
	await expect(page.locator('.maplibregl-marker').first()).toBeVisible();
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('report-alternative-mobile.png'), fullPage: true });
	let body!: RouteReportInput;
	page.on('request', (r) => {
		if (r.method() === 'POST' && r.url().endsWith('/api/v1/me/route-reports'))
			body = r.postDataJSON();
	});
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/reports\/[0-9a-f-]+\?submitted=1$/);
	expect(body.alternative).toEqual({ type: 'LineString', coordinates: coords });
	expect(body.annotation).toBeNull();
	expect(body.affected).toEqual({ kind: 'segment', from_stop_id: from.id, to_stop_id: to.id });
});

test('a lost response preserves a report body and retry key across reload', async ({
	page,
	request
}) => {
	await signIn(page);
	await page.goto(await reportPath(request));
	let lost = false;
	const attempts: { key: string | undefined; body: unknown }[] = [];
	await page.route('**/api/v1/me/route-reports', async (route) => {
		if (route.request().method() !== 'POST') return route.continue();
		attempts.push({
			key: route.request().headers()['idempotency-key'],
			body: route.request().postDataJSON()
		});
		if (!lost) {
			lost = true;
			const response = await route.fetch();
			expect(response.status()).toBe(201);
			await route.abort();
		} else await route.continue();
	});
	await page
		.getByLabel('What did you observe?', { exact: true })
		.fill('Only one retained observation after response loss');
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('We could not confirm the report');
	await page.reload();
	await expect(page.getByLabel('What did you observe?', { exact: true })).toHaveValue(
		'Only one retained observation after response loss'
	);
	await expect(page.getByLabel('What did you observe?', { exact: true })).toBeDisabled();
	await page.getByRole('button', { name: 'Retry report', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/reports\/[0-9a-f-]+\?submitted=1$/);
	expect(attempts).toHaveLength(2);
	expect(attempts[1]).toEqual(attempts[0]);
	expect(attempts[0].key).toMatch(/^[0-9a-f-]{36}$/);
	await page.goto('/routes/reports');
	await expect(page.getByRole('listitem')).toHaveCount(1);
});

test('private decisions remain scoped to the report owner', async ({ page, request, browser }) => {
	await signIn(page);
	await page.goto(await reportPath(request));
	await page
		.getByLabel('What did you observe?', { exact: true })
		.fill('Private report with a curator decision');
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/reports\/[0-9a-f-]+\?submitted=1$/);
	const path = new URL(page.url()).pathname;
	const id = path.split('/').at(-1);
	const token = await adminToken(request);
	const decision = await request.post(`${apiURL}/api/v1/admin/route-reports/${id}/state`, {
		headers: { Authorization: `Bearer ${token}` },
		data: {
			version: 1,
			status: 'declined',
			reason: 'Operator evidence does not support this proposed crossing.'
		}
	});
	expect(decision.ok()).toBeTruthy();
	await page.getByRole('button', { name: 'Refresh report status', exact: true }).click();
	await expect(page.getByRole('heading', { name: 'Status: Declined' })).toBeVisible();
	await expect(
		page.getByText('Operator evidence does not support this proposed crossing.', { exact: true })
	).toBeVisible();
	await expect(page.getByRole('link', { name: 'Open trip to review the update' })).toHaveCount(0);
	const other = await browser.newPage();
	try {
		await signIn(other);
		await other.goto(path);
		await expect(
			other.getByText('This report is not available for this account.', { exact: true })
		).toBeVisible();
		await expect(
			other.getByText('Private report with a curator decision', { exact: true })
		).toHaveCount(0);
	} finally {
		await other.close();
	}
});

test('a browser storage failure sends no report and keeps retry available', async ({
	page,
	request
}) => {
	await signIn(page);
	await page.goto(await reportPath(request));
	await page
		.getByLabel('What did you observe?', { exact: true })
		.fill('Kept while browser storage is full');
	let posts = 0;
	page.on('request', (r) => {
		if (r.method() === 'POST' && r.url().endsWith('/api/v1/me/route-reports')) posts++;
	});
	await page.evaluate(() => {
		const original = Storage.prototype.setItem;
		Storage.prototype.setItem = function (key, value) {
			if (key.startsWith('byl.route-report.v1:'))
				throw new DOMException('Fixture quota', 'QuotaExceededError');
			return original.call(this, key, value);
		};
	});
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('No report was sent');
	expect(posts).toBe(0);
	await expect(page.getByRole('button', { name: 'Retry report', exact: true })).toBeEnabled();
	await expect(page.getByLabel('What did you observe?', { exact: true })).toHaveValue(
		'Kept while browser storage is full'
	);
});

test('map picking marks one explicit problem point', async ({ page, request }) => {
	await signIn(page);
	await page.goto(await reportPath(request));
	await expect(page.locator('.maplibregl-marker').first()).toBeVisible();
	await page.getByRole('button', { name: 'Mark problem location', exact: true }).click();
	const canvas = page.locator('canvas.maplibregl-canvas');
	await canvas.scrollIntoViewIfNeeded();
	const marker = await page.locator('.maplibregl-marker').first().boundingBox();
	const box = await canvas.boundingBox();
	expect(marker && box).toBeTruthy();
	// MapLibre overlays controls and markers over the canvas. Click the open map
	// beside the actual retained stop after scrolling, rather than a moving pin.
	await page.mouse.click(Math.max(box!.x + 4, marker!.x - 10), marker!.y + marker!.height / 2);
	await expect(page.getByText(/Marked location:/)).toBeVisible();
	let body!: RouteReportInput;
	page.on('request', (r) => {
		if (r.method() === 'POST' && r.url().endsWith('/api/v1/me/route-reports'))
			body = r.postDataJSON();
	});
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/reports\/[0-9a-f-]+\?submitted=1$/);
	expect(Number.isFinite(body.annotation?.lat)).toBeTruthy();
	expect(Number.isFinite(body.annotation?.lng)).toBeTruthy();
	expect(body.alternative).toBeNull();
});

test('a legacy revision with no retained ready path stays unavailable for reporting', async ({
	page
}) => {
	await signIn(page);
	await page.goto('/tours/downtown-memory-loop/report?mode=walk');
	await expect(
		page.getByText('The selected revision has no retained path for this travel mode.', {
			exact: true
		})
	).toBeVisible();
	await expect(page.getByRole('button', { name: 'Send private report' })).toHaveCount(0);
});

async function curatorFixture(page: Page, request: APIRequestContext) {
	const f = await retained(request);
	const auth = await request.post(`${apiURL}/api/v1/auth/dev`, {
		data: { email: `curator-report-${crypto.randomUUID()}@example.com` }
	});
	const owner = await auth.json();
	const submitted = await request.post(`${apiURL}/api/v1/me/route-reports`, {
		headers: { Authorization: `Bearer ${owner.token}`, 'Idempotency-Key': crypto.randomUUID() },
		data: {
			tour_id: f.tour.id,
			revision_id: f.revision.id,
			mode: 'walk',
			affected: { kind: 'entrance', stop_id: f.revision.snapshot.tour.stops[0].id },
			reason: 'closed',
			details: `Closure review ${crypto.randomUUID()}`
		}
	});
	expect(submitted.status()).toBe(201);
	const report = (await submitted.json()) as Schemas['RouteReport'];
	const currentReport = report; // Intake enters the existing review state directly.
	const investigation: Schemas['RouteInvestigation'] = {
		id: crypto.randomUUID(),
		report_id: report.id,
		job_id: crypto.randomUUID(),
		created_by: currentReport.owner_account_id,
		created_at: '2026-11-08T00:00:00Z',
		input: {
			observations: [{ report: currentReport }],
			brief_id: crypto.randomUUID(),
			evidence_ids: []
		},
		result: {
			status: 'review',
			comparison_direction: 'published',
			before: f.revision.snapshot.routes.walk,
			after: f.revision.snapshot.routes.walk,
			distance_change_m: -20,
			duration_change_s: 30,
			affected_stop_ids: [report.submitted.affected.stop_id!],
			arrivals: [],
			evidence: [
				{
					id: crypto.randomUUID(),
					brief_id: crypto.randomUUID(),
					brief_version: 1,
					job_id: crypto.randomUUID(),
					source_key: 'synthetic',
					url: 'https://example.org/closure',
					reference: 'Synthetic access record',
					source_kind: 'page',
					created_at: '2026-11-08T00:00:00Z',
					result: {
						outcome: 'inspected_page',
						final_url: 'https://example.org/closure',
						publisher: 'Fixture operator',
						title: 'Synthetic retained closure source',
						retrieved_at: '2026-11-08T00:00:00Z',
						http_status: 200,
						content_type: 'text/html',
						content_sha256: 'e'.repeat(64),
						truncated: false,
						untrusted: true,
						media_reuse: 'unknown',
						passages: [
							{ reference: 'p1', text: 'Fixture closure passage <script>untrusted()</script>' }
						]
					}
				}
			],
			issues: [
				{
					code: 'field_review',
					detail: 'Unresolved crossing restriction fixture',
					origin: 'unknown',
					segment_id: 'crossing'
				}
			],
			stories_preserved: true,
			independent_corroboration: false,
			checked_at: '2026-11-08T00:00:00Z'
		}
	};
	const comparison: Schemas['RouteCorrectionComparison'] = {
		version: 'c'.repeat(64),
		report: currentReport,
		investigation,
		before: f.revision,
		prepared: structuredClone(f.revision.snapshot),
		path_reports: {},
		potentially_affected_tours: [f.tour.id],
		blockers: [],
		ready_to_publish: true
	};
	// Ready research/approval transport is synthetic. Real reports and dispositions
	// use the owned API/PostgreSQL; release stale-read/atomicity/race checks live in Go.
	await page.route(`**/api/v1/admin/route-reports/${report.id}/investigations?*`, (route) =>
		route.fulfill({ json: { items: [investigation] } })
	);
	await page.route(`**/api/v1/admin/route-reports/${report.id}/correction-comparison?*`, (route) =>
		route.fulfill({ json: comparison })
	);
	await signIn(page, 'admin@example.com');
	await page.goto(`/admin/queue/reports/${report.id}`);
	await expect(page.getByRole('region', { name: 'Original report' })).toContainText(
		currentReport.submitted.details
	);
	return { ...f, report: currentReport, comparison, investigation, owner };
}

test('curator compares retained paths, escaped sources and impacts before exact approval', async ({
	page,
	request
}, info) => {
	const f = await curatorFixture(page, request);
	await expect(page.getByText('Unresolved crossing restriction fixture')).toBeVisible();
	await page
		.getByText('Synthetic retained closure source · inspected_page', { exact: true })
		.click();
	await expect(
		page.getByText('Fixture closure passage <script>untrusted()</script>', { exact: true })
	).toBeVisible();
	await expect(
		page.getByRole('link', { name: 'Open evidence and story package workspace' })
	).toHaveAttribute('href', `/admin/briefs/${f.investigation.input.brief_id}`);
	const approve = page.getByRole('button', {
		name: 'Approve and publish this correction',
		exact: true
	});
	await expect(approve).toBeDisabled();
	await page
		.getByRole('button', { name: 'Reload current release comparison', exact: true })
		.click();
	await expect(page.getByRole('heading', { name: 'Reported published revision 1' })).toBeVisible();
	await expect(
		page.getByRole('heading', { name: 'Other tours requiring manual assessment' })
	).toBeVisible();
	await page
		.getByRole('region', { name: 'Release comparison' })
		.screenshot({ path: info.outputPath('curator-comparison-desktop.png') });
	await page.setViewportSize({ width: 320, height: 800 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page
		.getByRole('region', { name: 'Release comparison' })
		.screenshot({ path: info.outputPath('curator-comparison-mobile.png') });
	await page.setViewportSize({ width: 1280, height: 900 });
	await page
		.getByLabel('Decision reason (visible to the reporter)', { exact: true })
		.fill('Sources, access and stop impacts reviewed.');
	await page
		.getByLabel(
			'I reviewed the current paths, source/observation evidence, stop/story impacts and restrictions for this release.',
			{ exact: true }
		)
		.check();
	let posted: Schemas['ApproveRouteCorrectionRequest'] | undefined;
	await page.route(`**/api/v1/admin/route-reports/${f.report.id}/corrections`, async (route) => {
		posted = route.request().postDataJSON();
		await route.fulfill({ json: { id: crypto.randomUUID(), tour_id: f.tour.id } });
	});
	await approve.click();
	await expect(page.getByRole('status')).toContainText('Correction published as a new revision.');
	expect(posted).toEqual({
		version: f.comparison.version,
		investigation_id: f.investigation.id,
		note: 'Sources, access and stop impacts reviewed.',
		temporary: false,
		review_on: null
	});
	await expect(approve).toBeDisabled();
	await page.setViewportSize({ width: 320, height: 800 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('curator-report-mobile.png'), fullPage: true });
});

test('a stale correction approval invalidates consent and keeps the curator reason', async ({
	page,
	request
}) => {
	const f = await curatorFixture(page, request);
	await page
		.getByRole('button', { name: 'Reload current release comparison', exact: true })
		.click();
	await page
		.getByLabel('Decision reason (visible to the reporter)', { exact: true })
		.fill('Keep this decision wording after conflict.');
	const consent = page.getByLabel(
		'I reviewed the current paths, source/observation evidence, stop/story impacts and restrictions for this release.',
		{ exact: true }
	);
	await consent.check();
	await page.route(`**/api/v1/admin/route-reports/${f.report.id}/corrections`, (route) =>
		route.fulfill({
			status: 409,
			json: { error: { code: 'conflict', message: 'Changed route input' } }
		})
	);
	const approve = page.getByRole('button', {
		name: 'Approve and publish this correction',
		exact: true
	});
	await approve.click();
	await expect(page.getByRole('alert')).toContainText('This review changed.');
	await expect(consent).not.toBeChecked();
	await expect(approve).toBeDisabled();
	await expect(
		page.getByLabel('Decision reason (visible to the reporter)', { exact: true })
	).toHaveValue('Keep this decision wording after conflict.');
});

test('curators request information and decline with versioned reasons retained by the real API', async ({
	page,
	request
}) => {
	const f = await curatorFixture(page, request);
	const reason = page.getByLabel('Decision reason (visible to the reporter)', { exact: true });
	await reason.fill('Please confirm the closure date from the operator.');
	await page.getByRole('button', { name: 'Request information or research', exact: true }).click();
	await expect(page.getByRole('region', { name: 'Original report' })).toContainText(
		'More information or research needed'
	);
	await reason.fill('Operator source confirms the entrance remains open.');
	await page.getByRole('button', { name: 'Decline report', exact: true }).click();
	await expect(page.getByRole('region', { name: 'Original report' })).toContainText('Declined');
	const ownerRead = await request.get(`${apiURL}/api/v1/me/route-reports/${f.report.id}`, {
		headers: { Authorization: `Bearer ${f.owner.token}` }
	});
	expect(await ownerRead.json()).toMatchObject({
		status: 'declined',
		reason: 'Operator source confirms the entrance remains open.',
		version: f.report.version + 2
	});
});

test('an approved report outcome invites an explicit update without switching a started trip', async ({
	page,
	request
}) => {
	await signIn(page);
	const f = await retained(request);
	await page.goto(`/tours/${f.tour.id}/walk?mode=walk`);
	await expect(
		page.getByText(`Following revision ${f.revision.number}.`, { exact: false })
	).toBeVisible();
	await page.getByRole('link', { name: 'Suggest a route change', exact: true }).click();
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/reports\/[0-9a-f-]+\?submitted=1$/);
	const id = new URL(page.url()).pathname.split('/').at(-1)!;
	const newer = structuredClone(f.revision);
	newer.id = crypto.randomUUID();
	newer.number++;
	newer.snapshot.tour.revision_id = newer.id;
	newer.snapshot.tour.revision_number = newer.number;
	newer.snapshot.tour.stops[0].note = 'Reviewed report correction: use the east entrance.';
	await page.route(`**/api/v1/me/route-reports/${id}/correction`, (route) =>
		route.fulfill({
			json: {
				revision_id: newer.id,
				previous_revision_id: f.revision.id,
				temporary: false,
				review_on: null,
				reassessment_required: false
			}
		})
	);
	await page.route(`**/api/v1/tours/${f.tour.id}/revisions?*`, (route) =>
		route.fulfill({ json: { items: [newer, f.revision] } })
	);
	await page.route(`**/api/v1/tours/${f.tour.id}/revisions/${newer.id}`, (route) =>
		route.fulfill({ json: newer })
	);
	await page.getByRole('button', { name: 'Refresh report status', exact: true }).click();
	await page.getByRole('link', { name: 'Open trip to review the update', exact: true }).click();
	await expect(
		page.getByText(`Following revision ${f.revision.number}.`, { exact: false })
	).toBeVisible();
	await page.getByRole('button', { name: 'Check for route updates', exact: true }).click();
	await expect(
		page.getByText(`Revision ${newer.number} is available. Your current trip has not changed.`)
	).toBeVisible();
	await expect(
		page.getByText(`Following revision ${f.revision.number}.`, { exact: false })
	).toBeVisible();
	await page
		.getByRole('button', {
			name: `Use revision ${newer.number} with its own progress`,
			exact: true
		})
		.click();
	await expect(
		page.getByText(`Following revision ${newer.number}.`, { exact: false })
	).toBeVisible();
});

test('curator arrival adjustments reset access and use separate versioned proposal review', async ({
	page,
	request
}) => {
	const f = await curatorFixture(page, request);
	await page.getByText('Review an entrance or arrival point', { exact: true }).click();
	const latitude = page.getByLabel('Arrival latitude', { exact: true });
	await latitude.fill(String(Number(await latitude.inputValue()) + 0.0001));
	const statuses = page.getByRole('combobox', { name: 'Access status', exact: true });
	await expect(statuses).toHaveCount(2);
	await expect(statuses.nth(0)).toHaveValue('unknown');
	await expect(statuses.nth(1)).toHaveValue('unknown');
	const place = f.revision.snapshot.tour.stops[0].place;
	let proposed: Schemas['ArrivalProposalRequest'] | undefined;
	let reviewed: Schemas['ArrivalReviewRequest'] | undefined;
	const proposalID = crypto.randomUUID();
	await page.route(`**/api/v1/admin/places/${place.id}/arrivals`, async (route) => {
		proposed = route.request().postDataJSON();
		await route.fulfill({
			json: {
				id: proposalID,
				place_id: place.id,
				base_version: proposed!.version,
				point: proposed!.point,
				status: 'pending'
			}
		});
	});
	await page.route(
		`**/api/v1/admin/places/${place.id}/arrivals/${proposalID}/review`,
		async (route) => {
			reviewed = route.request().postDataJSON();
			await route.fulfill({
				json: {
					id: proposalID,
					place_id: place.id,
					base_version: proposed!.version,
					point: proposed!.point,
					status: 'approved'
				}
			});
		}
	);
	await page.getByRole('button', { name: 'Prepare arrival proposal', exact: true }).click();
	await expect(
		page.getByText('Arrival proposal retained privately.', { exact: false })
	).toBeVisible();
	expect(proposed?.version).toBe(place.arrival_point?.version ?? 0);
	await expect(
		page.getByRole('button', { name: 'Reload current release comparison', exact: true })
	).toBeDisabled();
	const approve = page.getByRole('button', {
		name: 'Approve arrival for working data',
		exact: true
	});
	await expect(approve).toBeDisabled();
	await page
		.getByLabel('Private arrival review reason', { exact: true })
		.fill('Separate access review recorded for this synthetic test.');
	await page
		.getByLabel('I reviewed this location, sources and separate walking/cycling access.', {
			exact: true
		})
		.check();
	await approve.click();
	await expect(
		page.getByText('Arrival approved for the working data.', { exact: false })
	).toBeVisible();
	expect(reviewed).toEqual({
		version: proposed!.version,
		action: 'approve',
		note: 'Separate access review recorded for this synthetic test.'
	});
	await expect(
		page.getByRole('button', { name: 'Approve and publish this correction', exact: true })
	).toBeDisabled();
});

test('a lost investigation response replays the retained selection and key after reload', async ({
	page,
	request
}) => {
	await signIn(page);
	await page.goto(await reportPath(request));
	await page
		.getByLabel('What did you observe?', { exact: true })
		.fill('Retained investigation retry fixture');
	await page.getByRole('button', { name: 'Send private report', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/reports\/[0-9a-f-]+\?submitted=1$/);
	const id = new URL(page.url()).pathname.split('/').at(-1)!;
	await signIn(page, 'report-investigator@example.com');
	await page.goto(`/admin/queue/reports/${id}`);
	await expect(page.getByRole('region', { name: 'Original report' })).toContainText(
		'Retained investigation retry fixture'
	);
	const attempts: { key: string | undefined; body: Schemas['RouteInvestigationRequest'] }[] = [];
	await page.route(`**/api/v1/admin/route-reports/${id}/investigations`, async (route) => {
		if (route.request().method() !== 'POST') return route.continue();
		attempts.push({
			key: route.request().headers()['idempotency-key'],
			body: route.request().postDataJSON()
		});
		if (attempts.length === 1) {
			const response = await route.fetch();
			expect(response.status()).toBe(202);
			await route.abort('failed');
		} else await route.continue();
	});
	await page.getByRole('button', { name: 'Queue retained investigation', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('Could not confirm the investigation');
	await page.reload();
	await page.getByRole('button', { name: 'Retry exact investigation', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Investigation queued privately.');
	expect(attempts).toHaveLength(2);
	expect(attempts[0].key).toBeTruthy();
	expect(attempts[1]).toEqual(attempts[0]);
});

test('curator storage failure sends no investigation and retains its in-memory retry identity', async ({
	page,
	request
}) => {
	const f = await curatorFixture(page, request);
	let posts = 0;
	page.on('request', (r) => {
		if (
			r.method() === 'POST' &&
			r.url().endsWith(`/api/v1/admin/route-reports/${f.report.id}/investigations`)
		)
			posts++;
	});
	await page.evaluate(() => {
		const original = Storage.prototype.setItem;
		Storage.prototype.setItem = function (key: string, value: string) {
			if (this === sessionStorage && key.startsWith('byl:report-investigation:')) {
				(window as unknown as { attemptedRetry: string }).attemptedRetry = value;
				throw new DOMException('Fixture quota', 'QuotaExceededError');
			}
			return original.call(this, key, value);
		};
		(window as unknown as { restoreRetryStorage: () => void }).restoreRetryStorage = () => {
			Storage.prototype.setItem = original;
		};
	});
	await page.getByRole('button', { name: 'Queue retained investigation', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('No investigation was sent.');
	expect(posts).toBe(0);
	const retained = await page.evaluate(() => {
		const state = window as unknown as { attemptedRetry: string; restoreRetryStorage: () => void };
		state.restoreRetryStorage();
		return JSON.parse(state.attemptedRetry) as {
			key: string;
			body: Schemas['RouteInvestigationRequest'];
		};
	});
	let posted: { key: string | undefined; body: Schemas['RouteInvestigationRequest'] } | undefined;
	await page.route(`**/api/v1/admin/route-reports/${f.report.id}/investigations`, async (route) => {
		posted = {
			key: route.request().headers()['idempotency-key'],
			body: route.request().postDataJSON()
		};
		await route.fulfill({ status: 202, json: f.investigation });
	});
	await page.getByRole('button', { name: 'Retry exact investigation', exact: true }).click();
	await expect(page.getByRole('status')).toContainText('Investigation queued privately.');
	expect(posted).toEqual(retained);
});
