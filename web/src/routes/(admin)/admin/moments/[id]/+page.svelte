<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PayloadForm from '#lib/components/PayloadForm.svelte';
	import MediaAttachments from '#lib/components/MediaAttachments.svelte';
	import { fieldsFor, applyFields, type Field } from '#lib/payload.ts';
	import {
		updateMoment,
		setMomentStatus,
		setMomentMedia,
		setMomentSources,
		setMomentTopics,
		errorMessage,
		type Source
	} from '#lib/admin.ts';
	import { ApiError } from '#lib/api/index.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let moment = $derived(data.moment);
	let fields: Field[] = $derived(
		fieldsFor('moment', {
			...moment,
			date_start: moment.date_start ?? '',
			date_end: moment.date_end ?? ''
		} as unknown as Record<string, unknown>).filter(
			(f) => f.key !== 'draft_notes' && f.key !== 'slug'
		)
	);
	let topicIds: string[] = $derived(moment.topic_ids ?? []);
	let sources: Source[] = $derived(moment.sources.map((s) => ({ ...s })));
	let mediaIds: string[] = $derived(moment.media.map((m) => m.id));
	let heroId = $derived(moment.hero_media?.id ?? '');
	let errors: Record<string, string> = $state({});
	let notice = $state({ kind: 'info' as 'info' | 'error' | 'success', message: '' });
	let busy = $state('');

	function toggleTopic(id: string) {
		topicIds = topicIds.includes(id) ? topicIds.filter((t) => t !== id) : [...topicIds, id];
	}

	async function save() {
		busy = 'save';
		errors = {};
		try {
			const body = applyFields({}, fields) as Record<string, unknown>;
			body.hero_media_id = heroId;
			await updateMoment(moment.id, body);
			await setMomentTopics(moment.id, topicIds);
			await setMomentSources(
				moment.id,
				sources.filter((s) => s.title.trim())
			);
			await setMomentMedia(moment.id, mediaIds);
			notice = { kind: 'success', message: 'Saved.' };
			await invalidateAll();
		} catch (e) {
			if (e instanceof ApiError) errors = e.fields ?? {};
			notice = { kind: 'error', message: errorMessage(e, 'Save failed.') };
		} finally {
			busy = '';
		}
	}

	async function status(action: 'publish' | 'unpublish') {
		busy = action;
		try {
			await setMomentStatus(moment.id, action);
			notice = {
				kind: 'success',
				message:
					action === 'publish'
						? 'Published. Narration is being generated if a provider is configured.'
						: 'Back to draft.'
			};
			await invalidateAll();
		} catch (e) {
			notice = { kind: 'error', message: errorMessage(e, 'Status change failed.') };
		} finally {
			busy = '';
		}
	}
</script>

<div class="flex flex-wrap items-center gap-3">
	<a href="/admin/places/{data.place.id}" class="text-sm text-byl-muted hover:underline"
		>← {data.place.name}</a
	>
	<h1 class="text-2xl font-semibold">{moment.title}</h1>
	<StatusPill status={moment.status} />
	{#if moment.has_narration}<span class="text-xs text-byl-muted">narrated</span>{/if}
</div>
<div class="mt-3"><Notice kind={notice.kind} message={notice.message} /></div>

<div class="mt-4 grid gap-6 lg:grid-cols-3">
	<div class="space-y-5 lg:col-span-2">
		<PayloadForm bind:fields {errors} />

		<section>
			<h2 class="text-sm font-semibold">Topics</h2>
			<div class="mt-2 flex flex-wrap gap-2">
				{#each data.topics as t (t.id)}
					<label
						class="flex items-center gap-1 rounded-md border border-byl-border-control px-2 py-1 text-sm {topicIds.includes(
							t.id
						)
							? 'bg-byl-action text-byl-action-text'
							: ''}"
					>
						<input
							type="checkbox"
							class="sr-only"
							checked={topicIds.includes(t.id)}
							onchange={() => toggleTopic(t.id)}
						/>{t.name}
					</label>
				{/each}
			</div>
		</section>

		<section>
			<div class="flex items-center justify-between">
				<h2 class="text-sm font-semibold">Sources</h2>
				<button
					class="text-xs underline"
					onclick={() =>
						(sources = [...sources, { title: '', url: '', publisher: '', license: '', note: '' }])}
					>Add source</button
				>
			</div>
			<div class="mt-2 space-y-2">
				{#each sources as s, i (i)}
					<div
						class="grid gap-2 rounded-md border border-byl-border bg-byl-background p-2 text-sm md:grid-cols-5"
					>
						<input
							class="rounded-md border-byl-border-control"
							placeholder="Title"
							bind:value={s.title}
						/>
						<input
							class="rounded-md border-byl-border-control md:col-span-2"
							placeholder="URL"
							bind:value={s.url}
						/>
						<input
							class="rounded-md border-byl-border-control"
							placeholder="Publisher"
							bind:value={s.publisher}
						/>
						<div class="flex gap-1">
							<input
								class="w-full rounded-md border-byl-border-control"
								placeholder="Licence"
								bind:value={s.license}
							/><button
								class="text-xs text-byl-muted"
								title="Remove"
								onclick={() => (sources = sources.filter((_, j) => j !== i))}>✕</button
							>
						</div>
					</div>
				{:else}
					<p class="text-xs text-byl-muted">Add at least one source so the claim can be checked.</p>
				{/each}
			</div>
		</section>

		<MediaAttachments
			library={[...data.library, ...moment.media]}
			bind:ids={mediaIds}
			bind:heroId
			chooseHero
		/>

		<div class="flex flex-wrap gap-2">
			<button
				class="rounded-md bg-byl-action px-3 py-1.5 text-sm text-byl-action-text disabled:opacity-50"
				disabled={!!busy}
				onclick={save}>Save</button
			>
			{#if moment.status === 'published'}
				<button
					class="rounded-md border border-byl-border-control px-3 py-1.5 text-sm disabled:opacity-50"
					disabled={!!busy}
					onclick={() => status('unpublish')}>Unpublish</button
				>
			{:else}
				<button
					class="rounded-md bg-byl-success-surface px-3 py-1.5 text-sm text-byl-action-text disabled:opacity-50"
					disabled={!!busy}
					onclick={() => status('publish')}>Publish</button
				>
			{/if}
		</div>
	</div>
	<aside class="space-y-3 text-sm">
		<div class="rounded-lg border border-byl-border bg-byl-background p-4">
			<h2 class="font-semibold">Preview</h2>
			<p class="mt-1 text-xs text-byl-muted">{moment.date_display}</p>
			<div class="prose prose-sm mt-2 max-w-none whitespace-pre-wrap">
				{fields.find((f) => f.key === 'body')?.value}
			</div>
		</div>
		{#if moment.narration}
			<audio controls src={moment.narration.public_url} class="w-full"></audio>
		{/if}
	</aside>
</div>
