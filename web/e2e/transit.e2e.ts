import { expect, test, type Page } from '@playwright/test';
import { transitFixture } from '../src/lib/tour/fixtures/transit-fixture';
import type { ScheduledTransitPlan } from '#lib/visitor.ts';
const apiURL = process.env.E2E_API_URL;
test.skip(!apiURL, 'E2E_API_URL not set');
async function setup(page: Page, kind = 'scheduled') {
	const { revision } = transitFixture(Date.now());
	const tour = revision.snapshot!.tour;
	let requests = 0;
	let latest = revision;
	const departures: string[] = [];
	await page.emulateMedia({ reducedMotion: 'reduce' });
	await page.route('https://tiles.openfreemap.org/styles/**', (route) =>
		route.fulfill({ json: { version: 8, sources: {}, layers: [] } })
	);
	await page.route('**/api/v1/itinerary-capabilities', (route) =>
		route.fulfill({ json: { scheduled_transit_v1: kind !== 'disabled' } })
	);
	await page.route(`**/api/v1/tours/${tour.slug}`, (route) => {
		expect(route.request().headers()['x-byl-capabilities']).toBe('scheduled-transit-v1');
		return kind === 'disabled'
			? route.fulfill({
					status: 403,
					json: {
						error: { code: 'unsupported_itinerary', message: 'Scheduled transit is disabled' }
					}
				})
			: route.fulfill({ json: tour });
	});
	await page.route(`**/api/v1/tours/${tour.id}/revisions?**`, (route) =>
		route.fulfill({ json: { items: [{ ...latest, snapshot: undefined }] } })
	);
	await page.route(`**/api/v1/tours/${tour.id}/revisions/*`, (route) => {
		expect(route.request().headers()['x-byl-capabilities']).toBe('scheduled-transit-v1');
		return route.fulfill({ json: route.request().url().endsWith(revision.id) ? revision : latest });
	});
	await page.route(`**/api/v1/tours/${tour.id}/revisions/*/transit-plan`, (route) => {
		requests++;
		expect(route.request().headers()['x-byl-capabilities']).toBe('scheduled-transit-v1');
		const body = route.request().postDataJSON();
		expect(Object.keys(body).sort()).toEqual(['departure_at', 'version']);
		expect(body.version).toBe(1);
		departures.push(body.departure_at);
		let plan: ScheduledTransitPlan = transitFixture(Date.now(), Date.parse(body.departure_at)).plan;
		if (route.request().url().includes(latest.id)) {
			plan.revision_id = latest.id;
			plan.connector_version = latest.snapshot!.tour.transit!.version;
		}
		if (kind === 'no_service')
			plan = {
				...plan,
				state: 'no_service',
				service: { state: 'no_service', scheduled: true, live: false, issues: ['no_service'] },
				segments: []
			};
		if (kind === 'stale')
			plan.service.source!.retrieved_at = new Date(Date.now() - 25 * 3600000).toISOString();
		if (kind === 'wrong_revision') plan.revision_id = 'wrong';
		return route.fulfill({ json: plan });
	});
	return {
		tour,
		revision,
		requests: () => requests,
		departures,
		update: () => {
			latest = structuredClone(revision);
			latest.id = '40000000-0000-4000-8000-000000008888';
			latest.number = revision.number + 1;
			latest.snapshot!.tour.revision_id = latest.id;
			latest.snapshot!.tour.revision_number = latest.number;
			Object.assign(latest.snapshot!.tour, {
				transit: { ...latest.snapshot!.tour.transit!, version: 2 }
			});
		}
	};
}
async function showTransit(page: Page) {
	const toggle = page.locator('summary').filter({ hasText: 'CTA connection ·' });
	if (new URL(page.url()).pathname.endsWith('/walk')) await expect(toggle).toBeVisible();
	if (await toggle.count()) {
		const open = await toggle.locator('..').getAttribute('open');
		if (open === null) await toggle.click();
	}
}
async function choose(page: Page) {
	await showTransit(page);
	await page.getByRole('button', { name: 'Request a timetable to review' }).click();
	await expect(page.getByLabel('Timetable proposal')).toBeVisible();
	await page.getByRole('button', { name: 'Use this scheduled plan' }).click();
	await expect(page.getByLabel('Retained scheduled plan')).toBeVisible();
}

test('retains a chosen timetable across reload and progress, with explicit walking handoff', async ({
	page
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	const f = await setup(page);
	await page.goto(`/tours/${f.tour.slug}`);
	await expect(
		page.getByRole('heading', { name: 'Walk + scheduled CTA connection' })
	).toBeVisible();
	await expect(page.getByText(/CTA times are scheduled, not live/)).toBeVisible();
	await choose(page);
	expect(f.requests()).toBe(1);
	await page.getByRole('link', { name: 'Open mixed trip' }).click();
	await expect(page).toHaveURL(/\/walk\?mode=walk$/);
	await showTransit(page);
	await expect(page.getByLabel('Retained scheduled plan')).toBeVisible();
	expect(f.requests()).toBe(1);
	await page.reload();
	await expect(page.locator('summary').filter({ hasText: 'CTA connection ·' })).toBeVisible();
	await showTransit(page);
	await expect(page.getByLabel('Retained scheduled plan')).toBeVisible();
	expect(f.requests()).toBe(1);
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	await page.getByRole('button', { name: /Continue to the next stop/ }).click();
	await expect(page.getByRole('link', { name: 'Directions', exact: true })).toHaveCount(0);
	const opener = page.getByRole('button', {
		name: 'Walking directions to boarding platform',
		exact: true
	});
	await opener.click();
	const dialog = page.getByRole('dialog', { name: 'Hand off walking directions?' });
	await expect(dialog.getByText('That app chooses its own route', { exact: false })).toBeVisible();
	const external = new URL(
		(await dialog
			.getByRole('link', { name: 'Open external walking directions ↗' })
			.getAttribute('href'))!
	);
	expect(external.searchParams.get('destination')).toBe('41.885737,-87.630886');
	expect(external.searchParams.get('travelmode')).toBe('walking');
	await page.keyboard.press('Escape');
	await expect(dialog).toHaveCount(0);
	await expect(opener).toBeFocused();
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.locator('summary').filter({ hasText: 'CTA connection ·' }).click();
	await page.screenshot({ path: test.info().outputPath('mixed-trip-mobile.png'), fullPage: true });
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	await page.getByRole('button', { name: /Continue to the next stop/ }).click();
	await expect(page.getByRole('link', { name: 'Directions', exact: true })).toBeVisible();
	expect(f.requests()).toBe(1);
});

for (const kind of ['no_service', 'stale', 'wrong_revision'])
	test(`${kind} stays explicit and cannot replace the selected trip`, async ({ page }) => {
		const f = await setup(page, kind);
		await page.goto(`/tours/${f.tour.slug}`);
		await page.getByRole('button', { name: 'Request a timetable to review' }).click();
		if (kind === 'wrong_revision')
			await expect(page.getByText(/timetable does not match/)).toBeVisible();
		else {
			await expect(page.getByLabel('Timetable proposal')).toBeVisible();
			await expect(
				page.getByText(
					kind === 'stale' ? /Stored timetable is stale/ : /No scheduled service found/
				)
			).toBeVisible();
		}
		await expect(page.getByLabel('Retained scheduled plan')).toHaveCount(0);
		if (kind === 'stale')
			await expect(page.getByRole('button', { name: 'Use this scheduled plan' })).toBeDisabled();
		else await expect(page.getByRole('button', { name: 'Use this scheduled plan' })).toHaveCount(0);
	});

test('disabled capability shows authoritative unavailability instead of an old cached walk', async ({
	page
}) => {
	const f = await setup(page, 'disabled');
	await page.addInitScript(
		({ tour }) => localStorage.setItem(`invisible-museum.tour.${tour.slug}`, JSON.stringify(tour)),
		{ tour: f.tour }
	);
	await page.goto(`/tours/${f.tour.slug}`);
	await expect(page.getByText('Scheduled transit is disabled', { exact: true })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Open mixed trip' })).toHaveCount(0);
	await page.goto(`/tours/${f.tour.slug}/walk`);
	await expect(page.getByRole('button', { name: "I'm here", exact: true })).toHaveCount(0);
});

test('an explicit revision update clears the old revision plan without choosing another train', async ({
	page
}) => {
	const f = await setup(page);
	await page.goto(`/tours/${f.tour.slug}/walk`);
	await choose(page);
	f.update();
	await page.getByRole('button', { name: 'Check for route updates' }).click();
	const adopt = page.getByRole('button', {
		name: new RegExp(`Use revision ${f.revision.number + 1} with its own progress`)
	});
	await expect(adopt).toBeVisible();
	await expect(page.getByLabel('Retained scheduled plan')).toBeVisible();
	await adopt.click();
	await expect(page.getByText(/No timetable has been retained for this revision/)).toBeVisible();
	await expect(page.getByLabel('Retained scheduled plan')).toHaveCount(0);
	expect(f.requests()).toBe(1);
	await choose(page);
	expect(f.requests()).toBe(2);
	await expect(page.getByText('0 of 3 stops', { exact: true })).toBeVisible();
});

test('failed private storage leaves the timetable proposal unaccepted', async ({ page }) => {
	const f = await setup(page);
	await page.addInitScript(() => {
		const write = Storage.prototype.setItem;
		Storage.prototype.setItem = function (key, value) {
			if (key.startsWith('byl.transit-plan.v1.')) throw Error('Synthetic quota failure');
			write.call(this, key, value);
		};
	});
	await page.goto(`/tours/${f.tour.slug}`);
	await page.getByRole('button', { name: 'Request a timetable to review' }).click();
	await page.getByRole('button', { name: 'Use this scheduled plan' }).click();
	await expect(page.getByText(/timetable could not be saved on this device/)).toBeVisible();
	await expect(page.getByLabel('Retained scheduled plan')).toHaveCount(0);
});
