import { ApiClient, type Schemas } from '#lib/api/client.ts';
import { session } from '#lib/auth.ts';
import type { ConsentChoice, Consent } from './contribution-store';
import type { DeviationSummary } from './deviation';

export interface ContributionAPI {
	gate(): Promise<{ enabled: boolean; detector_version: string; calibrated: boolean }>;
	grant(choice: ConsentChoice, key: string): Promise<Consent>;
	submit(consent: string, summary: DeviationSummary, key: string): Promise<unknown>;
	withdraw(consent: Consent): Promise<void>;
}

/** Capture one identity and server. Never read a replacement token inside a retry. */
export function contributionClient() {
	const token = session.token();
	const owner = session.account()?.id ?? null;
	const base = new URL(
		import.meta.env.VITE_API_BASE_URL || '/',
		window.location.origin
	).href.replace(/\/$/, '');
	const client = new ApiClient({ baseUrl: base, token: () => token });
	const publicClient = new ApiClient({ baseUrl: base, token: () => null });
	const sameIdentity = () => {
		if (owner === null || session.account()?.id !== owner || session.token() !== token)
			return false;
		try {
			const saved = JSON.parse(localStorage.getItem('invisible-museum.session') ?? 'null');
			return saved?.account?.id === owner && saved?.token === token;
		} catch {
			return false;
		}
	};
	const api: ContributionAPI = {
		gate: () => publicClient.get('/api/v1/route-contribution-intake'),
		grant: (choice, key) =>
			client.request('POST', '/api/v1/me/route-contributions/consent', choice, false, {
				'Idempotency-Key': key
			}),
		submit: (consent, summary, key) =>
			client.request<Schemas['ContributionReceipt']>(
				'POST',
				'/api/v1/me/route-contributions',
				{ consent_id: consent, summary },
				false,
				{ 'Idempotency-Key': key }
			),
		withdraw: (consent) =>
			publicClient.post('/api/v1/route-contributions/withdraw', {
				consent_id: consent.id,
				deletion_handle: consent.deletion_handle
			})
	};
	return { api, owner, base, sameIdentity };
}
