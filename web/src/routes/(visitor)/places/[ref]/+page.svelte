<script lang="ts">
	import { page } from '$app/state';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getPlace, getMoment, type PlaceDetail, type MomentDetail } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import MomentStory from '#lib/components/visitor/MomentStory.svelte';
	import MediaFigure from '#lib/components/visitor/MediaFigure.svelte';
	import MediaGallery from '#lib/components/visitor/MediaGallery.svelte';
	import SectionHeader from '#lib/components/visitor/SectionHeader.svelte';
	import HappeningsPanel from '#lib/components/visitor/HappeningsPanel.svelte';

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
		return p.moments.find((m) => m.id === featuredID) ?? p.primary_moment ?? p.moments[0] ?? null;
	});
	const story = new Loader<MomentDetail>(() => getMoment(featured?.id ?? ''));
	$effect(() => {
		if (featured?.id) story.run();
	});
	const hero = $derived(
		[featured?.hero_media, ...(place.data?.moments.map((moment) => moment.hero_media) ?? [])].find(
			(asset) => asset?.kind === 'image' && !asset.external
		)
	);
</script>

<svelte:head><title>{place.data?.name ?? 'Place'} · Byl</title></svelte:head>

<LoadState status={place.status} message={place.message} onretry={() => place.run()}>
	{#if place.data}
		{@const p = place.data}
		<div class="byl-eyebrow">
			{p.kind.replace(/_/g, ' ')}
		</div>
		<h1 class="byl-title">{p.name}</h1>
		{#if p.address}<p class="mt-1 text-sm text-byl-muted">{p.address}</p>{/if}
		{#if p.summary}<p class="byl-lede mt-3">{p.summary}</p>{/if}
		<div class="mt-6"><HappeningsPanel placeID={p.id} /></div>
		{#if hero}<div class="mt-6"><MediaFigure asset={hero} /></div>{/if}

		{#if p.moments.length > 1}
			<nav aria-label="Events and stories at this place" class="byl-section max-w-[66ch]">
				<SectionHeader title="Events and stories" eyebrow="At this place" />
				<ol class="byl-rows mt-4">
					{#each p.moments as moment (moment.id)}
						<li>
							<a
								href="?featured={moment.id}#story"
								aria-current={featured?.id === moment.id ? 'true' : undefined}
								class="byl-row flex-col gap-1 hover:underline sm:flex-row sm:gap-6"
							>
								<span class="text-sm text-byl-muted sm:w-32 sm:shrink-0"
									>{moment.date_display || 'Date not recorded'}</span
								>
								<span class="font-semibold text-byl-action"
									>{moment.title}{#if featured?.id === moment.id}<span class="sr-only">
											(selected)</span
										>{/if}</span
								>
							</a>
						</li>
					{/each}
				</ol>
			</nav>
		{/if}

		{#if featured}
			<section id="story" class="byl-section scroll-mt-6 space-y-3" aria-label="Selected history">
				<SectionHeader
					title={featured.title}
					eyebrow={featured.date_display || 'History and context'}
				/>
				<LoadState status={story.status} message={story.message} onretry={() => story.run()}>
					{#if story.data?.id === featured.id}<MomentStory
							moment={story.data}
							omitMediaId={hero?.id}
						/>{/if}
				</LoadState>
			</section>
		{/if}
		{#if p.media?.length}
			<section class="byl-section space-y-4" aria-label="From this place">
				<SectionHeader title="From this place" eyebrow="Photos and records" />
				<MediaGallery media={p.media.filter((asset) => asset.id !== hero?.id)} />
			</section>
		{/if}
		{#if !featured}
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
