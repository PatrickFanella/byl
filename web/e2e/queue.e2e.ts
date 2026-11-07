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
	await expect(page).not.toHaveURL(/\/signin/);
}

test('a visitor account sees the curator-role notice, never a blank page', async ({ page }) => {
	await signIn(page, `visitor-${Date.now()}@example.com`);
	await page.goto('/admin');
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
	await page.goto('/admin');
	await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
	await page.goto(`/admin/queue/${candidate.id}`);
	await expect(page.getByText('A fire damaged the depot in 1910.')).toBeVisible();
	await page.getByLabel('Title', { exact: true }).fill('Station Fire of 1910');
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

// Review fix: typing a place id must not throw away edits made to the other fields.
test('setting the place id keeps the edited title', async ({ page, request }) => {
	const token = await devToken(request, 'admin@example.com');
	const stationId = '00000000-0000-4000-8000-00000000b0b1';
	const created = await request.post(`${apiUrl}/api/v1/admin/candidates`, {
		headers: { Authorization: `Bearer ${token}` },
		data: {
			kind: 'moment',
			origin: 'import',
			source_name: 'e2e',
			external_id: `e2e-noplace-${Date.now()}`,
			payload: {
				title: 'Depot Dedication',
				raw_text: 'The new depot opened to the public in 1908.',
				source_url: 'https://example.org/depot',
				source_title: 'Example archive'
			}
		}
	});
	expect(created.ok()).toBeTruthy();
	const candidate = await created.json();

	await signIn(page, 'admin@example.com');
	await page.goto('/admin');
	await expect(page.getByRole('heading', { name: 'Dashboard' })).toBeVisible();
	await page.goto(`/admin/queue/${candidate.id}`);
	await expect(page.getByText(/this moment has no place/i)).toBeVisible();
	await page.getByLabel('Title', { exact: true }).fill('Depot Dedication of 1908');
	await page.getByLabel('Place id').fill(stationId);
	await page.getByLabel('Place id').blur();
	await expect(page.getByLabel('Title', { exact: true })).toHaveValue('Depot Dedication of 1908');
	await expect(page.getByText(/this moment has no place/i)).toHaveCount(0);
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
	const m = await moment.json();
	expect(m.title).toBe('Depot Dedication of 1908');
	expect(m.place_id).toBe(stationId);
});

test('an inspected draft proposal preserves unsaved editor text until explicitly applied', async ({
	page,
	request
}) => {
	await page.setViewportSize({ width: 390, height: 844 });
	const token = await devToken(request, 'admin@example.com');
	const created = await request.post(`${apiUrl}/api/v1/admin/candidates`, {
		headers: { Authorization: `Bearer ${token}` },
		data: {
			kind: 'moment',
			origin: 'import',
			target_place_id: '00000000-0000-4000-8000-00000000b0b1',
			payload: {
				title: 'Original title',
				body: 'Original body',
				raw_text: 'An uninspected anecdote.'
			}
		}
	});
	expect(created.ok()).toBeTruthy();
	const candidate = await created.json();
	await signIn(page, 'admin@example.com');
	await page.goto(`/admin/queue/${candidate.id}`);
	const evidenceId = '00000000-0000-4000-8000-00000000f001';
	const proposal = {
		title: 'An inspected proposal',
		title_citations: [{ evidence_id: evidenceId, passage: 'paragraph:1', quote: 'depot' }],
		date_citations: [],
		body: 'The archive records the depot opening.',
		date_start: '',
		date_end: '',
		date_display: '',
		date_precision: 'circa',
		notes: 'The exact day needs research.',
		topic_ids: [],
		sources: [{ title: 'Depot archive', url: 'https://archive.example/depot' }],
		review_gaps: [],
		uncertainties: [{ kind: 'research_gap', text: 'The exact day is unresolved.' }],
		claims: [
			{
				text: 'The archive records the depot opening.',
				kind: 'fact',
				attribution: '',
				citations: [{ evidence_id: evidenceId, passage: 'paragraph:1', quote: 'The depot opened.' }]
			}
		],
		evidence_manifest: [
			{
				id: evidenceId,
				title: 'Depot archive',
				url: 'https://archive.example/depot',
				outcome: 'inspected_page',
				content_sha256: 'a'.repeat(64),
				retrieved_at: '2026-01-01T00:00:00Z'
			}
		]
	};
	await page.route('**/api/v1/admin/route-briefs?*', (route) =>
		route.fulfill({
			json: {
				items: [
					{ id: 'brief-fixture', submitted: { topic: 'Rail evidence', area: 'Downers Grove' } }
				]
			}
		})
	);
	await page.route('**/api/v1/admin/route-briefs/brief-fixture/evidence?*', (route) =>
		route.fulfill({
			json: {
				items: [
					{
						id: evidenceId,
						result: {
							outcome: 'inspected_page',
							title: 'Depot archive',
							retrieved_at: '2026-01-01T00:00:00Z',
							content_sha256: 'a'.repeat(64),
							passages: [{ reference: 'paragraph:1', text: 'The depot opened.' }]
						}
					}
				]
			}
		})
	);
	await page.route(`**/api/v1/admin/candidates/${candidate.id}/draft`, (route) => {
		expect(route.request().postDataJSON()).toEqual({ evidence_ids: [evidenceId] });
		return route.fulfill({ status: 202, json: { job: { id: 'draft-job', status: 'queued' } } });
	});
	await page.route('**/api/v1/admin/jobs/draft-job', (route) =>
		route.fulfill({ json: { id: 'draft-job', status: 'done' } })
	);
	await page.route(`**/api/v1/admin/candidates/${candidate.id}`, (route) =>
		route.fulfill({
			json: {
				...candidate,
				origin: 'draft',
				payload: { ...candidate.payload, generated_drafts: [proposal] }
			}
		})
	);
	await page.getByLabel('Title', { exact: true }).fill('Curator title still in progress');
	await page.getByRole('button', { name: 'Draft with AI' }).click();
	await page.keyboard.press('Escape');
	await expect(page.getByRole('dialog')).toHaveCount(0);
	await expect(page.getByRole('button', { name: 'Draft with AI' })).toBeFocused();
	await page.getByRole('button', { name: 'Draft with AI' }).click();
	await page.getByLabel('Research brief').selectOption('brief-fixture');
	await page.getByRole('checkbox', { name: /Depot archive/ }).focus();
	await page.keyboard.press('Space');
	await expect(page.getByRole('checkbox', { name: /Depot archive/ })).toBeChecked();
	await page.getByRole('button', { name: 'Generate proposal' }).click();
	await page.getByLabel('Body (Markdown)').fill('Curator text written while the proposal runs.');
	await expect(page.getByRole('heading', { name: 'Generated proposal' })).toBeVisible();
	await expect(page.getByLabel('Title', { exact: true })).toHaveValue(
		'Curator title still in progress'
	);
	await expect(page.getByLabel('Body (Markdown)')).toHaveValue(
		'Curator text written while the proposal runs.'
	);
	await expect(page.getByText('The depot opened.', { exact: true })).toBeVisible();
	await page
		.getByRole('region', { name: 'Generated proposal' })
		.screenshot({ path: test.info().outputPath('historical-draft-mobile.png') });
	await page.getByRole('button', { name: 'Apply proposal to editor' }).click();
	await expect(page.getByLabel('Title', { exact: true })).toHaveValue('An inspected proposal');
	await expect(page.getByLabel('Body (Markdown)')).toHaveValue(proposal.body);
	await expect(page.getByRole('textbox', { name: /^Draft notes/ })).toHaveValue(
		/The exact day is unresolved/
	);
});
