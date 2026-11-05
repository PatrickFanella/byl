<script lang="ts">
	import { SvelteMap } from 'svelte/reactivity';
	import { city } from '#lib/city.svelte.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import { listPlacesNear, listTours, type PlaceSummary, type TourSummary } from '#lib/visitor.ts';
	import { distanceText } from '#lib/tour/estimate.ts';
	import CityPicker from '#lib/components/visitor/CityPicker.svelte';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import SectionHeader from '#lib/components/visitor/SectionHeader.svelte';
	import TourCard from '#lib/components/visitor/TourCard.svelte';

	let position: { lat: number; lng: number } | null = $state(null);
	let geoState: 'unknown' | 'denied' | 'unavailable' | 'ok' = $state('unknown');

	const tours = new Loader<TourSummary[]>(
		async () => (city.current ? (await listTours(city.current.slug)).items : []),
		(v) => v.length === 0
	);
	const groups = $derived.by(() => {
		const grouped = new SvelteMap<string, { name: string; tours: TourSummary[] }>();
		for (const tour of tours.data ?? []) {
			const key = tour.topic?.id ?? 'other';
			if (!grouped.has(key))
				grouped.set(key, { name: tour.topic?.name ?? 'Other stories', tours: [] });
			grouped.get(key)!.tours.push(tour);
		}
		return [...grouped.entries()];
	});

	const nearby = new Loader<PlaceSummary[]>(
		async () =>
			city.current && position
				? (await listPlacesNear(city.current.slug, position.lat, position.lng)).items
				: [],
		(v) => v.length === 0
	);

	$effect(() => {
		city.load();
	});
	$effect(() => {
		if (city.current) tours.run();
	});
	$effect(() => {
		if (city.current && position) nearby.run();
	});
	$effect(() => {
		if (typeof navigator === 'undefined' || !('geolocation' in navigator)) {
			geoState = 'unavailable';
			return;
		}
		navigator.geolocation.getCurrentPosition(
			(p) => {
				position = { lat: p.coords.latitude, lng: p.coords.longitude };
				geoState = 'ok';
			},
			(err) => {
				geoState = err.code === err.PERMISSION_DENIED ? 'denied' : 'unavailable';
			},
			{ maximumAge: 60_000, timeout: 10_000 }
		);
	});
</script>

<svelte:head><title>Byl</title></svelte:head>

<section class="space-y-3">
	<div class="text-xs font-bold tracking-normal text-byl-action">Byl</div>
	<h1 class="text-3xl font-semibold">
		{city.current ? `${city.current.name} on foot` : 'Your city on foot'}
	</h1>
	<p class="max-w-2xl text-byl-muted">
		Walk or ride through the stories that happened right where you are standing. Each tour follows
		one topic through a handful of places, with stories and sources at each stop.
	</p>
	<CityPicker />
</section>

{#if city.status === 'error'}
	<div class="mt-6">
		<LoadState status="error" message={city.message} onretry={() => city.load()} />
	</div>
{:else}
	<section class="mt-8 space-y-4">
		<SectionHeader title="Tours" eyebrow="By topic" />
		<LoadState
			status={city.status === 'ready' ? tours.status : 'loading'}
			message={tours.message}
			emptyTitle="No tours yet"
			emptyMessage="Curated tours for this city are still being written."
			onretry={() => tours.run()}
		>
			<div class="space-y-6">
				{#each groups as [key, group] (key)}
					<section class="space-y-3">
						<h3 class="text-lg font-semibold">{group.name}</h3>
						<div class="grid gap-4 sm:grid-cols-2">
							{#each group.tours as tour (tour.id)}<TourCard {tour} />{/each}
						</div>
					</section>
				{/each}
			</div>
		</LoadState>
	</section>

	<section class="mt-8 space-y-4">
		<SectionHeader title="Near you" eyebrow="Places" />
		{#if geoState === 'denied'}
			<LoadState
				status="empty"
				emptyTitle="Location is off"
				emptyMessage="Allow location access in your browser to see the places around you."
			/>
		{:else if geoState === 'unavailable'}
			<LoadState
				status="empty"
				emptyTitle="Location unavailable"
				emptyMessage="Pick a tour to travel to its first stop."
			/>
		{:else if !position}
			<LoadState status="loading" loadingText="Waiting for your location…" />
		{:else}
			<LoadState
				status={nearby.status}
				message={nearby.message}
				emptyTitle="Nothing within 2 km"
				emptyMessage="Pick a tour to travel to its first stop."
				onretry={() => nearby.run()}
			>
				<ul class="divide-y divide-byl-border rounded-lg border border-byl-border bg-byl-surface">
					{#each (nearby.data ?? []).slice(0, 8) as place (place.id)}
						<li>
							<a
								href="/places/{place.id}"
								class="flex items-center gap-3 px-4 py-3 hover:bg-byl-surface"
							>
								<span class="flex-1">
									<span class="block font-semibold">{place.name}</span>
									<span class="block text-xs text-byl-muted">
										{place.distance_m != null
											? `${distanceText(place.distance_m)} · `
											: ''}{place.moment_count}
										{place.moment_count === 1 ? 'moment' : 'moments'}
									</span>
								</span>
								<span class="text-byl-muted">›</span>
							</a>
						</li>
					{/each}
				</ul>
			</LoadState>
		{/if}
	</section>
{/if}
