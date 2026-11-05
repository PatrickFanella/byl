<script lang="ts">
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
			.map((p) =>
				p
					.replace(/^#+\s*/, '')
					.replace(/[*_`]/g, '')
					.trim()
			)
			.filter(Boolean)
	);
</script>

<svelte:head><title>{moment.data?.title ?? 'Moment'} · Invisible Museum</title></svelte:head>

<LoadState status={moment.status} message={moment.message} onretry={() => moment.run()}>
	{#if moment.data}
		{@const m = moment.data}
		<a href="/places/{m.place_id}" class="text-sm text-white/50 hover:underline">← Place</a>
		{#if m.date_display}
			<div class="mt-3 text-[11px] font-bold tracking-widest text-blue-300 uppercase">
				{m.date_display}
			</div>
		{/if}
		<h1 class="text-3xl font-semibold">{m.title}</h1>
		{#if m.has_narration || m.narration}
			<p class="mt-1 text-xs text-white/50">Audio guide available on the tour</p>
		{/if}

		{#if m.media.length}
			<div class="mt-5"><MediaGallery media={m.media} /></div>
		{/if}

		<article class="prose mt-5 max-w-2xl prose-invert">
			{#each paragraphs as p, i (i)}<p>{p}</p>{/each}
		</article>

		{#if m.topics.length}
			<div class="mt-5 flex flex-wrap gap-2">
				{#each m.topics as t (t.id)}
					<span class="rounded-full border border-white/20 px-3 py-1 text-xs">{t.name}</span>
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
				<div class="rounded-2xl border border-white/10 bg-white/5 p-4 text-sm">
					{#if s.url}
						<a
							href={s.url}
							target="_blank"
							rel="noreferrer"
							class="font-semibold text-blue-300 hover:underline">{s.title || s.url} ↗</a
						>
					{:else}
						<div class="font-semibold">{s.title}</div>
					{/if}
					<div class="text-xs text-white/50">
						{[s.publisher, s.license, s.retrieved_at ? `retrieved ${s.retrieved_at}` : '']
							.filter(Boolean)
							.join(' · ')}
					</div>
					{#if s.note}<div class="mt-1 text-xs text-white/40 italic">{s.note}</div>{/if}
				</div>
			{/each}
		</section>
	{/if}
</LoadState>
