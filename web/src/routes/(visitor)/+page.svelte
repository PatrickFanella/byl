<script lang="ts">
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

<svelte:head><title>Invisible Museum</title></svelte:head>

<section class="space-y-3">
	<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">Invisible Museum</div>
	<h1 class="text-3xl font-semibold">
		{city.current ? `${city.current.name} as a museum` : 'Your city as a museum'}
	</h1>
	<p class="max-w-2xl text-white/60">
		Walk or ride through the stories that happened right where you are standing. Each tour is a
		gallery: one topic, a handful of places, and the moment at each that belongs to the story.
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
			<div class="grid gap-4 sm:grid-cols-2">
				{#each tours.data ?? [] as tour (tour.id)}<TourCard {tour} />{/each}
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
				<ul class="divide-y divide-white/10 rounded-2xl border border-white/10 bg-white/5">
					{#each (nearby.data ?? []).slice(0, 8) as place (place.id)}
						<li>
							<a
								href="/places/{place.id}"
								class="flex items-center gap-3 px-4 py-3 hover:bg-white/10"
							>
								<span class="flex-1">
									<span class="block font-semibold">{place.name}</span>
									<span class="block text-xs text-white/60">
										{place.distance_m != null
											? `${distanceText(place.distance_m)} · `
											: ''}{place.moment_count}
										{place.moment_count === 1 ? 'moment' : 'moments'}
									</span>
								</span>
								<span class="text-white/40">›</span>
							</a>
						</li>
					{/each}
				</ul>
			</LoadState>
		{/if}
	</section>
{/if}
