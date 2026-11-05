<script lang="ts">
	import type { TourStop } from '#lib/visitor.ts';

	let { stop, visited = false }: { stop: TourStop; visited?: boolean } = $props();
	const featured = $derived(stop.featured_moment ?? null);
	const href = $derived(
		`/places/${stop.place.id}${featured ? `?featured=${featured.id}` : stop.place.primary_moment_id ? `?featured=${stop.place.primary_moment_id}` : ''}`
	);
</script>

<a
	{href}
	data-testid="stop-row"
	class="flex gap-4 rounded-2xl border border-white/10 bg-white/5 p-4 hover:bg-white/10"
>
	<span
		class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold {visited
			? 'bg-emerald-400 text-black'
			: 'bg-blue-500 text-black'}"
	>
		{visited ? '✓' : stop.position}
	</span>
	<span class="min-w-0 flex-1">
		<span class="block font-semibold">{stop.place.name}</span>
		{#if featured}
			<span class="block text-sm text-white/85">{featured.title}</span>
			{#if featured.date_display}
				<span class="block text-[11px] font-bold tracking-wide text-blue-300"
					>{featured.date_display}</span
				>
			{/if}
		{:else}
			<span class="block text-sm text-white/40">No moment published here yet</span>
		{/if}
		{#if stop.place.moment_count > 1}
			<span class="block text-xs text-white/50"
				>+{stop.place.moment_count - 1} more at this place</span
			>
		{/if}
		{#if stop.note}<span class="block text-xs text-white/50 italic">{stop.note}</span>{/if}
	</span>
</a>
