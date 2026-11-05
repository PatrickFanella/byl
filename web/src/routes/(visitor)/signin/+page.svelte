<script lang="ts">
	import Brand from '#lib/components/Brand.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, ApiError } from '#lib/api/index.ts';
	import { session, type SessionData } from '#lib/auth.ts';

	let email = $state('');
	let displayName = $state('');
	let error = $state('');
	let busy = $state(false);
	let devDisabled = $state(true);

	const googleClientId = import.meta.env.VITE_GOOGLE_CLIENT_ID as string | undefined;

	function finish(data: SessionData) {
		session.set(data);
		const next = page.url.searchParams.get('next') || '/';
		goto(next.startsWith('/') && !next.startsWith('//') && !next.includes('\\') ? next : '/');
	}

	async function devSignIn(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		error = '';
		try {
			const data = await api.post<SessionData>('/api/v1/auth/dev', {
				email,
				display_name: displayName
			});
			finish(data);
		} catch (err) {
			if (err instanceof ApiError && err.status === 404) {
				devDisabled = true;
				error = 'Development sign-in is disabled on this server.';
			} else if (err instanceof ApiError) {
				error = err.fields.email ? `Email ${err.fields.email}` : err.message;
			} else {
				error = 'Could not reach the API.';
			}
		} finally {
			busy = false;
		}
	}

	async function googleCredential(response: { credential: string }) {
		busy = true;
		error = '';
		try {
			const data = await api.post<SessionData>('/api/v1/auth/google', {
				id_token: response.credential
			});
			finish(data);
		} catch (err) {
			error = err instanceof ApiError ? err.message : 'Google sign-in failed.';
		} finally {
			busy = false;
		}
	}

	onMount(() => {
		void api
			.get<{ enabled: boolean }>('/api/v1/auth/dev')
			.then((config) => {
				devDisabled = !config.enabled;
			})
			.catch(() => {
				devDisabled = true;
			});
		if (!googleClientId) return;
		const script = document.createElement('script');
		script.src = 'https://accounts.google.com/gsi/client';
		script.async = true;
		script.onload = () => {
			const g = (
				window as unknown as {
					google?: {
						accounts: {
							id: {
								initialize: (o: object) => void;
								renderButton: (el: HTMLElement, o: object) => void;
							};
						};
					};
				}
			).google;
			const el = document.getElementById('google-button');
			if (!g || !el) return;
			g.accounts.id.initialize({ client_id: googleClientId, callback: googleCredential });
			g.accounts.id.renderButton(el, { theme: 'outline', size: 'large', width: 320 });
		};
		document.head.appendChild(script);
	});
</script>

<div class="flex min-h-screen items-center justify-center bg-byl-background p-6">
	<div
		class="w-full max-w-sm space-y-6 rounded-lg border border-byl-border bg-byl-background p-6 shadow-sm"
	>
		<div>
			<a href="/" aria-label="Byl home"><Brand /></a>
			<h1 class="mt-4 text-xl">Sign in to Byl</h1>
		</div>

		{#if googleClientId}
			<div id="google-button" class="flex justify-center"></div>
			<div class="flex items-center gap-3 text-xs text-byl-muted">
				<span class="h-px flex-1 bg-byl-surface"></span>or<span class="h-px flex-1 bg-byl-surface"
				></span>
			</div>
		{/if}

		{#if !devDisabled}
			<form class="space-y-3" onsubmit={devSignIn}>
				<label class="block text-sm">
					<span class="text-byl-text">Email</span>
					<input
						class="mt-1 w-full rounded-md border-byl-border-control"
						type="email"
						bind:value={email}
						required
						autocomplete="email"
					/>
				</label>
				<label class="block text-sm">
					<span class="text-byl-text">Display name</span>
					<input
						class="mt-1 w-full rounded-md border-byl-border-control"
						type="text"
						bind:value={displayName}
						autocomplete="name"
					/>
				</label>
				<button
					class="w-full rounded-md bg-byl-action px-4 py-2 text-byl-action-text disabled:opacity-50"
					disabled={busy}
					type="submit"
				>
					{busy ? 'Signing in…' : 'Sign in (development)'}
				</button>
				<p class="text-xs text-byl-muted">
					Development sign-in works only when the API runs with DEV_AUTH_ENABLED.
				</p>
			</form>
		{/if}

		{#if error}
			<p class="rounded-md bg-byl-danger-surface p-2 text-sm text-byl-danger">{error}</p>
		{/if}
	</div>
</div>
