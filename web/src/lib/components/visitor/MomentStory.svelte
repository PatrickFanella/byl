<script lang="ts">
	import { safeHref } from '#lib/format.ts';
	import { plainText } from '#lib/tour/narration.ts';
	import type { MomentDetail } from '#lib/visitor.ts';
	import MediaGallery from '#lib/components/visitor/MediaGallery.svelte';
	import SectionHeader from '#lib/components/visitor/SectionHeader.svelte';
	import LoadState from '#lib/components/visitor/LoadState.svelte';

	let { moment: m, omitMediaId = '' }: { moment: MomentDetail; omitMediaId?: string } = $props();
	const paragraphs = $derived(
		(m.body || m.excerpt || '')
			.split(/\n\s*\n/)
			.map(plainText)
			.filter(Boolean)
	);
	const media = $derived(m.media.filter((asset) => asset.id !== omitMediaId));
</script>

<article class="byl-story prose mt-5 max-w-2xl">
	{#each paragraphs as p, i (i)}<p>{p}</p>{/each}
</article>

{#if media.length}<section class="byl-section space-y-3" aria-label="Related material">
		<h2 class="byl-h2">Related material</h2>
		<MediaGallery {media} />
	</section>{/if}

{#if m.topics.length}
	<div class="mt-5 flex flex-wrap gap-2">
		{#each m.topics as t (t.id)}
			<span class="rounded border border-byl-border px-3 py-1 text-xs">{t.name}</span>
		{/each}
	</div>
{/if}

<section class="byl-section space-y-3">
	<SectionHeader title="Sources" eyebrow="Where this comes from" />
	{#if m.sources.length === 0}
		<LoadState
			status="empty"
			emptyTitle="No sources listed"
			emptyMessage="This moment was written without a cited source."
		/>
	{/if}
	{#if m.sources.length}
		<ul class="byl-rows max-w-[66ch]">
			{#each m.sources as s, i (i)}
				<li class="byl-row flex-col gap-1 text-sm">
					{#if s.url}
						<a
							href={safeHref(s.url)}
							target="_blank"
							rel="noreferrer"
							class="font-bold text-byl-action underline">{s.title || s.url} ↗</a
						>
					{:else}
						<div class="font-bold">{s.title}</div>
					{/if}
					<div class="text-byl-muted">
						{[s.publisher, s.license, s.retrieved_at ? `retrieved ${s.retrieved_at}` : '']
							.filter(Boolean)
							.join(' · ')}
					</div>
					{#if s.note}<div class="text-byl-muted italic">{s.note}</div>{/if}
				</li>
			{/each}
		</ul>
	{/if}
</section>
