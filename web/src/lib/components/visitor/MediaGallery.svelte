<script lang="ts">
	import type { MediaAsset } from '#lib/visitor.ts';
	import MediaFigure from '#lib/components/visitor/MediaFigure.svelte';

	let { media }: { media: MediaAsset[] } = $props();
	const credit = (a: MediaAsset) =>
		[a.credit, a.license].filter(Boolean).join(' · ') || 'Credit unavailable';
</script>

<div class="-mx-4 flex snap-x gap-4 overflow-x-auto px-4 pb-2">
	{#each media as asset (asset.id)}
		{#if asset.kind === 'image'}
			<div class="w-72 shrink-0 snap-start"><MediaFigure {asset} /></div>
		{:else}
			<figure class="w-72 shrink-0 snap-start">
				<div class="h-52 overflow-hidden rounded-lg border border-byl-border bg-byl-surface">
					{#if asset.kind === 'video'}
						<!-- svelte-ignore a11y_media_has_caption -->
						<video src={asset.public_url} controls class="h-full w-full object-cover"></video>
					{:else if asset.kind === 'audio'}
						<div class="flex h-full flex-col items-center justify-center gap-2 p-4">
							<audio src={asset.public_url} controls class="w-full"></audio>
						</div>
					{:else}
						<img src={asset.public_url} alt={asset.caption} class="h-full w-full object-cover" />
					{/if}
				</div>
				<figcaption class="mt-2 text-sm">
					{#if asset.caption}<div class="text-byl-muted">{asset.caption}</div>{/if}
					<div class="text-xs text-byl-muted">{credit(asset)}</div>
				</figcaption>
			</figure>
		{/if}
	{/each}
</div>
