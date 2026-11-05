<script lang="ts">
	import { modal } from '#lib/modal.ts';
	import { onMount, untrack } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session } from '#lib/auth.ts';
	import { getMoment, getRoute, getTour, myProgress, putProgress } from '#lib/visitor.ts';
	import { ActiveWalk } from '#lib/tour/walk.svelte.ts';
	import { distanceText } from '#lib/tour/estimate.ts';
	import { mapsURL } from '#lib/tour/directions.ts';
	import { NarrationPlayer, plan } from '#lib/tour/narration.ts';
	import type { TourStop } from '#lib/visitor.ts';
	import Map from '#lib/components/Map.svelte';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import ArrivalPanel from '#lib/components/visitor/ArrivalPanel.svelte';

	const ref = page.params.ref ?? '';
	const mode = page.url.searchParams.get('mode') === 'bike' ? 'bike' : 'walk';
	const AUDIO_KEY = 'invisible-museum.audio';
	const current = session.store;

	const walk: ActiveWalk = new ActiveWalk(ref, mode, {
		fetchTour: getTour,
		fetchRoute: getRoute,
		storage: localStorage,
		progressApi: {
			get: async () => (await myProgress()).items,
			put: (body) => putProgress(walk.tour?.id ?? '', body)
		},
		signedIn: () => $current !== null,
		accountID: () => $current?.account.id ?? null
	});

	let geoState: 'waiting' | 'ok' | 'denied' | 'unavailable' = $state('waiting');
	let audioEnabled = $state(false);
	let playing = $state(false);
	let showEnd = $state(false);
	let playVersion = 0;
	const player = new NarrationPlayer();
	player.onEnd = () => (playing = false);

	onMount(() => {
		try {
			audioEnabled = localStorage.getItem(AUDIO_KEY) === '1';
		} catch {
			// ignore
		}
		walk.load();
		if (!('geolocation' in navigator)) {
			geoState = 'unavailable';
			return;
		}
		const watch = navigator.geolocation.watchPosition(
			(p) => {
				geoState = 'ok';
				walk.handleFix({
					lat: p.coords.latitude,
					lng: p.coords.longitude,
					accuracy: p.coords.accuracy
				});
			},
			(err) => {
				geoState = err.code === err.PERMISSION_DENIED ? 'denied' : 'unavailable';
			},
			{ enableHighAccuracy: true, maximumAge: 5_000, timeout: 20_000 }
		);
		const onVisible = () => {
			if (document.visibilityState === 'visible') walk.syncIfNeeded();
		};
		const onOnline = () => walk.syncIfNeeded();
		const onStorage = (e: StorageEvent) => {
			if (e.key?.startsWith('invisible-museum.progress.')) walk.refreshFromStorage();
		};
		document.addEventListener('visibilitychange', onVisible);
		window.addEventListener('online', onOnline);
		window.addEventListener('storage', onStorage);
		return () => {
			navigator.geolocation.clearWatch(watch);
			document.removeEventListener('visibilitychange', onVisible);
			window.removeEventListener('online', onOnline);
			window.removeEventListener('storage', onStorage);
			stopAudio();
		};
	});

	// Auto-play the featured moment on arrival when the guide is on. Only the arrived stop and
	// the toggle are dependencies; `playing` is read untracked so playback cannot re-run this.
	$effect(() => {
		const stop = walk.arrivedStop;
		const on = audioEnabled;
		if (stop && on) untrack(() => void play(stop));
	});

	function stopAudio() {
		playVersion++;
		player.stop();
		playing = false;
	}

	function toggleListen(stop: TourStop) {
		if (playing) {
			stopAudio();
		} else {
			void play(stop);
		}
	}

	async function play(stop: TourStop) {
		const version = ++playVersion;
		const featured = stop.featured_moment;
		if (!featured) return;
		let narrationURL: string | null = null;
		let body = featured.excerpt;
		try {
			const m = await getMoment(featured.id);
			narrationURL = m.narration?.public_url ?? null;
			body = m.body || body;
		} catch {
			// offline: read the excerpt
		}
		if (version !== playVersion || walk.arrivedStop?.id !== stop.id) return;
		const p = plan(narrationURL, body);
		if (p.kind === 'none') return;
		playing = true;
		player.play(p);
	}

	function toggleAudio() {
		audioEnabled = !audioEnabled;
		try {
			localStorage.setItem(AUDIO_KEY, audioEnabled ? '1' : '0');
		} catch {
			// ignore
		}
		if (!audioEnabled) {
			stopAudio();
		}
	}

	async function end(reset: boolean) {
		stopAudio();
		await walk.flush();
		walk.end(reset);
		goto(`/tours/${ref}`);
	}

	const markers = $derived(
		walk.stops.map((s) => ({
			lat: s.place.latitude,
			lng: s.place.longitude,
			label: `${s.position}. ${s.place.name}`,
			position: s.position,
			visited: walk.isVisited(s),
			next: walk.nextStop?.id === s.id
		}))
	);
	const legs = $derived([
		...walk.pastLegs.map((l) => ({
			coords: l.geometry as [number, number][],
			state: 'past' as const
		})),
		...walk.futureLegs.map((l) => ({
			coords: l.geometry as [number, number][],
			state: 'future' as const
		})),
		...(walk.activeLeg
			? [{ coords: walk.activeLeg.geometry as [number, number][], state: 'active' as const }]
			: [])
	]);
	const user = $derived(walk.lastFix ? { lat: walk.lastFix.lat, lng: walk.lastFix.lng } : null);
	const ua = typeof navigator === 'undefined' ? '' : navigator.userAgent;
</script>

<svelte:head><title>{walk.tour?.title ?? 'Walk'} · Invisible Museum</title></svelte:head>

<div class="relative h-[100dvh] bg-[#0a0a0b] text-white">
	<div class="absolute inset-0">
		<Map {markers} {legs} {user} fit="ahead" height="100%" dark />
	</div>

	<header
		class="pointer-events-none absolute inset-x-0 top-0 z-10 flex items-center justify-between p-3"
	>
		<button
			class="pointer-events-auto rounded-full bg-black/70 px-3 py-1.5 text-sm backdrop-blur"
			onclick={() => (showEnd = true)}>End</button
		>
		<div
			class="pointer-events-auto truncate rounded-full bg-black/70 px-3 py-1.5 text-sm font-semibold backdrop-blur"
		>
			{walk.tour?.title ?? ''}
		</div>
		<button
			class="pointer-events-auto rounded-full bg-black/70 px-3 py-1.5 text-sm backdrop-blur"
			onclick={toggleAudio}
			aria-label={audioEnabled ? 'Audio guide on' : 'Audio guide off'}
			>{audioEnabled ? '🔊' : '🔇'}</button
		>
	</header>

	<div class="absolute inset-x-0 bottom-0 z-10 space-y-2 p-3">
		{#if walk.status !== 'ready'}
			<div class="rounded-2xl bg-black/80 p-4 backdrop-blur">
				<LoadState
					status={walk.status === 'error' ? 'error' : 'loading'}
					message={walk.message}
					loadingText="Opening the gallery…"
					onretry={() => walk.load()}
				/>
			</div>
		{:else}
			{#if walk.fromCache}
				<div
					class="inline-block rounded-full bg-black/70 px-3 py-1 text-xs text-white/70 backdrop-blur"
				>
					Offline: using the saved route
				</div>
			{/if}
			{#if walk.routeMessage}
				<div
					class="inline-block rounded-full bg-black/70 px-3 py-1 text-xs text-amber-200 backdrop-blur"
				>
					{walk.routeMessage}. Stops still work; use Directions.
				</div>
			{/if}
			{#if walk.nextStop}
				{@const next = walk.nextStop}
				<div class="rounded-2xl border border-white/10 bg-black/85 p-4 backdrop-blur">
					<div class="flex items-start justify-between gap-3">
						<div class="min-w-0">
							<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">
								Next · stop {next.position} of {walk.stops.length}
							</div>
							<div class="text-xl font-semibold">{next.place.name}</div>
							{#if next.featured_moment}
								<div class="truncate text-sm text-white/60">{next.featured_moment.title}</div>
							{/if}
						</div>
						<div class="text-right">
							<div class="text-lg font-semibold">
								{walk.distanceToNextM != null ? distanceText(walk.distanceToNextM) : '—'}
							</div>
							<div class="text-[10px] text-white/50">{walk.progressText}</div>
						</div>
					</div>
					{#if geoState === 'denied'}
						<p class="mt-2 text-xs text-white/60">
							Location is off. Tap I'm here at each stop, or allow location in your browser.
						</p>
					{:else if geoState === 'unavailable'}
						<p class="mt-2 text-xs text-white/60">
							Your location is unavailable right now. Tap I'm here at each stop.
						</p>
					{:else if geoState === 'waiting'}
						<p class="mt-2 text-xs text-white/60">Waiting for your location…</p>
					{/if}
					{#if walk.syncError}
						<p class="mt-2 text-xs text-amber-200">
							Progress saved on this device; the server will catch up later.
						</p>
					{/if}
					<div class="mt-3 flex flex-wrap gap-2">
						<button
							class="rounded-xl bg-white px-3 py-2 text-sm font-semibold text-black"
							onclick={() => walk.markArrived()}>I'm here</button
						>
						<a
							class="rounded-xl border border-white/20 px-3 py-2 text-sm hover:bg-white/10"
							href={mapsURL(next.place.latitude, next.place.longitude, mode, ua)}
							target="_blank"
							rel="noreferrer">Directions</a
						>
					</div>
				</div>
			{:else}
				<div class="rounded-2xl border border-white/10 bg-black/85 p-4 backdrop-blur">
					<div class="text-[11px] font-bold tracking-widest text-emerald-300 uppercase">
						Tour complete
					</div>
					<div class="text-lg font-semibold">You walked every stop of {walk.tour?.title}.</div>
					{#if !$current}
						<p class="mt-1 text-sm text-white/60">Sign in to keep this on your account.</p>
					{/if}
					<button
						class="mt-3 rounded-xl bg-white px-3 py-2 text-sm font-semibold text-black"
						onclick={() => end(false)}>Done</button
					>
				</div>
			{/if}
		{/if}
	</div>

	{#if walk.arrivedStop}
		<ArrivalPanel
			stop={walk.arrivedStop}
			isLast={walk.isComplete}
			{playing}
			onlisten={() => toggleListen(walk.arrivedStop!)}
			oncontinue={() => {
				stopAudio();
				walk.arrivedStop = null;
			}}
		/>
	{/if}

	{#if showEnd}
		<div
			class="absolute inset-0 z-30 flex items-end justify-center bg-black/60 p-4 sm:items-center"
		>
			<div
				class="w-full max-w-sm rounded-3xl border border-white/10 bg-[#0a0a0b] p-5"
				role="dialog"
				use:modal={() => {
					showEnd = false;
				}}
				aria-label="End this tour?"
			>
				<h2 class="text-lg font-semibold">End this tour?</h2>
				<p class="mt-1 text-sm text-white/60">{walk.progressText}</p>
				<div class="mt-4 grid gap-2">
					<button
						class="rounded-xl bg-white px-3 py-2 text-sm font-semibold text-black"
						onclick={() => end(false)}>Keep my progress and end</button
					>
					<button
						class="rounded-xl border border-red-400/50 px-3 py-2 text-sm text-red-200"
						onclick={() => end(true)}>Start over next time</button
					>
					<button
						class="rounded-xl border border-white/20 px-3 py-2 text-sm"
						onclick={() => (showEnd = false)}>Cancel</button
					>
				</div>
			</div>
		</div>
	{/if}
</div>
