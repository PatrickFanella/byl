<script lang="ts">
	import type { TourSummary } from '#lib/visitor.ts';
	import { estimateText } from '#lib/tour/estimate.ts';

	let {
		tour,
		href = `/tours/${tour.slug}`,
		showTopic = true
	}: { tour: TourSummary; href?: string; showTopic?: boolean } = $props();
	const hero = $derived(tour.hero_media?.public_url ?? null);
	const estimates = $derived(
		tour.modes.map((m) => ({ mode: m, text: estimateText(tour.routes?.[m]) })).filter((e) => e.text)
	);
</script>

<a {href} class="byl-tour text-left">
	{#if hero}
		<img src={hero} alt="" class="byl-thumb" />
	{/if}
	<span class="min-w-0 flex-1">
		{#if showTopic && tour.topic}
			<span class="byl-eyebrow block">{tour.topic.name}</span>
		{/if}
		<span class="byl-tour-title">{tour.title}</span>
		<span class="byl-meta mt-1">
			<span>{tour.stop_count} stops</span>
			{#each estimates as e (e.mode)}<span>{e.text}</span>{/each}
			{#if tour.owner_account_id}<span>visitor route</span>{/if}
		</span>
	</span>
</a>
