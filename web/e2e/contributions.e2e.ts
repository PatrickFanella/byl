import { expect, test, type Page, type APIRequestContext } from '@playwright/test';
import type { TourDetail, TourRevision } from '#lib/visitor.ts';
import fixtures from '../src/lib/tour/fixtures/deviation-v1.json' with { type: 'json' };

const apiURL = process.env.E2E_API_URL;
test.skip(!apiURL, 'E2E_API_URL not set');
declare global {
	interface Window {
		bylFixtureNow: number;
		bylFixtureFix?: (fix: GeolocationPosition) => void;
	}
}
const point = (x: number, y: number) => ({
	lat: fixtures.origin.lat + ((y / 6371000) * 180) / Math.PI,
	lng:
		fixtures.origin.lng +
		((x / (6371000 * Math.cos((fixtures.origin.lat * Math.PI) / 180))) * 180) / Math.PI
});

async function setup(page: Page, request: APIRequestContext, enabled = true, offline = false) {
	page.on(
		'pageerror',
		(e) => void test.info().attach('page-error', { body: e.message, contentType: 'text/plain' })
	);
	page.on('console', (message) => {
		if (message.type() === 'error')
			void test.info().attach('browser-error', { body: message.text(), contentType: 'text/plain' });
	});
	const epoch = Date.parse(new Date().toISOString().slice(0, 10) + 'T12:00:00Z');
	await page.addInitScript(
		({ epoch }) => {
			window.bylFixtureNow = epoch;
			Date.now = () => window.bylFixtureNow;
			Object.defineProperty(navigator, 'geolocation', {
				value: {
					watchPosition: (callback: (p: GeolocationPosition) => void) => {
						window.bylFixtureFix = callback;
						return 1;
					},
					getCurrentPosition: () => {},
					clearWatch: () => {
						window.bylFixtureFix = undefined;
					}
				}
			});
		},
		{ epoch }
	);
	await page.route('https://tiles.openfreemap.org/styles/**', (route) =>
		route.fulfill({ json: { version: 8, sources: {}, layers: [] } })
	);
	await page.goto('/signin');
	await page
		.getByLabel('Email', { exact: true })
		.fill(`contribution-${crypto.randomUUID()}@example.com`);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
	const tour = (await (
		await request.get(`${apiURL}/api/v1/tours/e2e-route-report-fixture`)
	).json()) as TourDetail;
	const revision = (await (
		await request.get(`${apiURL}/api/v1/tours/${tour.id}/revisions/${tour.revision_id}`)
	).json()) as TourRevision;
	const snapshot = revision.snapshot!;
	const source = {
		url: 'https://example.org/synthetic-observation',
		title: 'Synthetic browser observation reference',
		publisher: 'Test fixture',
		version: 'synthetic-v1',
		retrieved_on: '2026-11-07',
		license: 'CC0',
		license_url: 'https://creativecommons.org/publicdomain/zero/1.0/',
		note: 'Not a field route or real-world access assessment.'
	};
	snapshot.tour.corridor = {
		id: 'synthetic-corridor',
		version: 1,
		reviewed_at: new Date(epoch).toISOString(),
		content_sha256: 'a'.repeat(64),
		intent: {
			key: 'synthetic',
			name: 'Synthetic observation reference',
			source,
			routes: [
				{
					mode: 'walk',
					branch: 'fixture',
					direction: 'forward',
					path: [{ segment_id: 'synthetic-segment', reverse: false }]
				}
			],
			segments: [
				{
					id: 'synthetic-segment',
					kind: 'trail',
					from_node: 'a',
					to_node: 'b',
					branches: ['fixture'],
					modes: ['walk'],
					source,
					geometry: {
						type: 'LineString',
						coordinates: [
							[0, 0],
							[400, 0]
						].map(([x, y]) => {
							const p = point(x, y);
							return [p.lng, p.lat];
						})
					},
					signal_references: {
						walk: {
							version: 1,
							ambiguity_reviewed: true,
							source,
							competing_paths: [],
							stop_areas: []
						}
					}
				}
			]
		}
	};
	Object.assign(snapshot.routes.walk.journey!, {
		usable: true,
		segments: [
			{
				id: 'synthetic-segment',
				kind: 'trail',
				reverse: false,
				basis: 'approved_reference',
				modes: ['walk'],
				surface: null,
				access: 'allowed',
				source
			}
		]
	});
	// Synthetic capability/consent transports isolate browser lifecycle and privacy.
	// Server custody, payload rejection, deletion and frozen review have real PostgreSQL tests.
	let gates = 0;
	await page.route('**/api/v1/route-contribution-intake', (route) => {
		gates++;
		return route.fulfill({
			json: { enabled, detector_version: 'uncalibrated-v1', calibrated: false }
		});
	});
	await page.route(`**/api/v1/tours/${tour.id}/revisions/${revision.id}`, (route) =>
		route.fulfill({ json: revision })
	);
	const grants: { body: unknown; key: string | undefined }[] = [];
	const summaries: {
		body: { consent_id: string; summary: Record<string, unknown> };
		key: string | undefined;
	}[] = [];
	const withdrawals: unknown[] = [];
	const withdrawalAuthorizations: (string | undefined)[] = [];
	const consent = {
		id: '00000000-0000-4000-8000-000000000001',
		deletion_handle: 'synthetic-deletion-capability',
		expires_on: new Date(epoch + 30 * 86400000).toISOString().slice(0, 10)
	};
	await page.route('**/api/v1/me/route-contributions/consent', (route) => {
		grants.push({
			body: route.request().postDataJSON(),
			key: route.request().headers()['idempotency-key']
		});
		return route.fulfill({ status: 201, json: consent });
	});
	await page.route('**/api/v1/me/route-contributions', (route) => {
		summaries.push({
			body: route.request().postDataJSON(),
			key: route.request().headers()['idempotency-key']
		});
		return offline
			? route.abort('internetdisconnected')
			: route.fulfill({ status: 201, json: { id: 'summary-1', expires_on: consent.expires_on } });
	});
	await page.route('**/api/v1/route-contributions/withdraw', (route) => {
		withdrawals.push(route.request().postDataJSON());
		withdrawalAuthorizations.push(route.request().headers().authorization);
		return route.fulfill({ status: 204 });
	});
	await page.goto(`/tours/${tour.id}/walk?mode=walk`);
	await expect(page.getByText('Next · stop 1 of 4', { exact: true })).toBeVisible();
	await page.getByText('Optional route improvement', { exact: true }).click();
	await expect.poll(() => gates).toBeGreaterThan(0);
	return {
		epoch,
		grants,
		summaries,
		withdrawals,
		withdrawalAuthorizations,
		consent,
		gates: () => gates
	};
}

async function passage(page: Page, f: Awaited<ReturnType<typeof setup>>) {
	for (const [i, row] of fixtures.cases[0].samples.entries()) {
		if (!Array.isArray(row)) continue;
		const [t, x, y, accuracy] = row;
		await page.evaluate(
			(now) => {
				window.bylFixtureNow = now;
			},
			f.epoch + t * 1000
		);
		if (i > 0 && i % 4 === 0) {
			// Do not advance 40 seconds of fixture time while this real request is
			// still in flight: that would correctly expire foreground admission.
			const response = page.waitForResponse((r) =>
				r.url().endsWith('/api/v1/route-contribution-intake')
			);
			const before = f.gates();
			await page.getByRole('button', { name: 'Retry contribution work' }).click();
			await (await response).finished();
			await page.evaluate(() => Promise.resolve());
			await expect.poll(f.gates).toBeGreaterThan(before);
		}
		await page.evaluate(
			({ p, accuracy }) => {
				window.bylFixtureFix?.({
					timestamp: Date.now(),
					coords: {
						latitude: p.lat,
						longitude: p.lng,
						accuracy,
						altitude: null,
						altitudeAccuracy: null,
						heading: null,
						speed: null
					}
				} as GeolocationPosition);
			},
			{ p: point(x, y), accuracy }
		);
	}
}

test('disabled collection and absent consent leave navigation unchanged', async ({
	page,
	request
}) => {
	const f = await setup(page, request, false);
	await passage(page, f);
	await expect(
		page.getByRole('button', { name: 'Contribute day-level summaries for this revision and mode' })
	).toBeDisabled();
	expect(f.grants).toEqual([]);
	expect(f.summaries).toEqual([]);
	await page.getByRole('button', { name: "I'm here", exact: true }).click();
	await expect(page.getByText(/you have arrived/i)).toBeVisible();
});

test('explicit opt-in queues only minimal counts after network failure, then withdrawal clears and deletes', async ({
	page,
	request
}, info) => {
	const f = await setup(page, request, true, true);
	await passage(page, f);
	expect(f.grants).toEqual([]);
	expect(f.summaries).toEqual([]);
	await page.evaluate((now) => {
		window.bylFixtureNow = now;
	}, f.epoch);
	await page
		.getByRole('button', { name: 'Contribute day-level summaries for this revision and mode' })
		.click();
	await expect(page.getByRole('button', { name: 'Pause route observation' })).toBeVisible();
	await expect
		.poll(() =>
			page.evaluate(() => {
				const raw = Object.entries(localStorage).find(([k]) =>
					k.startsWith('byl.contributions.')
				)?.[1];
				return raw ? JSON.parse(raw)[0]?.consent?.id : null;
			})
		)
		.toBe(f.consent.id);
	await passage(page, f);
	await expect.poll(() => f.summaries.length).toBeGreaterThan(0);
	const body = f.summaries[0].body;
	expect(Object.keys(body).sort()).toEqual(['consent_id', 'summary']);
	expect(Object.keys(body.summary)).toHaveLength(10);
	expect(body.summary).toMatchObject({
		calibrated: false,
		qualified_passages: 1,
		sustained_departure_rejoins: 0
	});
	const stored = await page.evaluate(() =>
		Object.entries(localStorage).filter(([k]) => k.startsWith('byl.contributions.'))
	);
	expect(JSON.stringify(stored)).not.toMatch(
		/\\"(lat|lng|latitude|longitude|timestampMs|accuracyM|trace|token)\\"/
	);
	expect(JSON.parse(stored[0][1])[0].queue).toHaveLength(1);
	await page
		.getByRole('button', { name: 'Stop and delete contributions from this browser' })
		.click();
	await expect.poll(() => f.withdrawals.length).toBeGreaterThan(0);
	expect(f.withdrawals[0]).toEqual({
		consent_id: f.consent.id,
		deletion_handle: f.consent.deletion_handle
	});
	expect(f.withdrawalAuthorizations[0]).toBeUndefined();
	await expect
		.poll(() =>
			page.evaluate(() =>
				JSON.parse(
					Object.entries(localStorage).find(([k]) => k.startsWith('byl.contributions.'))![1]
				)
			)
		)
		.toEqual([]);
	await page.setViewportSize({ width: 390, height: 844 });
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('contribution-controls-mobile.png') });
});

test('pause excludes incomplete foreground sampling', async ({ page, request }) => {
	const f = await setup(page, request);
	await page
		.getByRole('button', { name: 'Contribute day-level summaries for this revision and mode' })
		.click();
	await expect(page.getByRole('button', { name: 'Pause route observation' })).toBeVisible();
	await page.getByRole('button', { name: 'Pause route observation' }).click();
	await passage(page, f);
	expect(f.summaries).toEqual([]);
	await expect(page.getByRole('button', { name: 'Resume route observation' })).toBeVisible();
});

test('a blocked durable store prevents contribution requests', async ({ page, request }) => {
	const f = await setup(page, request);
	await page.evaluate(() => {
		const original = Storage.prototype.setItem;
		Storage.prototype.setItem = function (k, v) {
			if (k.startsWith('byl.contributions.'))
				throw new DOMException('fixture full', 'QuotaExceededError');
			original.call(this, k, v);
		};
	});
	await page
		.getByRole('button', { name: 'Contribute day-level summaries for this revision and mode' })
		.click();
	expect(f.grants).toEqual([]);
	expect(f.summaries).toEqual([]);
	await expect(
		page.getByText(/storage is unavailable|Collection could not be checked/)
	).toBeVisible();
});

test('profile retry expires a seven-day offline summary and deletion works after sign-out', async ({
	page,
	request
}) => {
	const f = await setup(page, request, true, true);
	await page
		.getByRole('button', { name: 'Contribute day-level summaries for this revision and mode' })
		.click();
	await expect(page.getByRole('button', { name: 'Pause route observation' })).toBeVisible();
	await passage(page, f);
	await expect.poll(() => f.summaries.length).toBeGreaterThan(0);
	await page.goto('/profile');
	await page.getByText('Optional route improvement', { exact: true }).click();
	await page.evaluate(
		(now) => {
			window.bylFixtureNow = now;
		},
		f.epoch + 7 * 86400000
	);
	const response = page.waitForResponse((r) =>
		r.url().endsWith('/api/v1/route-contribution-intake')
	);
	await page.getByRole('button', { name: 'Retry contribution work' }).click();
	await (await response).finished();
	await expect
		.poll(() =>
			page.evaluate(() => {
				const raw = Object.entries(localStorage).find(([k]) =>
					k.startsWith('byl.contributions.')
				)![1];
				return JSON.parse(raw)[0].queue.length;
			})
		)
		.toBe(0);
	await page.getByRole('button', { name: 'Sign out', exact: true }).click();
	await expect(page).toHaveURL('/');
	await page.goto('/profile');
	await page.getByText('Optional route improvement', { exact: true }).click();
	await page
		.getByRole('button', { name: 'Stop and delete contributions from this browser' })
		.click();
	await expect.poll(() => f.withdrawals.length).toBeGreaterThan(0);
	expect(f.withdrawalAuthorizations.at(-1)).toBeUndefined();
	await page.setViewportSize({ width: 320, height: 900 });
	await page.evaluate(() => {
		document.documentElement.style.fontSize = '200%';
	});
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
});
