import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function fixture(page: Page, request: APIRequestContext) {
	const auth = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	const session = await auth.json();
	const created = await request.post(`${apiUrl}/api/v1/admin/route-briefs`, {
		headers: { Authorization: `Bearer ${session.token}` },
		data: { topic: 'Package review browser fixture', area: 'Downers Grove' }
	});
	expect(created.status()).toBe(201);
	const brief = await created.json();
	const tour = await (await request.get(`${apiUrl}/api/v1/tours/downtown-memory-loop`)).json();
	// The ready review response is synthetic transport data. Publication atomicity,
	// proof checks and concurrency are independently tested against PostgreSQL.
	const report = {
		schema_version: 1,
		version: 'a'.repeat(64),
		state: 'ready_for_review',
		ready_to_publish: true,
		brief,
		research: { id: '00000000-0000-4000-8000-000000000011', status: 'completed' },
		submitted: brief.submitted,
		proposal: null,
		tour,
		reconciliation: [],
		path_reports: {},
		routes: {
			walk: {
				geometry: {
					type: 'LineString',
					coordinates: tour.stops.map((s: { place: { longitude: number; latitude: number } }) => [
						s.place.longitude,
						s.place.latitude
					])
				},
				quality: { input_current: true },
				journey: { state: 'ready', usable: true, travel_duration_s: 900 }
			}
		},
		visitor_stories: [
			{
				position: 1,
				place_id: tour.stops[0].place.id,
				moment_id: tour.stops[0].featured_moment.id,
				candidate_id: '00000000-0000-4000-8000-000000000012',
				title: 'Railroad history fixture',
				body: 'Visitor narrative shown beside retained evidence.',
				text_source: 'evidence_linked_draft',
				source_urls: ['https://example.org/history'],
				evidence_ids: ['proof-one']
			}
		],
		evidence: [
			{
				id: 'proof-one',
				url: 'https://example.org/proof',
				result: {
					title: 'Inspected archive fixture',
					publisher: 'Fixture archive',
					retrieved_at: '2026-11-07T00:00:00Z',
					outcome: 'inspected_page',
					passages: [
						{ reference: 'p1', text: 'Retained evidence passage <script>untrusted()</script>' }
					]
				}
			}
		],
		entities: [
			{
				key: `moment:${tour.stops[0].featured_moment.id}`,
				kind: 'moment',
				id: tour.stops[0].featured_moment.id,
				required: true,
				disposition: 'reused_draft',
				current: {
					title: 'Prepared historical account',
					hero_media: tour.stops[0].featured_moment.hero_media
				},
				candidate: null,
				proposed: null,
				changes: []
			},
			{
				key: 'candidate:fixture',
				kind: 'moment',
				id: '00000000-0000-4000-8000-000000000012',
				required: false,
				disposition: 'proposed_shared_edit',
				current: { title: 'Shared history' },
				candidate: { id: '00000000-0000-4000-8000-000000000012', status: 'pending' },
				proposed: {
					title: 'Proposed shared title',
					uncertainties: [{ text: 'The precise opening date is uncertain.' }]
				},
				changes: [{ field: 'title', before: 'Shared history', after: 'Proposed shared title' }]
			}
		],
		tasks: [] as { code: string; entity_key: string; detail: string }[]
	};
	await page.route(`**/api/v1/admin/route-briefs/${brief.id}/review-package?*`, (route) =>
		route.fulfill({ json: report })
	);
	await page.goto('/signin');
	await page.evaluate(
		(value) => localStorage.setItem('invisible-museum.session', JSON.stringify(value)),
		session
	);
	await page.goto(`/admin/briefs/${brief.id}`);
	await page.getByLabel('Prepared tour', { exact: true }).selectOption(tour.id);
	await page.getByRole('button', { name: 'Reload review package' }).focus();
	await page.keyboard.press('Enter');
	await expect(page.getByRole('heading', { name: 'Visitor preview: ' + tour.title })).toBeVisible();
	expect(
		await page
			.getByRole('button', { name: 'Reload review package' })
			.evaluate((el) => getComputedStyle(el).outlineWidth)
	).toBe('3px');
	return { report, brief, tour };
}

test('curator compares sources, uncertainty and affected records before exact package approval', async ({
	page,
	request
}, info) => {
	const { brief, tour, report } = await fixture(page, request);
	await expect(page.getByText('Visitor narrative shown beside retained evidence.')).toBeVisible();
	await expect(page.getByText('The precise opening date is uncertain.')).toBeVisible();
	await expect(
		page.getByRole('img', { name: tour.stops[0].featured_moment.hero_media.caption })
	).toBeVisible();
	await page
		.getByRole('region', { name: 'Curator package review' })
		.screenshot({ path: info.outputPath('package-desktop.png') });
	await expect(
		page.getByText('Retained evidence passage <script>untrusted()</script>')
	).toBeVisible();
	await page.getByText('Affected records and proposed changes', { exact: true }).click();
	await expect(page.getByText('Proposed shared edit; remains unapplied')).toBeVisible();
	await page.getByText('Proposed title change', { exact: true }).click();
	await expect(page.getByText('"Proposed shared title"', { exact: false })).toBeVisible();
	const approve = page.getByRole('button', { name: 'Approve and publish reviewed package' });
	await expect(approve).toBeDisabled();
	await page.getByLabel('Submitted notes').fill('Keep my unsaved editorial wording.');
	await page
		.getByLabel('Decision note (visible to the brief owner)')
		.fill('Exact review approved.');
	let posted: unknown;
	await page.route(
		`**/api/v1/admin/route-briefs/${brief.id}/review-package/publish`,
		async (route) => {
			posted = route.request().postDataJSON();
			await route.fulfill({ json: { id: 'revision-fixture', tour_id: tour.id } });
		}
	);
	await page
		.getByLabel(
			'I reviewed the stories, evidence, route quality and selected dependencies in this package.'
		)
		.check();
	await approve.click();
	await expect(
		page.getByText(
			'Package published. The reviewed tour revision and selected dependencies were released together.'
		)
	).toBeVisible();
	expect(posted).toEqual({
		version: report.version,
		run_id: report.research.id,
		tour_id: tour.id,
		note: 'Exact review approved.'
	});
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Keep my unsaved editorial wording.'
	);
	await expect(approve).toBeDisabled();
	await page.setViewportSize({ width: 320, height: 800 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('package-mobile.png'), fullPage: true });
});

test('blocking issues and stale approval keep publication disabled and retain curator wording', async ({
	page,
	request
}) => {
	const { brief, report } = await fixture(page, request);
	const approve = page.getByRole('button', { name: 'Approve and publish reviewed package' });
	await page.getByLabel('Submitted notes').fill('Retain these unsaved notes.');
	await page.getByLabel('Decision note (visible to the brief owner)').fill('Check the crossing.');
	await page.route(`**/api/v1/admin/route-briefs/${brief.id}/review-package/publish`, (route) =>
		route.fulfill({
			status: 409,
			json: { error: { code: 'conflict', message: 'Package changed' } }
		})
	);
	await page
		.getByLabel(
			'I reviewed the stories, evidence, route quality and selected dependencies in this package.'
		)
		.check();
	await approve.click();
	await expect(page.getByRole('alert')).toContainText('This package changed.');
	await expect(approve).toBeDisabled();
	report.ready_to_publish = false;
	report.tasks = [
		{ code: 'route_not_usable', entity_key: 'walk', detail: 'The crossing is unresolved.' }
	];
	await page.getByRole('button', { name: 'Reload review package' }).click();
	await expect(page.getByRole('region', { name: 'Blocking review tasks' })).toContainText(
		'The crossing is unresolved.'
	);
	await expect(approve).toBeDisabled();
	await expect(page.getByLabel('Submitted notes')).toHaveValue('Retain these unsaved notes.');
	await expect(page.getByLabel('Decision note (visible to the brief owner)')).toHaveValue(
		'Check the crossing.'
	);
});

test('targeted research keeps unsaved edits and passes only explicitly selected proof', async ({
	page,
	request
}) => {
	const { brief, report } = await fixture(page, request);
	await page.getByLabel('Submitted notes').fill('Unsaved wording survives targeted research.');
	const research = page.getByRole('button', { name: 'Request targeted history research' });
	await expect(research).toBeDisabled();
	let posted: unknown;
	await page.route(
		`**/api/v1/admin/candidates/${report.visitor_stories[0].candidate_id}/draft`,
		async (route) => {
			posted = route.request().postDataJSON();
			await route.fulfill({ status: 202, json: { job: { id: 'synthetic-job' } } });
		}
	);
	await page.getByLabel('Use this inspected source for targeted research').check();
	await research.click();
	await expect(
		page.getByText('Targeted history research requested.', { exact: false })
	).toBeVisible();
	expect(posted).toEqual({ evidence_ids: ['proof-one'] });
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Unsaved wording survives targeted research.'
	);
	await expect(
		page.getByRole('button', { name: 'Approve and publish reviewed package' })
	).toBeDisabled();
	await page.getByRole('button', { name: 'Reload review package' }).click();
	await expect(
		page.getByLabel('Use this inspected source for targeted research')
	).not.toBeChecked();
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Unsaved wording survives targeted research.'
	);
	await page
		.getByLabel('Decision note (visible to the brief owner)')
		.fill('Needs further archive research.');
	// This decision uses the actual isolated API, including its expected version.
	await page.getByRole('button', { name: 'Return for research', exact: true }).click();
	await expect(
		page.getByText('Brief returned for research. No content was published.')
	).toBeVisible();
	await expect(page.getByText('State: needs research', { exact: true })).toBeVisible();
	await expect(page.getByLabel('Submitted notes')).toHaveValue(
		'Unsaved wording survives targeted research.'
	);
	expect(brief.version).toBe(1);
});

test('a rejected package stays private and a visitor cannot open package review', async ({
	page,
	request
}) => {
	const { brief } = await fixture(page, request);
	await page.getByRole('button', { name: 'Decline brief', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('Add a decision note');
	await page
		.getByLabel('Decision note (visible to the brief owner)')
		.fill('Unsupported historical claim.');
	await page.getByRole('button', { name: 'Decline brief', exact: true }).click();
	await expect(page.getByText('Brief declined. No content was published.')).toBeVisible();
	await page.getByRole('button', { name: 'Sign out', exact: true }).click();
	await page.getByLabel('Email').fill(`package-visitor-${Date.now()}@example.com`);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
	await page.goto(`/admin/briefs/${brief.id}`);
	await expect(page.getByRole('heading', { name: /needs the curator role/i })).toBeVisible();
	await expect(page.getByRole('button', { name: 'Reload review package' })).toHaveCount(0);
});
