<script lang="ts">
	import { plainText } from '#lib/tour/narration.ts';
	import { page } from '$app/state';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getMoment, type MomentDetail } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import MediaGallery from '#lib/components/visitor/MediaGallery.svelte';
	import SectionHeader from '#lib/components/visitor/SectionHeader.svelte';

	const id = $derived(page.params.id ?? '');
	const moment = new Loader<MomentDetail>(() => getMoment(id));

	$effect(() => {
		void id;
		moment.run();
	});

	/** Markdown body as paragraphs; inline marks are shown as plain text (no HTML injection). */
	const paragraphs = $derived(
		(moment.data?.body || moment.data?.excerpt || '')
			.split(/\n\s*\n/)
			.map(plainText)
			.filter(Boolean)
	);
</script>

<svelte:head><title>{moment.data?.title ?? 'Moment'} · Byl</title></svelte:head>

<LoadState status={moment.status} message={moment.message} onretry={() => moment.run()}>
	{#if moment.data}
		{@const m = moment.data}
		<a href="/places/{m.place_id}" class="text-sm text-byl-muted hover:underline">← Place</a>
		{#if m.date_display}
			<div class="mt-3 text-xs font-bold tracking-normal text-byl-action">
				{m.date_display}
			</div>
		{/if}
		<h1 class="text-3xl font-semibold">{m.title}</h1>
		{#if m.has_narration || m.narration}
			<p class="mt-1 text-xs text-byl-muted">Audio guide available on the tour</p>
		{/if}

		{#if m.media.length}
			<div class="mt-5"><MediaGallery media={m.media} /></div>
		{/if}

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
	{/if}
</LoadState>
