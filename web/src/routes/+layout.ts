// The admin app is a browser-only single-page app served as static files.
export const ssr = false;
export const prerender = false;

import { session } from '$lib/auth';
import { api, ApiError } from '$lib/api';
import type { Account } from '$lib/auth';

export async function load() {
	const restored = session.restore();
	if (restored) {
		try {
			const me = await api.get<Account>('/api/v1/me');
			session.updateAccount(me);
		} catch (e) {
			if (e instanceof ApiError && e.status === 401) session.clear();
		}
	}
	return {};
}
