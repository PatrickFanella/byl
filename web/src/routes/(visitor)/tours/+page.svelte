<script lang="ts">
	import { city } from '#lib/city.svelte.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import { listTours, type TourSummary } from '#lib/visitor.ts';
	import CityPicker from '#lib/components/visitor/CityPicker.svelte';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import TourCard from '#lib/components/visitor/TourCard.svelte';

	let topic = $state('');
	let mode = $state('');

	const tours = new Loader<TourSummary[]>(
		async () =>
			city.current
				? (await listTours(city.current.slug, topic || undefined, mode || undefined)).items
				: [],
		(v) => v.length === 0
	);

	$effect(() => {
		city.load();
	});
	$effect(() => {
		// re-run when the city or a filter changes
		void topic;
		void mode;
		if (city.current) tours.run();
	});

	const chip = (on: boolean) =>
		`rounded-full px-3 py-1.5 text-sm ${on ? 'bg-white text-black' : 'border border-white/20 text-white/80 hover:bg-white/10'}`;
</script>

<svelte:head><title>Tours · Invisible Museum</title></svelte:head>

<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">
	{city.current?.name ?? 'Tours'}
</div>
<h1 class="text-3xl font-semibold">Tours</h1>
<p class="mt-1 max-w-2xl text-white/60">
	Each tour is a gallery: one topic, a handful of places, and the moment at each that belongs to the
	story.
</p>
<div class="mt-3"><CityPicker /></div>

<div class="mt-5 space-y-3">
	{#if city.current?.topics?.length}
		<div class="flex flex-wrap gap-2">
			<button class={chip(topic === '')} onclick={() => (topic = '')}>All topics</button>
			{#each city.current.topics as t (t.id)}
				<button
					class={chip(topic === t.slug)}
					onclick={() => (topic = topic === t.slug ? '' : t.slug)}>{t.name}</button
				>
			{/each}
		</div>
	{/if}
	<div class="flex gap-2">
		<button class={chip(mode === '')} onclick={() => (mode = '')}>Any</button>
		<button class={chip(mode === 'walk')} onclick={() => (mode = mode === 'walk' ? '' : 'walk')}
			>Walking</button
		>
		<button class={chip(mode === 'bike')} onclick={() => (mode = mode === 'bike' ? '' : 'bike')}
			>Biking</button
		>
	</div>
</div>

<div class="mt-6">
	<LoadState
		status={city.status === 'ready' ? tours.status : city.status === 'error' ? 'error' : 'loading'}
		message={tours.message || city.message}
		emptyTitle="No tours match"
		emptyMessage={topic || mode
			? 'Try another topic or travel mode.'
			: 'Curated tours for this city are still being written.'}
		onretry={() => tours.run()}
	>
		<div class="grid gap-4 sm:grid-cols-2">
			{#each tours.data ?? [] as tour (tour.id)}<TourCard {tour} />{/each}
		</div>
	</LoadState>
</div>
