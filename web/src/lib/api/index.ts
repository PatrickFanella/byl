import { ApiClient } from './client';
import { session } from '../auth';

export * from './client';

/** The app-wide client. Same origin by default; VITE_API_BASE_URL overrides it. */
export const api = new ApiClient({
	baseUrl: import.meta.env.VITE_API_BASE_URL ?? '',
	token: () => session.token()
});
