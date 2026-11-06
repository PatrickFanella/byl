<script lang="ts">
	import { page } from '$app/state';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getMoment, type MomentDetail } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import MomentStory from '#lib/components/visitor/MomentStory.svelte';

	const id = $derived(page.params.id ?? '');
	const moment = new Loader<MomentDetail>(() => getMoment(id));

	$effect(() => {
		void id;
		moment.run();
	});
</script>

<svelte:head><title>{moment.data?.title ?? 'Moment'} · Byl</title></svelte:head>

<LoadState status={moment.status} message={moment.message} onretry={() => moment.run()}>
	{#if moment.data}
		{@const m = moment.data}
		<a href="/places/{m.place_id}" class="text-sm text-byl-muted hover:underline">← Place</a>
		{#if m.date_display}
			<div class="mt-3 text-xs font-bold tracking-normal text-byl-action">
				{m.date_display}
			</div>
		{/if}
		<h1 class="text-3xl font-semibold">{m.title}</h1>
		{#if m.has_narration || m.narration}
			<p class="mt-1 text-xs text-byl-muted">Audio guide available on the tour</p>
		{/if}

		<MomentStory moment={m} />
	{/if}
</LoadState>
