<script lang="ts">
	import { city } from '#lib/city.svelte.ts';
</script>

{#if city.cities.length > 1}
	<label class="inline-flex items-center gap-2 text-sm text-white/70">
		City
		<select
			class="rounded-full border border-white/20 bg-black/40 px-3 py-1 text-white"
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
