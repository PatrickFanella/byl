<script lang="ts">
	import type { TourSummary } from '#lib/visitor.ts';
	import { estimateText } from '#lib/tour/estimate.ts';

	let { tour, href = `/tours/${tour.slug}` }: { tour: TourSummary; href?: string } = $props();
	const hero = $derived(tour.hero_media?.public_url ?? null);
	const estimates = $derived(
		tour.modes.map((m) => ({ mode: m, text: estimateText(tour.routes?.[m]) })).filter((e) => e.text)
	);
</script>

<a
	{href}
	class="block overflow-hidden rounded-lg border border-byl-border bg-byl-surface text-left"
>
	{#if hero}
		<img src={hero} alt="" class="h-44 w-full object-cover" />
	{/if}
	<div class="bg-byl-background p-4">
		{#if tour.topic}
			<div class="text-xs font-bold tracking-normal text-byl-action">
				{tour.topic.name}
			</div>
		{/if}
		<div class="font-[Libre_Baskerville] text-xl">{tour.title}</div>
		<div class="mt-1 flex flex-wrap gap-3 text-xs text-byl-muted">
			<span>{tour.stop_count} stops</span>
			{#each estimates as e (e.mode)}<span>{e.text}</span>{/each}
			{#if tour.owner_account_id}<span class="text-byl-muted">visitor route</span>{/if}
		</div>
	</div>
</a>
