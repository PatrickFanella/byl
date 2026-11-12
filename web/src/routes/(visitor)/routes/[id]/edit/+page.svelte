<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getTour, type TourDetail } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import RouteBuilderForm from '#lib/components/visitor/RouteBuilderForm.svelte';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import { session } from '#lib/auth.ts';

	const current = session.store;

	const id = $derived(page.params.id ?? '');
	const tour = new Loader<TourDetail>(() => getTour(id));
	$effect(() => {
		void id;
		tour.run();
	});
</script>

<svelte:head><title>Edit route · Byl</title></svelte:head>

<a href="/routes" class="text-sm text-byl-muted hover:underline">← Your routes</a>
<h1 class="byl-title mt-2">Edit route</h1>
<div class="mt-5">
	{#if !$current}
		<SignInPrompt reason="Sign in to build routes that follow you to every device." />
	{:else}
		<LoadState status={tour.status} message={tour.message} onretry={() => tour.run()}>
			{#if tour.data}<RouteBuilderForm editing={tour.data} ondone={() => goto('/routes')} />{/if}
		</LoadState>
	{/if}
</div>
