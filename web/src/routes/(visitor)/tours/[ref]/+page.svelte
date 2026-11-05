<script lang="ts">
	import { page } from '$app/state';
	import { Loader } from '#lib/loader.svelte.ts';
	import { session } from '#lib/auth.ts';
	import { ApiError } from '#lib/api/index.ts';
	import {
		getRoute,
		getTour,
		mySaved,
		saveTour,
		unsaveTour,
		type TourDetail,
		type TourRoute
	} from '#lib/visitor.ts';
	import { distanceText, estimateText, modeLabel } from '#lib/tour/estimate.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import SectionHeader from '#lib/components/visitor/SectionHeader.svelte';
	import StopRow from '#lib/components/visitor/StopRow.svelte';
	import TourCard from '#lib/components/visitor/TourCard.svelte';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import Map from '#lib/components/Map.svelte';

	const current = session.store;
	const ref = $derived(page.params.ref ?? '');
	const tour = new Loader<TourDetail>(() => getTour(ref));
	let route = $state<TourRoute | null>(null);
	let saved = $state(false);
	let saveBusy = $state(false);
	let saveError = $state('');
	let wantSave = $state(false);

	$effect(() => {
		void ref;
		tour.run();
	});
	$effect(() => {
		const t = tour.data;
		if (!t) return;
		const mode = t.modes.find((m) => t.routes?.[m]?.status === 'ready');
		if (!mode) {
			route = null;
			return;
		}
		getRoute(t.id, mode)
			.then((r) => (route = r))
			.catch(() => (route = null));
	});
	$effect(() => {
		const t = tour.data;
		if (!t || !$current) return;
		mySaved()
			.then((p) => (saved = p.items.some((s) => s.id === t.id)))
			.catch(() => {});
	});

	async function toggleSave() {
		const t = tour.data;
		if (!t) return;
		if (!$current) {
			wantSave = true;
			return;
		}
		saveBusy = true;
		saveError = '';
		try {
			if (saved) await unsaveTour(t.id);
			else await saveTour(t.id);
			saved = !saved;
		} catch (e) {
			saveError = e instanceof ApiError ? e.message : 'Could not save.';
		} finally {
			saveBusy = false;
		}
	}

	const stopMarkers = $derived(
		(tour.data?.stops ?? []).map((s) => ({
			lat: s.place.latitude,
			lng: s.place.longitude,
			label: `${s.position}. ${s.place.name}`
		}))
	);
	const line = $derived((route?.geometry?.coordinates ?? []) as [number, number][]);
</script>

<svelte:head><title>{tour.data?.title ?? 'Tour'} · Invisible Museum</title></svelte:head>

<a href="/tours" class="text-sm text-white/50 hover:underline">← Tours</a>

<div class="mt-3">
	<LoadState status={tour.status} message={tour.message} onretry={() => tour.run()}>
		{#if tour.data}
			{@const t = tour.data}
			<div class="grid gap-6 lg:grid-cols-3">
				<div class="space-y-5 lg:col-span-2">
					<TourCard tour={t} href={`/tours/${t.slug}`} />
					<h1 class="sr-only">{t.title}</h1>
					{#if t.summary}<p class="text-white/70">{t.summary}</p>{/if}

					<div class="rounded-2xl border border-white/10 bg-white/5 p-4">
						<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">
							Getting around
						</div>
						<ul class="mt-2 space-y-1 text-sm">
							{#each t.modes as mode (mode)}
								{@const r = t.routes?.[mode]}
								<li class="flex justify-between gap-3">
									<span class="font-semibold">{modeLabel(mode)}</span>
									<span class="text-white/60">
										{#if estimateText(r)}
											{estimateText(r)} · {distanceText(r?.distance_m ?? 0)}
										{:else if r?.status === 'failed'}
											<span class="text-red-300">Route unavailable</span>
										{:else}
											Route being computed
										{/if}
									</span>
								</li>
							{/each}
						</ul>
					</div>

					<div class="flex flex-wrap gap-2">
						{#each t.modes as mode (mode)}
							<a
								href="/tours/{t.slug}/walk?mode={mode}"
								class="rounded-xl px-4 py-2 text-sm font-semibold {mode === t.modes[0]
									? 'bg-white text-black'
									: 'border border-white/20 hover:bg-white/10'}"
								>{mode === 'bike' ? 'Start riding' : 'Start walking'}</a
							>
						{/each}
						<button
							class="rounded-xl border border-white/20 px-4 py-2 text-sm hover:bg-white/10 disabled:opacity-50"
							onclick={toggleSave}
							disabled={saveBusy}>{saved ? '♥ Saved' : '♡ Save for later'}</button
						>
						<a
							href="invisiblemuseum://tour/{t.slug}"
							class="rounded-xl border border-white/20 px-4 py-2 text-sm text-white/70 hover:bg-white/10"
							>Open in the app</a
						>
					</div>
					{#if wantSave && !$current}
						<SignInPrompt reason="Sign in to save tours and keep your progress on every device." />
					{/if}
					{#if saveError}<p class="text-sm text-red-300">{saveError}</p>{/if}

					<section class="space-y-3">
						<SectionHeader title="Stops" eyebrow="{t.stops.length} places" />
						{#if t.stops.length === 0}
							<LoadState
								status="empty"
								emptyTitle="No stops yet"
								emptyMessage="This tour has no published places."
							/>
						{/if}
						{#each t.stops as stop (stop.id)}<StopRow {stop} />{/each}
					</section>
				</div>
				<aside class="space-y-3">
					<div class="overflow-hidden rounded-2xl border border-white/10">
						<Map markers={stopMarkers} {line} height="24rem" />
					</div>
					{#if !route}
						<p class="text-xs text-white/50">
							The route line appears once the route has been computed.
						</p>
					{/if}
				</aside>
			</div>
		{/if}
	</LoadState>
</div>
