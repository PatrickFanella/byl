<script lang="ts">
	import { goto } from '$app/navigation';
	import { Loader } from '#lib/loader.svelte.ts';
	import { routeSuggestionIntake, myRouteSuggestions, type RouteSuggestion } from '#lib/visitor.ts';
	import { suggestionStatuses } from '#lib/routes/suggestions.ts';
	import RouteSuggestionForm from './RouteSuggestionForm.svelte';
	import LoadState from './LoadState.svelte';

	let { ownerID }: { ownerID: string } = $props();
	let offset = $state(0);
	const intake = new Loader(routeSuggestionIntake);
	const suggestions = new Loader<RouteSuggestion[]>(
		async () => (await myRouteSuggestions(offset)).items,
		(items) => items.length === 0
	);
	$effect(() => {
		intake.run();
	});
	$effect(() => {
		suggestions.run();
	});
	function move(delta: number) {
		offset = Math.max(0, offset + delta);
	}
</script>

<div class="mt-6">
	{#if intake.status === 'error'}
		<p role="alert" class="text-sm text-byl-danger">
			We could not check whether new suggestions are open.
		</p>
		<button
			class="mt-2 rounded-lg border border-byl-border px-3 py-2 text-sm"
			onclick={() => intake.run()}>Check intake again</button
		>
	{:else if intake.status === 'loading'}
		<p role="status" class="text-byl-muted">Checking suggestion availability…</p>
	{:else if intake.data?.enabled === true}
		<RouteSuggestionForm
			{ownerID}
			onsaved={async (saved) => {
				await goto(`/routes/suggestions/${encodeURIComponent(saved.id)}?submitted=1`);
			}}
		/>
	{:else}
		<p class="rounded-lg border border-byl-border bg-byl-surface p-4">
			New suggestions are closed. You can still read the status of suggestions you have already
			sent.
		</p>
	{/if}

	<section class="mt-10" aria-labelledby="your-suggestions-title">
		<div class="flex flex-wrap items-center justify-between gap-3">
			<h2 id="your-suggestions-title" class="text-xl font-semibold">Your submissions</h2>
			<button
				class="rounded-lg border border-byl-border px-3 py-2 text-sm"
				onclick={() => suggestions.run()}>Refresh submissions</button
			>
		</div>
		<div class="mt-4">
			<LoadState
				status={suggestions.status}
				message={suggestions.message}
				emptyTitle="No suggestions on this page"
				emptyMessage="Suggestions you send will appear here with their private status."
				onretry={() => suggestions.run()}
			>
				<ul class="divide-y divide-byl-border border-y border-byl-border">
					{#each (suggestions.data ?? []).slice(0, 20) as suggestion (suggestion.id)}
						<li class="py-4">
							<a
								href={`/routes/suggestions/${encodeURIComponent(suggestion.id)}`}
								class="font-semibold text-byl-action underline underline-offset-4"
								>{suggestion.submitted.topic}</a
							>
							<p class="mt-1 text-sm text-byl-muted">
								{suggestion.submitted.area} · {suggestionStatuses[suggestion.status]?.label ??
									'Status unavailable'}
							</p>
						</li>
					{/each}
				</ul>
			</LoadState>
		</div>
		{#if offset > 0 || (suggestions.data?.length ?? 0) > 20}
			<nav aria-label="Submission pages" class="mt-4 flex flex-wrap items-center gap-4 text-sm">
				<button
					disabled={offset === 0 || suggestions.status === 'loading'}
					class="rounded-lg border border-byl-border px-3 py-2 disabled:opacity-50"
					onclick={() => move(-20)}>Previous submissions</button
				>
				<span>Page {offset / 20 + 1}</span>
				<button
					disabled={(suggestions.data?.length ?? 0) <= 20 || suggestions.status === 'loading'}
					class="rounded-lg border border-byl-border px-3 py-2 disabled:opacity-50"
					onclick={() => move(20)}>Next submissions</button
				>
			</nav>
		{/if}
	</section>
</div>
