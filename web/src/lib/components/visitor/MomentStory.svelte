<script lang="ts">
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

{#if media.length}<div class="mt-5"><MediaGallery {media} /></div>{/if}
<article class="byl-story prose mt-5 max-w-2xl">
	{#each paragraphs as p, i (i)}<p>{p}</p>{/each}
</article>

{#if m.topics.length}
	<div class="mt-5 flex flex-wrap gap-2">
		{#each m.topics as t (t.id)}
			<span class="rounded border border-byl-border px-3 py-1 text-xs">{t.name}</span>
		{/each}
	</div>
{/if}

<section class="mt-8 space-y-3">
	<SectionHeader title="Sources" eyebrow="Where this comes from" />
	{#if m.sources.length === 0}
		<LoadState
			status="empty"
			emptyTitle="No sources listed"
			emptyMessage="This moment was written without a cited source."
		/>
	{/if}
	{#each m.sources as s, i (i)}
		<div class="rounded-lg border border-byl-border bg-byl-surface p-4 text-sm">
			{#if s.url}
				<a
					href={s.url}
					target="_blank"
					rel="noreferrer"
					class="font-semibold text-byl-action hover:underline">{s.title || s.url} ↗</a
				>
			{:else}
				<div class="font-semibold">{s.title}</div>
			{/if}
			<div class="text-xs text-byl-muted">
				{[s.publisher, s.license, s.retrieved_at ? `retrieved ${s.retrieved_at}` : '']
					.filter(Boolean)
					.join(' · ')}
			</div>
			{#if s.note}<div class="mt-1 text-xs text-byl-muted italic">{s.note}</div>{/if}
		</div>
	{/each}
</section>
