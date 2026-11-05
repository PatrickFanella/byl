<script lang="ts">
	import { page } from '$app/state';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getPlace, type PlaceDetail } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import MomentCard from '#lib/components/visitor/MomentCard.svelte';
	import SectionHeader from '#lib/components/visitor/SectionHeader.svelte';

	const ref = $derived(page.params.ref ?? '');
	const featuredID = $derived(page.url.searchParams.get('featured'));
	const place = new Loader<PlaceDetail>(() => getPlace(ref));

	$effect(() => {
		void ref;
		place.run();
	});

	const featured = $derived.by(() => {
		const p = place.data;
		if (!p) return null;
		return p.moments.find((m) => m.id === featuredID) ?? p.primary_moment ?? null;
	});
	const others = $derived((place.data?.moments ?? []).filter((m) => m.id !== featured?.id));
</script>

<svelte:head><title>{place.data?.name ?? 'Place'} · Invisible Museum</title></svelte:head>

<LoadState status={place.status} message={place.message} onretry={() => place.run()}>
	{#if place.data}
		{@const p = place.data}
		<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">
			{p.kind.replace(/_/g, ' ')}
		</div>
		<h1 class="text-3xl font-semibold">{p.name}</h1>
		{#if p.address}<p class="mt-1 text-sm text-white/60">{p.address}</p>{/if}
		{#if p.summary}<p class="mt-3 max-w-2xl text-white/80">{p.summary}</p>{/if}

		{#if featured}
			<section class="mt-6 space-y-3">
				<SectionHeader
					title={featured.title}
					eyebrow={featuredID ? 'Featured on this tour' : 'Primary moment'}
				/>
				<MomentCard moment={featured} prominent testid="featured-moment" />
			</section>
		{/if}
		{#if others.length}
			<section class="mt-6 space-y-3">
				<SectionHeader
					title={featured ? 'Also at this place' : 'Moments here'}
					eyebrow="{others.length} more"
				/>
				{#each others as m (m.id)}<MomentCard moment={m} />{/each}
			</section>
		{/if}
		{#if !featured && others.length === 0}
			<div class="mt-6">
				<LoadState
					status="empty"
					emptyTitle="Nothing published here yet"
					emptyMessage="Curators are still writing this place's moments."
				/>
			</div>
		{/if}
	{/if}
</LoadState>
