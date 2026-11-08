import { expect, test, type Page } from '@playwright/test';
import type { Schemas } from '#lib/api/index.ts';
const apiURL = process.env.E2E_API_URL;
test.skip(!apiURL, 'E2E_API_URL not set');
async function signIn(page: Page) {
	await page.goto('/signin');
	await page.getByLabel('Email', { exact: true }).fill('admin@example.com');
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
}
function sample(
	tour: Schemas['TourDetail'],
	revision: Schemas['TourRevision'],
	report: string
): Schemas['ContributionReview'] {
	const now = new Date();
	const monday = new Date(
		Date.UTC(
			now.getUTCFullYear(),
			now.getUTCMonth(),
			now.getUTCDate() - ((now.getUTCDay() + 6) % 7)
		)
	);
	const end = new Date(monday.getTime() + 7 * 86400000);
	return {
		tour_id: tour.id,
		tour_title: tour.title,
		revision_id: revision.id,
		revision_number: revision.number,
		segment_id: 'synthetic-section',
		segment_kind: 'trail',
		mode: 'walk',
		direction: 'forward',
		window_start: monday.toISOString().slice(0, 10),
		window_end: end.toISOString().slice(0, 10),
		observed_through: now.toISOString().slice(0, 10),
		qualified_passages: 5,
		departure_rejoins: 3,
		contributor_scopes: 5,
		departure_scopes: 3,
		flagged: true,
		thresholds: {
			version: 'uncalibrated-review-v1',
			minimum_scopes: 5,
			minimum_departure_scopes: 3,
			minimum_departure_percent: 30
		},
		geometry: {
			type: 'LineString',
			coordinates: tour.stops.map((s) => [s.place.longitude, s.place.latitude])
		},
		reports: [
			{
				id: report,
				status: 'submitted',
				reason: 'blocked',
				affected: { kind: 'segment', from_stop_id: tour.stops[0].id, to_stop_id: tour.stops[1].id }
			}
		],
		reports_truncated: false
	};
}

test('curator inspects sample limits and enters the real report correction workflow', async ({
	page,
	request
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	const tokenReply = await request.post(`${apiURL}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	const { token } = await tokenReply.json();
	const tour = (await (
		await request.get(`${apiURL}/api/v1/tours/e2e-route-report-fixture`)
	).json()) as Schemas['TourDetail'];
	const revision = (await (
		await request.get(`${apiURL}/api/v1/tours/${tour.id}/revisions/${tour.revision_id}`)
	).json()) as Schemas['TourRevision'];
	const created = await request.post(`${apiURL}/api/v1/me/route-reports`, {
		headers: { Authorization: `Bearer ${token}` },
		data: {
			tour_id: tour.id,
			revision_id: revision.id,
			mode: 'walk',
			reason: 'blocked',
			details: 'Synthetic aggregation UI fixture; not a field observation.',
			affected: { kind: 'segment', from_stop_id: tour.stops[0].id, to_stop_id: tour.stops[1].id }
		}
	});
	expect(created.ok()).toBeTruthy();
	const report = await created.json();
	let item = sample(tour, revision, report.id);
	// Counts are a browser contract fixture. Real PostgreSQL/HTTP aggregation is
	// tested independently; report creation and correction UI remain real here.
	await page.route('**/api/v1/admin/route-contribution-reviews?**', (route) => {
		const query = new URL(route.request().url()).searchParams;
		if (query.has('revision_id'))
			expect([...query.keys()].sort()).toEqual([
				'direction',
				'mode',
				'revision_id',
				'segment_id',
				'window_start'
			]);
		return route.fulfill({ json: { items: [item] }, headers: { 'Cache-Control': 'no-store' } });
	});
	await page.route('https://tiles.openfreemap.org/styles/**', (route) =>
		route.fulfill({ json: { version: 8, sources: {}, layers: [] } })
	);
	await signIn(page);
	await page.goto('/admin/queue');
	await page.getByRole('link', { name: 'Review route contribution patterns' }).click();
	await expect(
		page.getByText('3 departure/rejoins / 5 eligible observed passages · 60.0%')
	).toBeVisible();
	const limits = page.getByText('How samples and thresholds are counted', { exact: true });
	await limits.focus();
	await limits.press('Enter');
	await expect(page.getByText(/Scopes are not verified people/)).toBeVisible();
	await limits.press('Enter');
	await page.getByRole('link', { name: 'Inspect this section and related reports' }).click();
	await expect(
		page.getByText('Their relationship to this section is unverified.', { exact: false })
	).toBeVisible();
	await expect(
		page.getByRole('link', { name: 'Open working tour and normal review controls' })
	).toHaveAttribute('href', `/admin/tours/${tour.id}`);
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.getByRole('region', { name: 'Map', exact: true }).scrollIntoViewIfNeeded();
	await page.screenshot({ path: test.info().outputPath('review-mobile.png'), fullPage: true });
	item = { ...item, flagged: false, qualified_passages: 4, contributor_scopes: 4 };
	await page.getByRole('button', { name: 'Reload live counts' }).click();
	await expect(
		page.getByRole('status').filter({ hasText: 'Thresholds no longer met' })
	).toBeVisible();
	await expect(
		page.getByText('3 departure/rejoins / 4 eligible observed passages · 75.0%')
	).toBeVisible();
	await page.getByRole('link', { name: /Review blocked report/ }).click();
	await expect(page).toHaveURL(new RegExp(`/admin/queue/reports/${report.id}$`));
	await expect(
		page.getByText(/Observations and investigation paths are unverified leads/)
	).toBeVisible();
	await expect(page.getByRole('button', { name: 'Queue retained investigation' })).toBeVisible();
});

test('empty and failed live review reads show explicit states', async ({ page }) => {
	await signIn(page);
	await page.route('**/api/v1/admin/route-contribution-reviews?**', (route) =>
		route.fulfill({ json: { items: [] } })
	);
	await page.goto('/admin/queue/contributions');
	await expect(page.getByText(/No current tasks meet the sample/)).toBeVisible();
	await page.route('**/api/v1/admin/route-contribution-reviews?**', (route) =>
		route.fulfill({
			status: 503,
			json: { error: { code: 'unavailable', message: 'Synthetic unavailable' } }
		})
	);
	await page.getByRole('button', { name: 'Reload live counts' }).click();
	await expect(page.getByRole('alert')).toBeVisible();
	await expect(page.getByText(/No current tasks meet the sample/)).toHaveCount(0);
});
