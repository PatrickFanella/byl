import { expect, test, type Page, type APIRequestContext } from '@playwright/test';

const apiURL = process.env.E2E_API_URL;
test.skip(!apiURL, 'E2E_API_URL not set');

async function signIn(page: Page, email = `suggestion-${crypto.randomUUID()}@example.com`) {
	await page.goto('/signin');
	await page.getByLabel('Email', { exact: true }).fill(email);
	await page.getByRole('button', { name: /sign in/i }).click();
	await expect(page).not.toHaveURL(/\/signin/);
}

async function adminToken(request: APIRequestContext) {
	const response = await request.post(`${apiURL}/api/v1/auth/dev`, {
		data: { email: 'admin@example.com' }
	});
	expect(response.ok()).toBeTruthy();
	return (await response.json()).token as string;
}

async function requireOpenIntake(request: APIRequestContext) {
	const response = await request.get(`${apiURL}/api/v1/route-brief-intake`);
	test.skip(!(await response.json()).enabled, 'This fixture intentionally closes new intake.');
}

async function submitMinimal(page: Page, topic: string) {
	await page.goto('/routes/suggestions');
	await page.getByLabel('Topic (required)').fill(topic);
	await page.getByLabel('Area (required)').fill('Downers Grove');
	await page.getByRole('button', { name: 'Submit suggestion', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/suggestions\/[0-9a-f-]+\?submitted=1$/);
}

test('suggestions and private status ask signed-out visitors to sign in', async ({ page }) => {
	const privateCalls: string[] = [];
	page.on('request', (request) => {
		if (request.url().includes('/api/v1/me/route-briefs')) privateCalls.push(request.url());
	});
	await page.goto('/routes/suggestions');
	await expect(
		page.getByText('Sign in to suggest a tour and read your private submission status.')
	).toBeVisible();
	await expect(page.getByRole('button', { name: 'Submit suggestion' })).toHaveCount(0);
	await page.goto('/routes/suggestions/00000000-0000-4000-8000-000000000000');
	await expect(page.getByText('Sign in to read your private suggestion status.')).toBeVisible();
	expect(privateCalls).toEqual([]);
});

test('a minimal topic and area become a private suggestion, separate from personal routes', async ({
	page,
	request
}, info) => {
	await requireOpenIntake(request);
	await signIn(page);
	await page.goto('/routes');
	await page.getByRole('link', { name: 'Tour suggestions', exact: true }).click();
	await page.setViewportSize({ width: 320, height: 740 });
	await expect(page.getByLabel('Topic (required)')).toBeVisible();
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('suggestion-form-320.png'), fullPage: true });
	await submitMinimal(page, 'Workers and the railroad');
	await expect(
		page.getByRole('heading', { name: 'Workers and the railroad', exact: true })
	).toBeVisible();
	await expect(page.getByRole('heading', { name: 'Status: Received' })).toBeVisible();
	await expect(
		page.getByText('Suggestion saved privately. Curators will review it before any publication.')
	).toBeVisible();
	await expect(page.getByText(/not verified history/)).toBeVisible();
	await expect(page.getByRole('link', { name: /start walking|share|publish/i })).toHaveCount(0);
	expect(
		await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)
	).toBeTruthy();
	await page.screenshot({ path: info.outputPath('suggestion-status-320.png'), fullPage: true });
	await page.goto('/routes');
	await expect(page.getByText('No routes yet', { exact: true })).toBeVisible();
	await page.getByRole('link', { name: 'Tour suggestions', exact: true }).click();
	await expect(
		page.getByRole('link', { name: 'Workers and the railroad', exact: true })
	).toBeVisible();
});

test('optional leads and useful field errors retain the suggestion input', async ({
	page,
	request
}) => {
	await requireOpenIntake(request);
	await signIn(page);
	await page.goto('/routes/suggestions');
	await page.getByRole('button', { name: 'Submit suggestion', exact: true }).click();
	await expect(page.getByLabel('Topic (required)')).toHaveAttribute('aria-invalid', 'true');
	await expect(page.getByLabel('Topic (required)')).toBeFocused();
	await page.getByLabel('Topic (required)').fill('A question about the depot');
	await page.keyboard.press('Tab');
	await expect(page.getByLabel('Area (required)')).toBeFocused();
	expect(
		await page.getByLabel('Area (required)').evaluate((el) => getComputedStyle(el).outlineWidth)
	).toBe('3px');
	await page.keyboard.type('DuPage County');
	await page.getByText('Optional research leads', { exact: true }).click();
	await page
		.getByLabel('Research notes', { exact: true })
		.fill('An unverified question for a curator.');
	await page.getByLabel('Place leads', { exact: true }).fill('An old depot, address unknown');
	await page.getByLabel('Event leads', { exact: true }).fill('When did the line open?');
	await page.getByLabel('Source links', { exact: true }).fill('not-a-link');
	await page.getByRole('button', { name: 'Submit suggestion', exact: true }).click();
	await expect(page.getByLabel('Source links', { exact: true })).toBeFocused();
	await expect(page.getByLabel('Source links', { exact: true })).toHaveAccessibleDescription(
		/HTTP or HTTPS links without credentials/
	);
	await expect(page.getByLabel('Research notes', { exact: true })).toHaveValue(
		'An unverified question for a curator.'
	);
	await page.getByLabel('Source links', { exact: true }).fill('https://example.org/depot');
	await page.getByRole('button', { name: 'Submit suggestion', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/suggestions\/[0-9a-f-]+\?submitted=1$/);
	await expect(page.getByText('An old depot, address unknown', { exact: true })).toBeVisible();
	await expect(page.getByText('When did the line open?', { exact: true })).toBeVisible();
	await expect(
		page.getByRole('link', { name: 'https://example.org/depot', exact: true })
	).toHaveAttribute('href', 'https://example.org/depot');
});

test('a lost submission response survives reload and retries the same body and key', async ({
	page,
	request
}) => {
	await requireOpenIntake(request);
	await signIn(page);
	let lost = false;
	const attempts: { key: string | undefined; body: unknown }[] = [];
	await page.route('**/api/v1/me/route-briefs', async (route) => {
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
	await page.goto('/routes/suggestions');
	await page.getByLabel('Topic (required)').fill('One suggestion after response loss');
	await page.getByLabel('Area (required)').fill('Downers Grove');
	await page.getByRole('button', { name: 'Submit suggestion', exact: true }).click();
	await expect(page.getByRole('alert')).toContainText('We could not confirm the submission');
	await expect(page.getByLabel('Topic (required)')).toBeDisabled();
	await page.reload();
	await expect(page.getByLabel('Topic (required)')).toHaveValue(
		'One suggestion after response loss'
	);
	await expect(
		page.getByRole('link', { name: 'One suggestion after response loss', exact: true })
	).toHaveCount(1);
	await page.getByRole('button', { name: 'Retry same suggestion', exact: true }).click();
	await expect(page).toHaveURL(/\/routes\/suggestions\/[0-9a-f-]+\?submitted=1$/);
	expect(attempts).toHaveLength(2);
	expect(attempts[1]).toEqual(attempts[0]);
	expect(attempts[0].key).toMatch(/^[0-9a-f-]{36}$/);
	await page.goto('/routes/suggestions');
	await expect(
		page.getByRole('link', { name: 'One suggestion after response loss', exact: true })
	).toHaveCount(1);
	await expect(page.getByLabel('Topic (required)')).toHaveValue('');
});

test('owners follow research, review and decision states while another account cannot read them', async ({
	page,
	request,
	browser
}) => {
	await requireOpenIntake(request);
	await signIn(page);
	await submitMinimal(page, 'Private lifecycle observation');
	const path = page.url().split('?')[0];
	const id = path.split('/').pop();
	const token = await adminToken(request);
	const states = [
		['processing', 'Research in progress'],
		['needs_research', 'More research needed'],
		['review', 'Curator review'],
		['accepted', 'Accepted'],
		['draft', 'Received'],
		['declined', 'Declined']
	];
	for (const [index, [status, label]] of states.entries()) {
		const response = await request.post(`${apiURL}/api/v1/admin/route-briefs/${id}/state`, {
			headers: { Authorization: `Bearer ${token}` },
			data: {
				version: index + 1,
				status,
				reason: `Private decision ${status}: sources require curator judgement.`
			}
		});
		expect(response.ok()).toBeTruthy();
		await page.getByRole('button', { name: 'Refresh status', exact: true }).click();
		await expect(
			page.getByRole('heading', { name: `Status: ${label}`, exact: true })
		).toBeVisible();
		await expect(
			page.getByText(`Private decision ${status}: sources require curator judgement.`, {
				exact: true
			})
		).toBeVisible();
	}
	const stranger = await browser.newContext();
	const other = await stranger.newPage();
	await signIn(other);
	await other.goto(path);
	await expect(
		other.getByText('This suggestion is not available for this account.', { exact: true })
	).toBeVisible();
	await expect(other.getByText('Private lifecycle observation', { exact: true })).toHaveCount(0);
	await expect(other.getByText(/Private decision declined/)).toHaveCount(0);
	await other.goto('/routes/suggestions');
	await expect(
		other.getByRole('link', { name: 'Private lifecycle observation', exact: true })
	).toHaveCount(0);
	await stranger.close();
});

test('closed intake keeps earlier status readable and personal route planning available', async ({
	page,
	request
}) => {
	const token = await adminToken(request);
	const topic = `Before closure ${crypto.randomUUID()}`;
	const response = await request.post(`${apiURL}/api/v1/admin/route-briefs`, {
		headers: { Authorization: `Bearer ${token}` },
		data: { topic, area: 'Downers Grove' }
	});
	expect(response.status()).toBe(201);
	const capability = await request.get(`${apiURL}/api/v1/route-brief-intake`);
	const realGate = (await capability.json()).enabled;
	// Default fixture opens intake. A second local run with the env gate false
	// uses the real closed API, including the direct-POST rejection below.
	if (realGate)
		await page.route('**/api/v1/route-brief-intake', (route) =>
			route.fulfill({ json: { enabled: false } })
		);
	else {
		const rejected = await request.post(`${apiURL}/api/v1/me/route-briefs`, {
			headers: { Authorization: `Bearer ${token}` },
			data: { topic: 'Closed', area: 'Here' }
		});
		expect(rejected.status()).toBe(403);
		expect((await rejected.json()).error.code).toBe('intake_disabled');
	}
	await signIn(page, 'admin@example.com');
	await page.goto('/routes/suggestions');
	await expect(page.getByText(/New suggestions are closed/)).toBeVisible();
	await expect(page.getByRole('button', { name: 'Submit suggestion' })).toHaveCount(0);
	await page.getByRole('link', { name: topic, exact: true }).click();
	await expect(page.getByRole('heading', { name: topic, exact: true })).toBeVisible();
	await page.goto('/routes/new');
	await expect(page.getByLabel('Name', { exact: true })).toBeEnabled();
	await expect(page.getByRole('heading', { name: 'New route', exact: true })).toBeVisible();
});

test('unfinished drafts resume in this tab and stay partitioned by account', async ({
	page,
	request
}) => {
	await requireOpenIntake(request);
	await signIn(page);
	await page.goto('/routes/suggestions');
	await page.getByLabel('Topic (required)').fill('Unfinished private thought');
	await page.getByLabel('Area (required)').fill('A neighborhood');
	await page.getByRole('link', { name: 'plan a personal route', exact: true }).click();
	await page.goto('/routes/suggestions');
	await expect(page.getByLabel('Topic (required)')).toHaveValue('Unfinished private thought');
	await page.reload();
	await expect(page.getByLabel('Area (required)')).toHaveValue('A neighborhood');
	await page.goto('/profile');
	await page.getByRole('button', { name: /sign out/i }).click();
	await signIn(page);
	await page.goto('/routes/suggestions');
	await expect(page.getByLabel('Topic (required)')).toHaveValue('');
	await expect(page.getByLabel('Area (required)')).toHaveValue('');
});

test('an unavailable intake capability fails closed without hiding saved statuses', async ({
	page,
	request
}) => {
	await requireOpenIntake(request);
	await signIn(page);
	await page.route('**/api/v1/route-brief-intake', (route) => route.abort());
	await page.goto('/routes/suggestions');
	await expect(page.getByRole('alert')).toContainText(
		'We could not check whether new suggestions are open.'
	);
	await expect(page.getByRole('button', { name: 'Submit suggestion' })).toHaveCount(0);
	await expect(page.getByRole('heading', { name: 'Your submissions', exact: true })).toBeVisible();
	await page.unroute('**/api/v1/route-brief-intake');
	await page.getByRole('button', { name: 'Check intake again', exact: true }).click();
	await expect(page.getByRole('button', { name: 'Submit suggestion', exact: true })).toBeVisible();
});

test('a closed or expired submission keeps its retry identity until the account can continue', async ({
	page,
	request
}) => {
	await requireOpenIntake(request);
	await signIn(page);
	for (const [status, code, message] of [
		[403, 'intake_disabled', 'Suggestions closed'],
		[401, 'unauthorized', 'Sign in again']
	] as const) {
		await page.route('**/api/v1/me/route-briefs', (route) =>
			route.request().method() === 'POST'
				? route.fulfill({ status, json: { error: { code, message: 'Submission unavailable' } } })
				: route.continue()
		);
		await page.goto('/routes/suggestions');
		await page.getByLabel('Topic (required)').fill(`Retain original request ${status}`);
		await page.getByLabel('Area (required)').fill('Downers Grove');
		await page.getByRole('button', { name: 'Submit suggestion', exact: true }).click();
		await expect(page.getByRole('alert')).toContainText('original input and retry key');
		await expect(page.getByLabel('Topic (required)')).toHaveValue(
			`Retain original request ${status}`
		);
		if (status === 403)
			await expect(page.getByRole('button', { name: message, exact: true })).toBeDisabled();
		else await expect(page.getByRole('link', { name: message, exact: true })).toBeVisible();
		await page.unroute('**/api/v1/me/route-briefs');
		await page.reload();
		await expect(
			page.getByRole('button', { name: 'Retry same suggestion', exact: true })
		).toBeEnabled();
		await page.getByRole('button', { name: 'Retry same suggestion', exact: true }).click();
		await expect(page).toHaveURL(/\/routes\/suggestions\/[0-9a-f-]+\?submitted=1$/);
		await expect(
			page.getByRole('heading', { name: `Retain original request ${status}`, exact: true })
		).toBeVisible();
	}
});

test('older submission pages load by offset without losing the account scope', async ({
	page,
	request
}) => {
	await requireOpenIntake(request);
	await signIn(page);
	await submitMinimal(page, 'Pagination fixture seed');
	const offsets: number[] = [];
	let seed: { submitted: { topic: string } } | undefined;
	// Only list volume is synthetic; this browser still uses its real owner token.
	await page.route('**/api/v1/me/route-briefs?*', async (route) => {
		const response = await route.fetch();
		seed ??= (await response.json()).items[0];
		const template = seed;
		if (!template) throw new Error('Missing owner record for pagination fixture');
		const offset = Number(new URL(route.request().url()).searchParams.get('offset'));
		offsets.push(offset);
		const count = offset === 0 ? 21 : 1;
		await route.fulfill({
			json: {
				items: Array.from({ length: count }, (_, index) => ({
					...template,
					id: `00000000-0000-4000-8000-${String(offset + index + 1).padStart(12, '0')}`,
					submitted: { ...template.submitted, topic: `Page item ${offset + index + 1}` }
				}))
			}
		});
	});
	await page.goto('/routes/suggestions');
	await expect(page.getByRole('link', { name: 'Page item 1', exact: true })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Page item 21', exact: true })).toHaveCount(0);
	await page.getByRole('button', { name: 'Next submissions', exact: true }).click();
	await expect(page.getByRole('link', { name: 'Page item 21', exact: true })).toBeVisible();
	await expect(page.getByRole('link', { name: 'Page item 1', exact: true })).toHaveCount(0);
	await page.getByRole('button', { name: 'Previous submissions', exact: true }).click();
	await expect(page.getByRole('link', { name: 'Page item 1', exact: true })).toBeVisible();
	expect(offsets).toEqual([0, 20, 0]);
});
