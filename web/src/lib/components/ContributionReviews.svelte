<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { session } from '#lib/auth.ts';
	import {
		listContributionReviews,
		getContributionReview,
		contributionReviewURL,
		errorMessage,
		type ContributionReview,
		type ContributionReviewSelection
	} from '#lib/admin.ts';
	import MapView from './Map.svelte';
	let { search = '' }: { search?: string } = $props();
	const actor = untrack(() => session.account()?.id);
	const query = untrack(() => new URLSearchParams(search));
	const selected = untrack(() =>
		query.has('revision_id')
			? (Object.fromEntries(
					['revision_id', 'segment_id', 'mode', 'direction', 'window_start'].map((k) => [
						k,
						query.get(k) ?? ''
					])
				) as ContributionReviewSelection)
			: null
	);
	const offset = untrack(() => Math.max(0, Number(query.get('offset')) || 0));
	let active = true;
	let items = $state<ContributionReview[]>([]);
	let busy = $state(true);
	let error = $state('');
	onDestroy(() => {
		active = false;
	});
	const current = () => active && session.account()?.id === actor;
	async function reload() {
		busy = true;
		error = '';
		items = [];
		try {
			const result = selected
				? await getContributionReview(selected)
				: await listContributionReviews(offset);
			if (current()) items = result.items;
		} catch (e) {
			if (current()) error = errorMessage(e, 'Could not load live contribution review tasks.');
		} finally {
			if (current()) busy = false;
		}
	}
	onMount(reload);
	function share(item: ContributionReview) {
		return item.qualified_passages
			? `${((100 * item.departure_rejoins) / item.qualified_passages).toFixed(1)}%`
			: 'No eligible observations';
	}
</script>

<h1 class="text-2xl font-semibold">Route contribution review tasks</h1>
<p class="mt-3 max-w-3xl text-sm">
	Review repeated departure/rejoin patterns before investigating a route change. These are
	uncalibrated, opted-in observations from the current and preceding three UTC weeks.
</p>
<details class="mt-3 max-w-3xl text-sm">
	<summary class="cursor-pointer underline">How samples and thresholds are counted</summary>
	<p class="mt-2">
		Each task keeps one retained revision, logical corridor segment, mode, direction and UTC Monday
		week separate. It requires at least five distinct account scopes, three scopes with departures
		and a departure/rejoin share of at least 30%. Scopes are not verified people; repeat visits,
		multiple accounts, uneven GPS coverage and self-selection limit the sample. Missing observations
		never count as passages without departures.
	</p>
	<p class="mt-2">
		A task prompts curator investigation; it never changes, approves or reroutes a public itinerary.
		Counts do not establish motive, closure, legal access or safety.
	</p>
</details>
<div class="mt-4 flex flex-wrap gap-4 text-sm">
	<a class="underline" href="/admin/queue">Review queue</a>
	{#if selected}<a class="underline" href="/admin/queue/contributions">All flagged patterns</a>{/if}
	<button
		class="rounded border border-byl-border-control px-3 py-1.5"
		disabled={busy}
		onclick={reload}>{busy ? 'Loading…' : 'Reload live counts'}</button
	>
</div>
{#if error}<p role="alert" class="mt-4 text-byl-danger">{error}</p>{/if}
{#if !busy && !error && items.length === 0}<p class="mt-6 text-sm">
		No current tasks meet the sample and share thresholds. This is not evidence that routes have no
		problems. Only the current and preceding three UTC weeks are considered.
	</p>{/if}
<div aria-busy={busy} class="mt-6 space-y-6">
	{#each items as item (`${item.revision_id}:${item.segment_id}:${item.mode}:${item.direction}:${item.window_start}`)}
		<article class="rounded border border-byl-border bg-byl-surface p-4">
			<h2 class="text-lg font-semibold">{item.tour_title} · Revision {item.revision_number}</h2>
			<p class="mt-1 text-sm">
				Section {item.segment_id}
				{item.segment_kind ? `(${item.segment_kind})` : ''} · {item.mode} · {item.direction}
			</p>
			<p class="mt-2 text-sm">
				UTC week {item.window_start} to {item.window_end} (end exclusive); observations through {item.observed_through}.
			</p>
			<p class="mt-3 font-semibold">
				{item.departure_rejoins} departure/rejoins / {item.qualified_passages} eligible observed passages
				· {share(item)}
			</p>
			<p class="mt-1 text-sm">
				{item.contributor_scopes} distinct account scopes; {item.departure_scopes} with departures. Each
				scope contributes at most one eligible summary per section, direction and UTC day. Exact upload
				retries add nothing.
			</p>
			<p class="mt-2 text-sm" role="status">
				{item.flagged
					? 'Sample and share thresholds met · Needs curator review'
					: 'Thresholds no longer met · This task is no longer flagged'}
			</p>
			<p class="mt-2 text-sm text-byl-muted">
				Counts are read live. Withdrawal and expiry can reduce or clear the task on reload. No
				cached rollup or approval is retained. Threshold policy: {item.thresholds.version}.
			</p>
			{#if !selected}
				<a class="mt-3 inline-block underline" href={contributionReviewURL(item)}
					>Inspect this section and related reports</a
				>
			{:else}
				{#if item.geometry?.coordinates?.length}<div class="mt-4">
						<MapView line={item.geometry.coordinates as [number, number][]} height="18rem" />
					</div>
				{:else}<p class="mt-3 text-sm">
						The retained section has no displayable reference geometry. Do not infer a path from
						these counts.
					</p>{/if}
				<h3 class="mt-4 font-semibold">Reports on this revision and mode</h3>
				<p class="mt-1 text-sm">
					Reports describe story legs or stops, not logical corridor IDs. Their relationship to this
					section is unverified. Compare the affected locations and sources before using the normal
					investigation and correction workflow.
				</p>
				<ul class="mt-3 space-y-2">
					{#each item.reports as report (report.id)}<li>
							<a class="underline" href={`/admin/queue/reports/${encodeURIComponent(report.id)}`}
								>Review {report.reason} report · {report.status} · {report.affected.kind}</a
							>
						</li>
					{:else}<li class="text-sm text-byl-muted">
							No explicit reports on this revision and mode. Review the working tour and gather
							independent evidence; these counts do not invent a visitor report.
						</li>{/each}
				</ul>
				{#if item.reports_truncated}<p class="mt-2 text-sm">
						Showing the 20 newest related reports. <a class="underline" href="/admin/queue/reports"
							>Open the report queue for more</a
						>.
					</p>{/if}
				<a
					class="mt-4 inline-block underline"
					href={`/admin/tours/${encodeURIComponent(item.tour_id)}`}
					>Open working tour and normal review controls</a
				>
				<p class="mt-1 text-sm text-byl-muted">
					The working tour may differ from this retained revision. Existing access, source,
					route-quality and publication checks still apply.
				</p>
			{/if}
		</article>
	{/each}
</div>
{#if !selected && !busy && !error}
	<nav aria-label="Contribution review pages" class="mt-4 flex gap-4 text-sm">
		{#if offset > 0}<a
				class="underline"
				href={`/admin/queue/contributions?offset=${Math.max(0, offset - 50)}`}>Previous page</a
			>{/if}
		{#if items.length === 50}<a
				class="underline"
				href={`/admin/queue/contributions?offset=${offset + 50}`}>Next page</a
			>{/if}
	</nav>
{/if}
