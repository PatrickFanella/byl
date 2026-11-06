<script lang="ts">
	import type { MediaAsset } from '#lib/visitor.ts';
	let { asset }: { asset: MediaAsset } = $props();
</script>

<figure>
	<a
		href={asset.public_url}
		target="_blank"
		rel="noreferrer"
		aria-label={`View full photo: ${asset.caption}`}
		class="block"
	>
		<img
			src={asset.public_url}
			alt={asset.caption}
			width={asset.width ?? undefined}
			height={asset.height ?? undefined}
			class="max-h-[32rem] w-full rounded-lg bg-byl-surface object-contain"
		/>
	</a>
	<figcaption class="mt-2 max-w-2xl space-y-1 text-sm text-byl-muted">
		{#if asset.caption}<p>{asset.caption}</p>{/if}
		<p class="text-xs">
			{asset.credit || 'Credit unavailable'}
			{#if asset.license}
				· {#if asset.license_url}<a
						class="underline"
						href={asset.license_url}
						target="_blank"
						rel="noreferrer">{asset.license}</a
					>{:else}{asset.license}{/if}
			{/if}
			{#if asset.source_url}
				· <a class="underline" href={asset.source_url} target="_blank" rel="noreferrer"
					>Image source</a
				>{/if}
		</p>
	</figcaption>
</figure>
