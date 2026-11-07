<script lang="ts">
	import {
		getBriefItinerary,
		startBriefResearch,
		errorMessage,
		type RouteBrief,
		type BriefItinerary
	} from '#lib/admin.ts';
	import { ApiError } from '#lib/api/index.ts';
	let {
		brief,
		onresearchrequested = () => {}
	}: { brief: RouteBrief; onresearchrequested?: (id: string, version: number) => void } = $props();
	let report = $state<BriefItinerary | null>(null);
	let loadedBrief = $state('');
	let queuedVersion = $state('');
	let busy = $state('');
	let message = $state('');
	let error = $state('');
	const sourceKey = $derived(`${brief.id}:${brief.version}`);
	const canStart = $derived(
		(brief.status === 'draft' || brief.status === 'needs_research') && queuedVersion !== sourceKey
	);
	const shapes: Record<string, string> = {
		loop: 'Loop',
		point_to_point: 'Point to point',
		out_and_back: 'Out and back'
	};
	const exclusions: Record<string, string> = {
		proposal_stop_limit: 'Outside this bounded eight-stop proposal; retained for further review',
		not_thematic: 'Does not match the requested theme',
		missing_supported_location: 'Needs a supported location and historical account',
		another_moment_selected_at_place: 'Another event was selected at this place',
		geography_or_duration_requires_review: 'Does not fit the provisional geography or time screen'
	};
	async function start() {
		const key = sourceKey;
		busy = 'start';
		message = '';
		error = '';
		try {
			const run = await startBriefResearch(brief.id, brief.version);
			if (sourceKey === key) {
				queuedVersion = key;
				onresearchrequested(brief.id, brief.version);
				message =
					run.status === 'active'
						? 'Research queued from the saved brief. Inspect the proposal after processing; your unsaved form text has been kept.'
						: 'Saved research is already available for this version. Inspect the proposal; your unsaved form text has been kept.';
			}
		} catch (e) {
			error = errorMessage(
				e,
				'Could not start research. Reload the saved brief before retrying if its version changed.'
			);
		} finally {
			busy = '';
		}
	}
	async function inspect() {
		const id = brief.id;
		busy = 'inspect';
		message = '';
		error = '';
		try {
			const result = await getBriefItinerary(id);
			if (brief.id === id) {
				report = result;
				loadedBrief = id;
				if (!result.proposal)
					message =
						'Assembly has not produced an itinerary yet. Inspect again after research finishes.';
			}
		} catch (e) {
			if (e instanceof ApiError && e.status === 404)
				message = 'No research run is available yet. Start research from the saved brief.';
			else error = errorMessage(e, 'Could not load the retained itinerary proposal.');
		} finally {
			busy = '';
		}
	}
</script>

<section
	aria-label="Itinerary proposal"
	class="mt-6 max-w-3xl border-y border-byl-border py-4 text-sm"
>
	<h2 class="text-lg font-semibold">Research and itinerary</h2>
	<p class="mt-2 text-byl-muted">
		Research uses the saved brief. Save any edits first. It can reuse sourced histories or
		evidence-linked drafts; unresolved places stay linked for review.
	</p>
	<div class="mt-3 flex flex-wrap gap-3">
		<button
			type="button"
			class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
			disabled={!!busy || !canStart}
			onclick={start}>Start research from saved brief</button
		>
		<button
			type="button"
			class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
			disabled={!!busy}
			onclick={inspect}
			>{busy === 'inspect' ? 'Loading proposal…' : 'Inspect itinerary proposal'}</button
		>
	</div>
	{#if message}<p role="status" class="mt-3">{message}</p>{/if}
	{#if error}<p role="alert" class="mt-3 text-byl-danger">{error}</p>{/if}
	{#if report?.proposal && loadedBrief === brief.id}
		{@const p = report.proposal}
		<div class="mt-5 space-y-4">
			<h3 class="text-lg font-semibold break-words">{p.title}</h3>
			<p class="font-medium">
				{p.status === 'needs_research' ? 'Needs research' : 'Private draft proposal'} · {shapes[
					p.trip_shape
				]}
			</p>
			<p>{p.introduction}</p>
			<p class="text-xs text-byl-muted">
				Retained research snapshot from saved brief version {report.source_version}. Current
				sources, visible features and travel still need review. No tour has been created or
				published.
			</p>
			<dl class="grid grid-cols-2 gap-2 sm:grid-cols-3">
				<div>
					<dt class="text-byl-muted">Planned dwell</dt>
					<dd>{p.planned_dwell_minutes} min</dd>
				</div>
				<div>
					<dt class="text-byl-muted">Planned narration</dt>
					<dd>{p.planned_narration_minutes} min</dd>
				</div>
				<div>
					<dt class="text-byl-muted">Travel time</dt>
					<dd>Not computed</dd>
				</div>
			</dl>
			{#if p.target_duration_minutes}<p>
					Requested duration: {p.target_duration_minutes} min, including travel that remains uncomputed.
				</p>{/if}
			<ol class="space-y-4">
				{#each p.stops as stop, i (stop.material.place_id)}
					<li class="border-l-2 border-byl-border pl-3">
						<h4 class="font-semibold">
							{i + 1}. {stop.material.place_name} — {stop.material.moment_title}
						</h4>
						<p class="mt-2 whitespace-pre-wrap">{stop.material.narrative}</p>
						<p class="mt-2 text-xs text-byl-muted">
							{stop.theme_reason} · {stop.material.evidence_kind === 'curated_record'
								? 'Existing curated record'
								: 'Evidence-linked candidate draft'}
						</p>
						<p class="mt-2">{stop.what_to_notice}</p>
						{#if stop.transition}<p class="mt-2 text-byl-muted">{stop.transition}</p>{/if}
						<div class="mt-2 flex flex-wrap gap-3 text-xs">
							<a class="underline" href="/admin/places/{stop.material.place_id}">Review place</a>
							{#if stop.material.moment_id}<a
									class="underline"
									href="/admin/moments/{stop.material.moment_id}">Review historical account</a
								>{/if}
							{#if stop.material.candidate_id}<a
									class="underline"
									href="/admin/queue/{stop.material.candidate_id}">Review candidate</a
								>{/if}
							{#each stop.material.source_urls as url, index (url)}<a
									class="break-all underline"
									href={url}
									target="_blank"
									rel="noreferrer">Source {index + 1}</a
								>{/each}
						</div>
					</li>
				{:else}<li>
						No supported itinerary stops yet. Retained research links and gaps remain available
						below.
					</li>{/each}
			</ol>
			{#if p.travel_sequence.length}<p>
					<strong>Travel sequence:</strong>
					{p.travel_sequence.map((i) => i + 1).join(' → ')}. Repeated numbers are return travel,
					without another story stop.
				</p>{/if}
			{#if p.research_gaps.length}
				<div>
					<h4 class="font-semibold">Research gaps</h4>
					<ul class="mt-2 space-y-2">
						{#each p.research_gaps as gap, i (gap.code + gap.record_id + i)}<li>
								{gap.detail}
								{#if gap.place_candidate_id}<a
										class="ml-2 underline"
										href="/admin/queue/{gap.place_candidate_id}">Place candidate</a
									>{/if}
								{#if gap.moment_candidate_id}<a
										class="ml-2 underline"
										href="/admin/queue/{gap.moment_candidate_id}">Event candidate</a
									>{/if}
							</li>{/each}
					</ul>
				</div>
			{/if}
			{#if p.excluded.length}<details>
					<summary class="cursor-pointer font-medium"
						>Excluded material ({p.excluded.length})</summary
					>
					<ul class="mt-2 space-y-2">
						{#each p.excluded as item, i (item.material.record_id + i)}<li>
								{item.material.place_name} · {item.material.moment_title}: {exclusions[
									item.reason
								] ?? 'Requires further review'}
							</li>{/each}
					</ul>
				</details>{/if}
			<details>
				<summary class="cursor-pointer font-medium">Planning assumptions</summary>
				<ul class="mt-2 list-disc space-y-2 pl-5">
					{#each p.assumptions as assumption (assumption)}<li>{assumption}</li>{/each}
				</ul>
			</details>
		</div>
	{/if}
</section>
