<script lang="ts">
	import { city } from '#lib/city.svelte.ts';
</script>

{#if city.cities.length > 1}
	<label class="inline-flex items-center gap-2 text-sm text-byl-muted">
		City
		<select
			class="rounded border border-byl-border bg-byl-background px-3 py-1 text-byl-text"
			value={city.current?.slug ?? ''}
			onchange={(e) => {
				const c = city.cities.find((x) => x.slug === (e.currentTarget as HTMLSelectElement).value);
				if (c) city.select(c);
			}}
		>
			{#each city.cities as c (c.id)}<option value={c.slug}>{c.name}</option>{/each}
		</select>
	</label>
{/if}
