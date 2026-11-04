import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

// These tests drive the built app against a real API running with DEV_AUTH_ENABLED=true and
// ADMIN_BOOTSTRAP_EMAILS=admin@example.com on a fresh (seeded) database. Set E2E_API_URL to run them.
const apiUrl = process.env.E2E_API_URL;
test.skip(!apiUrl, 'E2E_API_URL not set');

async function devToken(request: APIRequestContext, email: string): Promise<string> {
	const res = await request.post(`${apiUrl}/api/v1/auth/dev`, {
		data: { email, display_name: email.split('@')[0] }
	});
	expect(res.ok()).toBeTruthy();
	return (await res.json()).token as string;
}

async function signIn(page: Page, email: string) {
	await page.goto('/signin');
	await page.getByLabel('Email').fill(email);
	await page.getByRole('button', { name: /sign in/i }).click();
}

test('a visitor account sees the curator-role notice, never a blank page', async ({ page }) => {
	await signIn(page, `visitor-${Date.now()}@example.com`);
	await expect(page.getByRole('heading', { name: /needs the curator role/i })).toBeVisible();
	await page.getByRole('button', { name: /sign out/i }).click();
	await expect(page).toHaveURL(/\/signin/);
});

test('an admin can approve a candidate with an edited title', async ({ page, request }) => {
	const token = await devToken(request, 'admin@example.com');
	const stationId = '00000000-0000-4000-8000-00000000b0b1';
	const created = await request.post(`${apiUrl}/api/v1/admin/candidates`, {
		headers: { Authorization: `Bearer ${token}` },
		data: {
			kind: 'moment',
			origin: 'import',
			source_name: 'e2e',
			external_id: `e2e-${Date.now()}`,
			target_place_id: stationId,
			payload: {
				title: 'Station Fire',
				raw_text: 'A fire damaged the depot in 1910.',
				source_url: 'https://example.org/fire',
				source_title: 'Example archive'
			}
		}
	});
	expect(created.ok()).toBeTruthy();
	const candidate = await created.json();

	await signIn(page, 'admin@example.com');
	await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
	await page.goto(`/queue/${candidate.id}`);
	await expect(page.getByText('A fire damaged the depot in 1910.')).toBeVisible();
	await page.getByLabel('Title').fill('Station Fire of 1910');
	await page.getByRole('button', { name: /approve/i }).click();
	await expect(page.getByText(/approved/i).first()).toBeVisible();

	const approved = await request.get(`${apiUrl}/api/v1/admin/candidates/${candidate.id}`, {
		headers: { Authorization: `Bearer ${token}` }
	});
	const body = await approved.json();
	expect(body.status).toBe('approved');
	const moment = await request.get(`${apiUrl}/api/v1/admin/moments/${body.approved_entity_id}`, {
		headers: { Authorization: `Bearer ${token}` }
	});
	expect((await moment.json()).title).toBe('Station Fire of 1910');
});
