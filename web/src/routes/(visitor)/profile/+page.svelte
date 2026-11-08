<script lang="ts">
	import AppearancePicker from '#lib/components/AppearancePicker.svelte';
	import ContributionSettings from '#lib/components/visitor/ContributionSettings.svelte';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { session } from '#lib/auth.ts';
	import { api } from '#lib/api/index.ts';

	const current = session.store;
	const AUDIO_KEY = 'invisible-museum.audio';
	let audio = $state(false);
	let signingOut = $state(false);

	let locationState = $state('not requested');
	onMount(() => {
		let permission: PermissionStatus | undefined;
		void navigator.permissions
			?.query({ name: 'geolocation' })
			.then((status) => {
				permission = status;
				const update = () => {
					locationState =
						status.state === 'granted'
							? 'allowed'
							: status.state === 'denied'
								? 'blocked'
								: 'not requested';
				};
				update();
				status.onchange = update;
			})
			.catch(() => {
				locationState = 'unavailable';
			});
		return () => {
			if (permission) permission.onchange = null;
		};
	});

	$effect(() => {
		try {
			audio = localStorage.getItem(AUDIO_KEY) === '1';
		} catch {
			// ignore
		}
	});

	function setAudio(on: boolean) {
		audio = on;
		try {
			localStorage.setItem(AUDIO_KEY, on ? '1' : '0');
		} catch {
			// ignore
		}
	}

	async function signOut() {
		signingOut = true;
		try {
			await api.del('/api/v1/auth/session');
		} catch {
			// token may already be gone
		}
		session.clear();
		signingOut = false;
		goto('/');
	}
</script>

<svelte:head><title>Profile · Byl</title></svelte:head>

<div class="text-xs font-bold tracking-normal text-byl-action">Profile</div>
<div class="my-6"><AppearancePicker /></div>
<h1 class="text-3xl font-semibold">{$current?.account.display_name || 'Visitor'}</h1>

<div class="mt-6 grid gap-4 sm:grid-cols-2">
	<section class="rounded-lg border border-byl-border bg-byl-surface p-5">
		<div class="text-xs font-bold tracking-normal text-byl-action">Account</div>
		{#if $current}
			<p class="mt-2 text-sm">{$current.account.email}</p>
			<p class="text-xs text-byl-muted">{$current.account.role}</p>
			<button
				class="mt-3 rounded-lg border border-byl-border px-3 py-2 text-sm hover:bg-byl-surface disabled:opacity-50"
				onclick={signOut}
				disabled={signingOut}>Sign out</button
			>
		{:else}
			<p class="mt-2 text-sm text-byl-muted">
				Sign in to save tours, keep progress, and build routes. Browsing and walking tours work
				without an account.
			</p>
			<a
				href="/signin?next=/profile"
				class="mt-3 inline-block rounded-lg bg-byl-action px-3 py-2 text-sm font-semibold text-byl-action-text"
				>Sign in</a
			>
		{/if}
	</section>
	<section class="rounded-lg border border-byl-border bg-byl-surface p-5">
		<div class="text-xs font-bold tracking-normal text-byl-action">On the tour</div>
		<label class="mt-2 flex items-start gap-3 text-sm">
			<input
				type="checkbox"
				class="mt-1"
				checked={audio}
				onchange={(e) => setAudio(e.currentTarget.checked)}
			/>
			<span>
				<span class="block font-semibold">Audio guide</span>
				<span class="block text-xs text-byl-muted"
					>Play each stop's story when you arrive: recorded narration when there is one, otherwise
					read aloud by your browser.</span
				>
			</span>
		</label>
		<p class="mt-3 text-xs text-byl-muted">Location: {locationState}</p>
		<p class="mt-3 text-xs text-byl-muted">
			Location is requested by your browser when you start a tour; the walk also works with the "I'm
			here" button.
		</p>
	</section>
	<section class="min-w-0 rounded-lg border border-byl-border bg-byl-surface p-5 sm:col-span-2">
		<ContributionSettings />
	</section>
	<section class="rounded-lg border border-byl-border bg-byl-surface p-5 sm:col-span-2">
		<div class="text-xs font-bold tracking-normal text-byl-action">About</div>
		<p class="mt-2 text-sm text-byl-muted">
			Stories are drawn from open archives and reviewed by curators; each moment lists its sources
			and media credits.
		</p>
	</section>
</div>
