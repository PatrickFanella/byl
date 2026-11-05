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
	class="flex gap-4 rounded-lg border border-byl-border bg-byl-surface p-4 hover:bg-byl-surface"
>
	<span
		class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full text-sm font-bold {visited
			? 'bg-byl-success-surface text-byl-success'
			: 'bg-byl-action text-byl-action-text'}"
	>
		{visited ? '✓' : stop.position}
	</span>
	<span class="min-w-0 flex-1">
		<span class="block font-semibold">{stop.place.name}</span>
		{#if featured}
			<span class="block text-sm text-byl-muted">{featured.title}</span>
			{#if featured.date_display}
				<span class="block text-xs font-bold tracking-wide text-byl-action"
					>{featured.date_display}</span
				>
			{/if}
		{:else}
			<span class="block text-sm text-byl-muted">No moment published here yet</span>
		{/if}
		{#if stop.place.moment_count > 1}
			<span class="block text-xs text-byl-muted"
				>+{stop.place.moment_count - 1} more at this place</span
			>
		{/if}
		{#if stop.note}<span class="block text-xs text-byl-muted italic">{stop.note}</span>{/if}
	</span>
</a>
