<script lang="ts">
	import { page } from '$app/state';
	import { ApiError } from '#lib/api/index.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getRouteSuggestion } from '#lib/visitor.ts';
	import { suggestionStatuses } from '#lib/routes/suggestions.ts';
	import LoadState from './LoadState.svelte';
	let { id }: { id: string } = $props();
	const brief = new Loader(async () => {
		try {
			return await getRouteSuggestion(id);
		} catch (error) {
			if (error instanceof ApiError && error.status === 404)
				throw new ApiError(404, 'not_found', 'This suggestion is not available for this account.');
			throw error;
		}
	});
	$effect(() => {
		brief.run();
	});
	const status = $derived(
		brief.data
			? (suggestionStatuses[brief.data.status] ?? {
					label: 'Status unavailable',
					detail: 'Reload to check the current submission status.'
				})
			: null
	);
</script>

<div class="mt-6 max-w-3xl">
	{#if page.url.searchParams.get('submitted') === '1' && brief.status === 'ready'}<p
			role="status"
			class="mb-5 rounded-lg border border-byl-border bg-byl-surface p-4"
		>
			Suggestion saved privately. Curators will review it before any publication.
		</p>{/if}
	<LoadState status={brief.status} message={brief.message} onretry={() => brief.run()}>
		{#if brief.data && status}
			<h2 class="text-2xl font-semibold break-words">{brief.data.submitted.topic}</h2>
			<p class="mt-2 break-words text-byl-muted">{brief.data.submitted.area}</p>
			<div
				class="mt-5 rounded-lg border border-byl-border bg-byl-surface p-5"
				aria-labelledby="suggestion-state"
			>
				<h3 id="suggestion-state" class="font-semibold">Status: {status.label}</h3>
				<p class="mt-2 text-sm">{status.detail}</p>
				{#if brief.data.reason}<h4 class="mt-4 text-sm font-semibold">Curator update</h4>
					<p class="mt-1 text-sm break-words whitespace-pre-wrap">{brief.data.reason}</p>{/if}
				<p class="mt-3 text-xs text-byl-muted">
					Updated {new Date(brief.data.updated_at).toLocaleString()}
				</p>
				<button
					class="mt-4 rounded-lg border border-byl-border px-3 py-2 text-sm"
					onclick={() => brief.run()}>Refresh status</button
				>
			</div>
			<section class="mt-7" aria-labelledby="research-leads-title">
				<h3 id="research-leads-title" class="text-lg font-semibold">Private research leads</h3>
				<p class="mt-2 text-sm text-byl-muted">
					These are leads for investigation, not verified history. Only you and authorized curators
					can read this suggestion. Curators may refine the saved brief during research.
				</p>
				{#if brief.data.submitted.notes}<h4 class="mt-5 text-sm font-semibold">Research notes</h4>
					<p class="mt-1 break-words whitespace-pre-wrap">{brief.data.submitted.notes}</p>{/if}
				{#each [{ key: 'place_leads', label: 'Place leads' }, { key: 'event_leads', label: 'Event leads' }, { key: 'source_links', label: 'Source links' }] as group (group.key)}
					{@const entries =
						brief.data.submitted[group.key as 'place_leads' | 'event_leads' | 'source_links'] ?? []}
					{#if entries.length}
						<h4 class="mt-5 text-sm font-semibold">{group.label}</h4>
						<ul class="mt-1 list-disc space-y-1 pl-5">
							{#each entries as entry, index (index)}<li class="break-words">
									{#if group.key === 'source_links' && /^https?:\/\//i.test(entry)}<a
											href={entry}
											class="text-byl-action underline"
											target="_blank"
											rel="noreferrer">{entry}</a
										>{:else}{entry}{/if}
								</li>{/each}
						</ul>
					{/if}
				{/each}
			</section>
		{/if}
	</LoadState>
</div>
