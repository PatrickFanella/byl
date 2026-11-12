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
</script>

<svelte:head><title>Tours · Byl</title></svelte:head>

<div class="byl-eyebrow">
	{city.current?.name ?? 'Tours'}
</div>
<h1 class="byl-title">Tours</h1>
<p class="byl-lede mt-3">
	Each tour is a gallery: one topic, a handful of places, and the moment at each that belongs to the
	story.
</p>
<div class="mt-3"><CityPicker /></div>

<div class="mt-8 space-y-3">
	{#if city.current?.topics?.length}
		<div class="flex flex-wrap gap-2">
			<button class="byl-chip" aria-pressed={topic === ''} onclick={() => (topic = '')}
				>All topics</button
			>
			{#each city.current.topics as t (t.id)}
				<button
					class="byl-chip"
					aria-pressed={topic === t.id}
					onclick={() => (topic = topic === t.id ? '' : t.id)}>{t.name}</button
				>
			{/each}
		</div>
	{/if}
	<div class="flex gap-2">
		<button class="byl-chip" aria-pressed={mode === ''} onclick={() => (mode = '')}>Any</button>
		<button
			class="byl-chip"
			aria-pressed={mode === 'walk'}
			onclick={() => (mode = mode === 'walk' ? '' : 'walk')}>Walking</button
		>
		<button
			class="byl-chip"
			aria-pressed={mode === 'bike'}
			onclick={() => (mode = mode === 'bike' ? '' : 'bike')}>Biking</button
		>
	</div>
</div>

<div class="mt-8">
	<LoadState
		status={city.status === 'ready' ? tours.status : city.status === 'error' ? 'error' : 'loading'}
		message={tours.message || city.message}
		emptyTitle="No tours match"
		emptyMessage={topic || mode
			? 'Try another topic or travel mode.'
			: 'Curated tours for this city are still being written.'}
		onretry={() => tours.run()}
	>
		<div class="grid gap-x-6 gap-y-4 sm:grid-cols-2">
			{#each tours.data ?? [] as tour (tour.id)}<TourCard {tour} />{/each}
		</div>
	</LoadState>
</div>
