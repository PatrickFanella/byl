<script lang="ts">
	import { safeHref } from '#lib/format.ts';
	import type { MediaAsset } from '#lib/visitor.ts';
	import MediaFigure from '#lib/components/visitor/MediaFigure.svelte';
	let { media }: { media: MediaAsset[] } = $props();
	const photos = $derived(media.filter((a) => a.kind === 'image'));
	const documents = $derived(media.filter((a) => a.kind === 'document'));
	const videos = $derived(media.filter((a) => a.kind === 'video'));
	const audio = $derived(media.filter((a) => a.kind === 'audio'));
</script>

<div class="space-y-7">
	{#if photos.length}
		<section aria-label="Photos">
			<h3 class="font-semibold">Photos</h3>
			<div class="mt-3 grid gap-6 sm:grid-cols-2 lg:grid-cols-3">
				{#each photos as asset (asset.id)}<MediaFigure {asset} />{/each}
			</div>
		</section>
	{/if}
	{#if documents.length}
		<section aria-label="Documents">
			<h3 class="font-semibold">Documents</h3>
			<ul class="mt-2 divide-y divide-byl-border border-y border-byl-border">
				{#each documents as asset (asset.id)}
					<li class="space-y-1 py-4">
						<a
							href={asset.public_url}
							target="_blank"
							rel="noreferrer"
							class="font-semibold text-byl-action underline">{asset.caption} ↗</a
						>
						<p class="text-sm text-byl-muted">
							{asset.credit} · {asset.external
								? 'Read at source'
								: asset.mime_type === 'application/pdf'
									? 'Download PDF'
									: 'Download text document'}
						</p>
						<p class="text-xs text-byl-muted">
							{#if !asset.external && asset.license_url}<a
									href={safeHref(asset.license_url)}
									target="_blank"
									rel="noreferrer"
									class="underline">{asset.license}</a
								> ·
							{/if}
							<a
								href={safeHref(asset.source_url)}
								target="_blank"
								rel="noreferrer"
								class="underline">Original source</a
							>
						</p>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
	{#if videos.length}
		<section aria-label="Videos">
			<h3 class="font-semibold">Videos</h3>
			<p class="mt-1 text-sm text-byl-muted">Watch on the publisher’s site.</p>
			<ul class="mt-2 divide-y divide-byl-border border-y border-byl-border">
				{#each videos as asset (asset.id)}
					<li class="space-y-1 py-4">
						<a
							href={asset.public_url}
							target="_blank"
							rel="noreferrer"
							class="font-semibold text-byl-action underline">{asset.caption} ↗</a
						>
						<p class="text-sm text-byl-muted">{asset.credit}</p>
					</li>
				{/each}
			</ul>
		</section>
	{/if}
	{#if audio.length}
		<section aria-label="Audio">
			<h3 class="font-semibold">Audio</h3>
			{#each audio as asset (asset.id)}
				<figure class="mt-3 max-w-md">
					<audio src={asset.public_url} controls class="w-full"></audio>
					<figcaption class="mt-1 text-sm text-byl-muted">
						{asset.caption} · {asset.credit}
					</figcaption>
				</figure>
			{/each}
		</section>
	{/if}
</div>
