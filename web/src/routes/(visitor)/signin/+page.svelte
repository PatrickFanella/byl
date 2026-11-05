<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { api, ApiError } from '#lib/api/index.ts';
	import { session, type SessionData } from '#lib/auth.ts';

	let email = $state('');
	let displayName = $state('');
	let error = $state('');
	let busy = $state(false);
	let devDisabled = $state(false);

	const googleClientId = import.meta.env.VITE_GOOGLE_CLIENT_ID as string | undefined;

	function finish(data: SessionData) {
		session.set(data);
		const next = page.url.searchParams.get('next') || '/';
		goto(next.startsWith('/') ? next : '/');
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

<div class="flex min-h-screen items-center justify-center bg-stone-50 p-6">
	<div class="w-full max-w-sm space-y-6 rounded-xl border border-stone-200 bg-white p-6 shadow-sm">
		<div>
			<h1 class="text-xl font-semibold">Invisible Museum</h1>
			<p class="text-sm text-stone-600">Curator sign-in</p>
		</div>

		{#if googleClientId}
			<div id="google-button" class="flex justify-center"></div>
			<div class="flex items-center gap-3 text-xs text-stone-400">
				<span class="h-px flex-1 bg-stone-200"></span>or<span class="h-px flex-1 bg-stone-200"
				></span>
			</div>
		{/if}

		{#if !devDisabled}
			<form class="space-y-3" onsubmit={devSignIn}>
				<label class="block text-sm">
					<span class="text-stone-700">Email</span>
					<input
						class="mt-1 w-full rounded-md border-stone-300"
						type="email"
						bind:value={email}
						required
						autocomplete="email"
					/>
				</label>
				<label class="block text-sm">
					<span class="text-stone-700">Display name</span>
					<input
						class="mt-1 w-full rounded-md border-stone-300"
						type="text"
						bind:value={displayName}
						autocomplete="name"
					/>
				</label>
				<button
					class="w-full rounded-md bg-stone-900 px-4 py-2 text-white disabled:opacity-50"
					disabled={busy}
					type="submit"
				>
					{busy ? 'Signing in…' : 'Sign in (development)'}
				</button>
				<p class="text-xs text-stone-500">
					Development sign-in works only when the API runs with DEV_AUTH_ENABLED.
				</p>
			</form>
		{/if}

		{#if error}
			<p class="rounded-md bg-red-50 p-2 text-sm text-red-700">{error}</p>
		{/if}
	</div>
</div>
