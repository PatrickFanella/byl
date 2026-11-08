import { expect, test } from '@playwright/test';
import { readFileSync } from 'node:fs';
import type { CuratorReviewPackage } from '../src/lib/admin.ts';

const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');
const packet = new URL('../../docs/product/pilots/prairie-path/', import.meta.url);
const read = (name: string) => JSON.parse(readFileSync(new URL(name, packet), 'utf8'));
const stops = read('stops.json') as { name: string; body: string; moment_title: string }[];

// The transport fixture comes from the actual PostgreSQL/HTTP replay. Its repeated
// draft bodies are stored once in stops.json and restored here. Authentication and
// brief loading use the disposable API; publication/access behavior has real PG proof.
test('prairie-path draft preview retains proposed arrivals and strict topology gaps, computed geometry and publication blockers at phone width', async ({
	page,
	request
}, info) => {
	await page.setViewportSize({ width: 390, height: 844 });
	const auth = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	const admin = await auth.json();
	const signedIn = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email: 'prairie-path-browser-curator@example.com' }
	});
	const session = await signedIn.json();
	const granted = await request.patch(
		`${apiUrl}/api/v1/admin/accounts/${session.account.id}/role`,
		{
			headers: { Authorization: `Bearer ${admin.token}` },
			data: { role: 'curator' }
		}
	);
	expect(granted.status()).toBe(200);
	const identity = await request.get(`${apiUrl}/api/v1/me`, {
		headers: { Authorization: `Bearer ${session.token}` }
	});
	session.account = await identity.json();
	const created = await request.post(`${apiUrl}/api/v1/admin/route-briefs`, {
		headers: { Authorization: `Bearer ${session.token}` },
		data: read('brief.json')
	});
	expect(created.status()).toBe(201);
	const brief = await created.json();
	const report = read('review-package.fixture.json') as CuratorReviewPackage;
	report.brief = brief;
	for (const story of report.visitor_stories) {
		const draft = stops.find((stop) => stop.moment_title === story.title);
		expect(draft).toBeDefined();
		story.body = draft!.body;
	}
	expect(report.ready_to_publish).toBe(false);
	expect(report.routes.bike?.geometry?.coordinates.length).toBeGreaterThan(30);
	expect(report.routes.bike?.provider).toBe('valhalla');
	let publications = 0;
	page.on('request', (req) => {
		if (req.method() === 'POST' && req.url().includes('/review-package/publish')) publications++;
	});
	await page.route('**/api/v1/admin/tours?*', (route) =>
		route.fulfill({ json: { items: [report.tour], next_cursor: '' } })
	);
	await page.route(`**/api/v1/admin/route-briefs/${brief.id}/review-package?*`, (route) =>
		route.fulfill({ json: report })
	);
	await page.goto('/signin');
	await page.evaluate(
		(value) => localStorage.setItem('invisible-museum.session', JSON.stringify(value)),
		session
	);
	await page.goto(`/admin/briefs/${brief.id}`);
	await page.getByLabel('Prepared tour', { exact: true }).selectOption(report.tour!.id);
	await page.getByRole('button', { name: 'Reload review package' }).click();
	const panel = page.getByRole('region', { name: 'Curator package review' });
	await expect(
		panel.getByRole('heading', { name: `Visitor preview: ${report.tour!.title}` })
	).toBeVisible();
	for (const draft of stops) {
		await expect(
			panel.getByRole('heading', {
				name: new RegExp(draft.moment_title.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'))
			})
		).toBeVisible();
		await expect(panel.getByText(draft.body, { exact: true })).toBeVisible();
	}
	await expect(panel.getByRole('link', { name: 'Historical source 1', exact: true })).toHaveCount(
		3
	);
	await expect(panel.getByRole('checkbox', { name: /I reviewed the stories/ })).toBeDisabled();
	await expect(
		panel.getByRole('button', { name: 'Approve and publish reviewed package' })
	).toBeDisabled();
	expect(publications).toBe(0);
	expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(
		true
	);
	await panel
		.getByRole('heading', { name: `Visitor preview: ${report.tour!.title}` })
		.scrollIntoViewIfNeeded();
	await panel.locator('canvas').scrollIntoViewIfNeeded();
	await expect(panel.locator('.maplibregl-marker')).toHaveCount(3, { timeout: 30_000 });
	await expect(panel.locator('.maplibregl-marker').first()).toBeInViewport();
	await page.screenshot({ path: info.outputPath('prairie-path-mobile.png') });
});
