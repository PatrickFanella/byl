<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getTour, type TourDetail } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import RouteBuilderForm from '#lib/components/visitor/RouteBuilderForm.svelte';

	const id = $derived(page.params.id ?? '');
	const tour = new Loader<TourDetail>(() => getTour(id));
	$effect(() => {
		void id;
		tour.run();
	});
</script>

<svelte:head><title>Edit route · Invisible Museum</title></svelte:head>

<a href="/routes" class="text-sm text-white/50 hover:underline">← Your routes</a>
<h1 class="mt-2 text-3xl font-semibold">Edit route</h1>
<div class="mt-5">
	<LoadState status={tour.status} message={tour.message} onretry={() => tour.run()}>
		{#if tour.data}<RouteBuilderForm editing={tour.data} ondone={() => goto('/routes')} />{/if}
	</LoadState>
</div>
