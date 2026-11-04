import { describe, expect, it, vi, beforeEach } from 'vitest';
import { ApiClient, ApiError } from './client';

function jsonResponse(status: number, body: unknown): Response {
	return new Response(JSON.stringify(body), {
		status,
		headers: { 'Content-Type': 'application/json' }
	});
}

describe('ApiClient', () => {
	let calls: { url: string; init: RequestInit }[];
	let fetchMock: typeof fetch;

	beforeEach(() => {
		calls = [];
		fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
			calls.push({ url: String(input), init: init ?? {} });
			return jsonResponse(200, { items: [{ id: '1' }], next_cursor: '' });
		}) as unknown as typeof fetch;
	});

	it('adds the bearer token and json headers', async () => {
		const client = new ApiClient({ baseUrl: '', token: () => 'tok', fetch: fetchMock });
		await client.get('/api/v1/me');
		const headers = new Headers(calls[0].init.headers);
		expect(headers.get('Authorization')).toBe('Bearer tok');
		expect(headers.get('Accept')).toContain('application/json');
	});

	it('omits authorization when there is no token', async () => {
		const client = new ApiClient({ baseUrl: '', token: () => null, fetch: fetchMock });
		await client.get('/api/v1/cities');
		expect(new Headers(calls[0].init.headers).has('Authorization')).toBe(false);
	});

	it('unwraps list responses', async () => {
		const client = new ApiClient({ baseUrl: '', token: () => null, fetch: fetchMock });
		const page = await client.list<{ id: string }>('/api/v1/cities');
		expect(page.items).toEqual([{ id: '1' }]);
		expect(page.nextCursor).toBe('');
	});

	it('maps the error envelope to ApiError with fields', async () => {
		const failing = vi.fn(async () =>
			jsonResponse(422, {
				error: { code: 'validation_failed', message: 'request failed validation', fields: { title: 'is required' } }
			})
		) as unknown as typeof fetch;
		const client = new ApiClient({ baseUrl: '', token: () => null, fetch: failing });
		await expect(client.post('/api/v1/me/tours', {})).rejects.toMatchObject({
			status: 422,
			code: 'validation_failed',
			fields: { title: 'is required' }
		});
		try {
			await client.post('/api/v1/me/tours', {});
		} catch (e) {
			expect(e).toBeInstanceOf(ApiError);
			expect((e as ApiError).message).toBe('request failed validation');
		}
	});

	it('returns undefined for 204', async () => {
		const noContent = vi.fn(async () => new Response(null, { status: 204 })) as unknown as typeof fetch;
		const client = new ApiClient({ baseUrl: '', token: () => 't', fetch: noContent });
		await expect(client.del('/api/v1/auth/session')).resolves.toBeUndefined();
	});

	it('sends multipart bodies untouched', async () => {
		const client = new ApiClient({ baseUrl: 'http://api', token: () => 't', fetch: fetchMock });
		const form = new FormData();
		form.append('caption', 'x');
		await client.postForm('/api/v1/admin/media', form);
		expect(calls[0].url).toBe('http://api/api/v1/admin/media');
		expect(calls[0].init.body).toBe(form);
		expect(new Headers(calls[0].init.headers).has('Content-Type')).toBe(false);
	});
});
