<script lang="ts">
	import type { TourStop } from '#lib/visitor.ts';

	let { stop, visited = false }: { stop: TourStop; visited?: boolean } = $props();
	const featured = $derived(stop.featured_moment ?? null);
	const thumb = $derived(
		featured?.hero_media?.kind === 'image' ? (featured.hero_media.public_url ?? null) : null
	);
	const href = $derived(
		`/places/${stop.place.id}${featured ? `?featured=${featured.id}` : stop.place.primary_moment_id ? `?featured=${stop.place.primary_moment_id}` : ''}`
	);
</script>

<a {href} data-testid="stop-row" class="byl-row">
	<span class="byl-marker {visited ? 'byl-marker-visited' : ''}">
		{visited ? '✓' : stop.position}
	</span>
	<span class="min-w-0 flex-1">
		<span class="byl-row-title">{stop.place.name}</span>
		{#if visited}<span class="block text-sm text-byl-success">Visited</span>{/if}
		{#if featured}
			<span class="block text-sm text-byl-muted">{featured.title}</span>
			{#if featured.date_display}
				<span class="byl-eyebrow mt-1 block">{featured.date_display}</span>
			{/if}
		{:else}
			<span class="block text-sm text-byl-muted">No moment published here yet</span>
		{/if}
		{#if stop.place.moment_count > 1}
			<span class="block text-sm text-byl-muted"
				>+{stop.place.moment_count - 1} more at this place</span
			>
		{/if}
		{#if stop.note}<span class="mt-1 block text-sm text-byl-muted italic">{stop.note}</span>{/if}
	</span>
	{#if thumb}<img src={thumb} alt="" loading="lazy" class="byl-thumb" />{/if}
</a>
