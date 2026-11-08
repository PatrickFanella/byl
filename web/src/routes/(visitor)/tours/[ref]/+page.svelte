<script lang="ts">
	import { page } from '$app/state';
	import { Loader } from '#lib/loader.svelte.ts';
	import { session } from '#lib/auth.ts';
	import { ApiError } from '#lib/api/index.ts';
	import {
		getRoute,
		getTour,
		getTourRevision,
		mySaved,
		saveTour,
		unsaveTour,
		type TourDetail,
		type TourRevision,
		type TourRoute
	} from '#lib/visitor.ts';
	import { distanceText, estimateText, modeLabel } from '#lib/tour/estimate.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import SectionHeader from '#lib/components/visitor/SectionHeader.svelte';
	import StopRow from '#lib/components/visitor/StopRow.svelte';
	import TourCard from '#lib/components/visitor/TourCard.svelte';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import Map from '#lib/components/Map.svelte';
	import { validTransitSelection } from '#lib/tour/transit.ts';
	import TransitPanel from '#lib/components/visitor/TransitPanel.svelte';
	import JourneySummary from '#lib/components/visitor/JourneySummary.svelte';

	const current = session.store;
	const ref = $derived(page.params.ref ?? '');
	const tour = new Loader<TourDetail>(() => getTour(ref));
	let route = $state<TourRoute | null>(null);
	let retained = $state<TourRevision | null>(null);
	let saved = $state(false);
	let saveBusy = $state(false);
	const mixedReady = $derived(
		!tour.data?.transit || (!!retained && validTransitSelection(retained))
	);
	let saveError = $state('');
	let wantSave = $state(false);

	$effect(() => {
		void ref;
		tour.run();
	});
	$effect(() => {
		const t = tour.data;
		if (!t) return;
		route = null;
		retained = null;
		const mode = t.transit ? 'walk' : t.modes.find((m) => t.routes?.[m]?.status === 'ready');
		if (!mode) {
			route = null;
			return;
		}
		(t.revision_id
			? getTourRevision(t.id, t.revision_id).then((r) => {
					if (tour.data === t) retained = r;
					return r.snapshot?.routes[mode] ?? null;
				})
			: getRoute(t.id, mode)
		)
			.then((r) => {
				if (tour.data === t) route = r;
			})
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
			lat: s.place.arrival_point?.point?.latitude ?? s.place.latitude,
			lng: s.place.arrival_point?.point?.longitude ?? s.place.longitude,
			label: `${s.position}. ${s.place.name}`
		}))
	);
	const line = $derived((route?.geometry?.coordinates ?? []) as [number, number][]);
</script>

<svelte:head><title>{tour.data?.title ?? 'Tour'} · Byl</title></svelte:head>

<a href="/tours" class="text-sm text-byl-muted hover:underline">← Tours</a>

<div class="mt-3">
	<LoadState status={tour.status} message={tour.message} onretry={() => tour.run()}>
		{#if tour.data}
			{@const t = tour.data}
			<div class="grid gap-6 lg:grid-cols-3">
				<div class="space-y-5 lg:col-span-2">
					<TourCard tour={t} href={`/tours/${t.slug}`} />
					<h1 class="sr-only">{t.title}</h1>
					{#if t.status !== 'published'}<p class="rounded-lg border border-byl-border p-3 text-sm">
							Draft · Only you can open this route. Its path needs review before publication or
							sharing.
						</p>{/if}
					{#if t.summary}<p class="text-byl-muted">{t.summary}</p>{/if}

					{#if !t.transit || t.modes.some((m) => m !== 'walk')}
						<div class="rounded-lg border border-byl-border bg-byl-surface p-4">
							<div class="text-xs font-bold tracking-normal text-byl-action">Getting around</div>
							<ul class="mt-2 space-y-1 text-sm">
								{#each t.modes.filter((m) => !t.transit || m !== 'walk') as mode (mode)}
									{@const r = t.routes?.[mode]}
									<li class="flex justify-between gap-3">
										<span class="font-semibold">{modeLabel(mode)}</span>
										<span class="text-byl-muted">
											{#if estimateText(r)}
												{estimateText(r)} · {distanceText(r?.distance_m ?? 0)}
											{:else if r?.status === 'failed'}
												<span class="text-byl-danger">Route unavailable</span>
											{:else}
												Route being computed
											{/if}
										</span>
									</li>
								{/each}
							</ul>
							{#each ['walk', 'bike'].filter((m) => !t.modes.includes(m as 'walk' | 'bike')) as unavailable (unavailable)}<p
									class="mt-2 text-sm text-byl-muted"
								>
									{modeLabel(unavailable)} is not offered on this revision.
								</p>{/each}
							{#each t.modes.filter((m) => !t.transit || m !== 'walk') as mode (mode)}<JourneySummary
									{mode}
									journey={t.journeys?.[mode]}
								/>{/each}
						</div>
					{/if}
					{#if retained?.snapshot?.tour.transit && mixedReady}
						{#key `${retained.id}:${$current?.account.id ?? 'anonymous'}`}<TransitPanel
								revision={retained}
							/>{/key}
					{/if}
					{#if t.transit && !mixedReady}<p role="status" class="text-sm text-byl-warning">
							The retained scheduled connection is unavailable or unsupported. Reconnect to review
							it before opening this mixed trip.
						</p>{/if}
					<div class="flex flex-wrap gap-2">
						{#if t.status === 'published'}
							{#each t.modes as mode (mode)}
								{#if t.journeys?.[mode]?.state !== 'blocked' && mixedReady}
									<a
										href="/tours/{t.slug}/walk?mode={mode}"
										class="rounded-lg px-4 py-2 text-sm font-semibold {mode === t.modes[0]
											? 'bg-byl-action text-byl-action-text'
											: 'border border-byl-border hover:bg-byl-surface'}"
										>{t.transit && mode === 'walk'
											? 'Open mixed trip'
											: mode === 'bike'
												? 'Start riding'
												: 'Start walking'}</a
									>
								{/if}
							{/each}
							<button
								class="rounded-lg border border-byl-border px-4 py-2 text-sm hover:bg-byl-surface disabled:opacity-50"
								onclick={toggleSave}
								disabled={saveBusy}>{saved ? '♥ Saved' : '♡ Save for later'}</button
							>
							{#if !t.transit}<a
									href="invisiblemuseum://tour/{t.slug}"
									class="rounded-lg border border-byl-border px-4 py-2 text-sm text-byl-muted hover:bg-byl-surface"
									>Open in the app</a
								>{/if}
						{/if}
					</div>
					{#if t.revision_id}<p class="text-sm">
							<a
								class="text-byl-action underline"
								href={`/tours/${encodeURIComponent(t.id)}/report?revision=${encodeURIComponent(t.revision_id)}&mode=${t.modes[0] ?? 'walk'}`}
								>Suggest a route change</a
							>
						</p>{/if}
					{#if wantSave && !$current}
						<SignInPrompt reason="Sign in to save tours and keep your progress on every device." />
					{/if}
					{#if saveError}<p class="text-sm text-byl-danger">{saveError}</p>{/if}

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
					<div class="overflow-hidden rounded-lg border border-byl-border">
						<Map
							markers={stopMarkers}
							line={t.transit ? [] : line}
							legs={(t.transit?.walking_legs ?? []).map((l) => ({
								coords: l.geometry as [number, number][],
								state: 'future' as const
							}))}
							height="24rem"
						/>
					</div>
					{#if t.transit}<p class="text-xs text-byl-muted">
							Only retained ordinary walking sections are drawn. Station walks need an explicit
							directions handoff; the rail ride is not a walking line.
						</p>{:else if !route}
						<p class="text-xs text-byl-muted">
							The route line appears once the route has been computed.
						</p>
					{/if}
				</aside>
			</div>
		{/if}
	</LoadState>
</div>
