// Thin typed fetch wrapper for the Invisible Museum API: adds the bearer token, maps the
// error envelope to ApiError, and unwraps list responses.
import type { components } from './types';

export type Schemas = components['schemas'];

export interface ListPage<T> {
	items: T[];
	nextCursor: string;
}

export class ApiError extends Error {
	status: number;
	code: string;
	fields: Record<string, string>;

	constructor(status: number, code: string, message: string, fields: Record<string, string> = {}) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.code = code;
		this.fields = fields;
	}
}

export interface ApiClientOptions {
	baseUrl?: string;
	token: () => string | null;
	fetch?: typeof fetch;
}

type Method = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export class ApiClient {
	private baseUrl: string;
	private token: () => string | null;
	private fetchFn: typeof fetch;

	constructor(opts: ApiClientOptions) {
		this.baseUrl = (opts.baseUrl ?? '').replace(/\/$/, '');
		this.token = opts.token;
		this.fetchFn = opts.fetch ?? ((input, init) => fetch(input, init));
	}

	async request<T>(method: Method, path: string, body?: unknown, isForm = false): Promise<T> {
		const headers = new Headers({ Accept: 'application/json' });
		const token = this.token();
		if (token) headers.set('Authorization', `Bearer ${token}`);
		let payload: BodyInit | undefined;
		if (isForm) {
			payload = body as FormData;
		} else if (body !== undefined) {
			headers.set('Content-Type', 'application/json');
			payload = JSON.stringify(body);
		}
		const res = await this.fetchFn(this.baseUrl + path, { method, headers, body: payload });
		if (res.status === 204) return undefined as T;
		const text = await res.text();
		let data: unknown = undefined;
		if (text) {
			try {
				data = JSON.parse(text);
			} catch {
				data = undefined;
			}
		}
		if (!res.ok) {
			const envelope = (data as { error?: { code?: string; message?: string; fields?: Record<string, string> } } | undefined)?.error;
			throw new ApiError(res.status, envelope?.code ?? 'http_error', envelope?.message ?? `request failed with status ${res.status}`, envelope?.fields ?? {});
		}
		return data as T;
	}

	get<T>(path: string): Promise<T> {
		return this.request<T>('GET', path);
	}

	async list<T>(path: string): Promise<ListPage<T>> {
		const raw = await this.request<{ items: T[]; next_cursor?: string }>('GET', path);
		return { items: raw.items ?? [], nextCursor: raw.next_cursor ?? '' };
	}

	post<T>(path: string, body?: unknown): Promise<T> {
		return this.request<T>('POST', path, body);
	}

	put<T>(path: string, body?: unknown): Promise<T> {
		return this.request<T>('PUT', path, body);
	}

	patch<T>(path: string, body?: unknown): Promise<T> {
		return this.request<T>('PATCH', path, body);
	}

	del(path: string): Promise<void> {
		return this.request<void>('DELETE', path);
	}

	postForm<T>(path: string, form: FormData): Promise<T> {
		return this.request<T>('POST', path, form, true);
	}
}

/** Query-string helper that drops empty values. */
export function qs(params: Record<string, string | number | boolean | undefined | null>): string {
	const p = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) {
		if (v === undefined || v === null || v === '') continue;
		p.set(k, String(v));
	}
	const s = p.toString();
	return s ? `?${s}` : '';
}
