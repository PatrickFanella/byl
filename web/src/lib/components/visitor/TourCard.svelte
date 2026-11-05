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
	class="relative block h-44 overflow-hidden rounded-3xl border border-white/10 bg-gradient-to-br from-blue-500 to-purple-500 text-left"
>
	{#if hero}
		<img src={hero} alt="" class="absolute inset-0 h-full w-full object-cover" />
	{:else}
		<svg
			class="absolute top-1/2 left-1/2 h-16 w-16 -translate-x-1/2 -translate-y-1/2 text-white/60"
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="0.8"
			aria-hidden="true"
		>
			<path d="M3 21h18M5 21V10m14 11V10M3 10l9-6 9 6M8 21v-7m4 7v-7m4 7v-7" />
		</svg>
	{/if}
	<div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 to-transparent p-4">
		{#if tour.topic}
			<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">
				{tour.topic.name}
			</div>
		{/if}
		<div class="text-xl font-semibold">{tour.title}</div>
		<div class="mt-1 flex flex-wrap gap-3 text-xs text-white/80">
			<span>{tour.stop_count} stops</span>
			{#each estimates as e (e.mode)}<span>{e.text}</span>{/each}
			{#if tour.owner_account_id}<span class="text-white/50">visitor route</span>{/if}
		</div>
	</div>
</a>
