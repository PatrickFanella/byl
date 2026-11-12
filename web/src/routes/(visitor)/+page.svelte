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

<svelte:head>
	<title>Byl · Local history, on foot or by bike</title>
	<meta name="description" content="Local history, on foot or by bike." />
</svelte:head>

<section class="byl-opening space-y-4">
	<p class="byl-eyebrow">Local history, on foot or by bike</p>
	<h1 class="byl-display">
		{city.current ? `${city.current.name} on foot` : 'Your city on foot'}
	</h1>
	<p class="byl-lede">
		Walk or ride through the stories that happened right where you are standing. Each tour follows
		one topic through a handful of places, with stories and sources at each stop.
	</p>
	<div class="flex flex-wrap items-center gap-3 pt-2">
		<a href="/tours" class="byl-btn byl-btn-primary">Browse tours</a>
		<CityPicker />
	</div>
</section>

{#if city.status === 'error'}
	<div class="mt-6">
		<LoadState status="error" message={city.message} onretry={() => city.load()} />
	</div>
{:else}
	<div class="byl-home-grid">
		<section class="byl-section space-y-6">
			<SectionHeader title="Tours" eyebrow="By topic" />
			<LoadState
				status={city.status === 'ready' ? tours.status : 'loading'}
				message={tours.message}
				emptyTitle="No tours yet"
				emptyMessage="Curated tours for this city are still being written."
				onretry={() => tours.run()}
			>
				<div class="space-y-8">
					{#each groups as [key, group] (key)}
						<section class="space-y-3">
							<h3 class="byl-h3">{group.name}</h3>
							<div class="grid gap-x-6 gap-y-4 sm:grid-cols-2">
								{#each group.tours as tour (tour.id)}<TourCard {tour} showTopic={false} />{/each}
							</div>
						</section>
					{/each}
				</div>
			</LoadState>
		</section>

		<section class="byl-section space-y-6">
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
					<ul class="byl-rows">
						{#each (nearby.data ?? []).slice(0, 8) as place (place.id)}
							<li>
								<a href="/places/{place.id}" class="byl-row items-center">
									<span class="flex-1">
										<span class="byl-row-title">{place.name}</span>
										<span class="byl-meta">
											{place.distance_m != null
												? `${distanceText(place.distance_m)} · `
												: ''}{place.moment_count}
											{place.moment_count === 1 ? 'moment' : 'moments'}
										</span>
									</span>
									<span class="text-byl-muted" aria-hidden="true">›</span>
								</a>
							</li>
						{/each}
					</ul>
				</LoadState>
			{/if}
		</section>
	</div>
{/if}
