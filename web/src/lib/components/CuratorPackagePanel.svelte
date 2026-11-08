<script lang="ts">
	import { onMount } from 'svelte';
	import Map from './Map.svelte';
	import { ApiError } from '#lib/api/index.ts';
	import {
		getReviewPackage,
		publishReviewPackage,
		decideRouteBrief,
		resumeBriefResearch,
		draftCandidate,
		listTours,
		errorMessage,
		type RouteBrief,
		type CuratorReviewPackage,
		type MomentDetail,
		type TourSummary
	} from '#lib/admin.ts';
	let {
		brief,
		onchanged = () => {}
	}: { brief: RouteBrief; onchanged?: () => void | Promise<void> } = $props();
	let tours = $state<TourSummary[]>([]);
	let tourID = $state('');
	let report = $state<CuratorReviewPackage | null>(null);
	let loadedKey = $state('');
	let loadedTour = $state('');
	let busy = $state('');
	let error = $state('');
	let message = $state('');
	let invalidated = $state(false);
	let confirmed = $state(false);
	let note = $state('');
	let mode = $state<'walk' | 'bike'>('walk');
	let selectedEvidence = $state<Record<string, string[]>>({});
	let published = $state(false);
	const sourceKey = $derived(`${brief.id}:${brief.version}`);
	const current = $derived(
		!!report && !invalidated && !published && loadedKey === sourceKey && loadedTour === tourID
	);
	const route = $derived(report?.routes[mode]);
	const markers = $derived(
		(report?.tour?.stops ?? []).map((stop) => ({
			lat: stop.place.arrival_point?.point?.latitude ?? stop.place.latitude,
			lng: stop.place.arrival_point?.point?.longitude ?? stop.place.longitude,
			label: stop.place.name,
			position: stop.position
		}))
	);
	const descriptions: Record<string, string> = {
		reused_published: 'Already published; content is reused',
		reused_draft: 'Prepared draft; selected dependencies become public',
		new_entity: 'Unmaterialized candidate; remains private',
		proposed_shared_edit: 'Proposed shared edit; remains unapplied',
		proposed_draft_edit: 'Proposed draft edit; remains unapplied',
		unavailable: 'Unavailable; inspect the blocking tasks'
	};
	onMount(() => {
		let active = true;
		listTours('')
			.then((result) => {
				if (active) tours = result.items;
			})
			.catch(() => {
				if (active)
					error =
						'Could not load prepared tours. Reload this page to retry; your brief is still saved.';
			});
		return () => {
			active = false;
		};
	});
	function fail(e: unknown, fallback: string) {
		if (e instanceof ApiError && e.status === 409) {
			invalidated = true;
			confirmed = false;
			error =
				'This package changed. Reload and review it before deciding. Your brief text and decision note have been kept.';
		} else error = errorMessage(e, fallback);
	}
	async function inspect() {
		const key = sourceKey,
			id = brief.id,
			selected = tourID;
		busy = 'load';
		error = '';
		message = '';
		confirmed = false;
		try {
			const result = await getReviewPackage(id, selected);
			if (key === sourceKey && selected === tourID) {
				report = result;
				loadedKey = key;
				loadedTour = selected;
				invalidated = false;
				published = false;
				selectedEvidence = Object.fromEntries(
					result.visitor_stories
						.filter((story) => story.candidate_id)
						.map((story) => [story.candidate_id, []])
				);
				mode = result.tour?.modes[0] ?? 'walk';
			}
		} catch (e) {
			invalidated = true;
			fail(e, 'Could not load the review package.');
		} finally {
			busy = '';
		}
	}
	async function decide(action: 'publish' | 'needs_research' | 'declined') {
		if (!report || !current || busy) return;
		if (action !== 'publish' && !note.trim()) {
			error = 'Add a decision note before returning or declining this brief.';
			document.getElementById('package-note')?.focus();
			return;
		}
		if (
			action === 'publish' &&
			(!confirmed || !report.ready_to_publish || !report.research || !report.tour)
		)
			return;
		busy = action;
		error = '';
		message = '';
		try {
			if (action === 'publish') {
				await publishReviewPackage(brief.id, {
					version: report.version,
					run_id: report.research!.id,
					tour_id: report.tour!.id,
					note
				});
				published = true;
				message =
					'Package published. The reviewed tour revision and selected dependencies were released together.';
			} else {
				await getReviewPackage(brief.id, loadedTour, report.version);
				await decideRouteBrief(brief.id, report.brief.version, action, note.trim());
				invalidated = true;
				message =
					action === 'declined'
						? 'Brief declined. No content was published.'
						: 'Brief returned for research. No content was published.';
			}
			confirmed = false;
			await onchanged();
		} catch (e) {
			fail(e, 'Could not save the package decision. Reload the package before retrying.');
		} finally {
			busy = '';
		}
	}
	async function research(candidate = '') {
		if (!report || !current || busy) return;
		busy = 'research';
		error = '';
		message = '';
		confirmed = false;
		try {
			await getReviewPackage(brief.id, loadedTour, report.version);
			if (candidate) {
				await draftCandidate(candidate, selectedEvidence[candidate] ?? []);
				message =
					'Targeted history research requested. Existing editor text is unchanged; reload the package after the job finishes to inspect the new proposal.';
			} else if (report.research) {
				await resumeBriefResearch(brief.id, report.brief.version, report.research.id);
				message =
					'Research resumed from the first incomplete stage. Completed work and your unsaved brief text have been kept.';
			}
			invalidated = true;
		} catch (e) {
			fail(e, 'Could not request research. Existing edits have been kept.');
		} finally {
			busy = '';
		}
	}
	function href(url: string) {
		return /^https?:\/\//i.test(url) ? url : undefined;
	}
	function storyImage(momentID: string) {
		const moment = report?.entities.find((entry) => entry.key === `moment:${momentID}`)?.current as
			Partial<MomentDetail> | undefined;
		const image = moment?.hero_media;
		return image?.kind === 'image' && href(image.public_url) ? image : null;
	}
	function pendingCandidate(id: string) {
		return report?.entities.some(
			(entry) => entry.candidate?.id === id && entry.candidate.status === 'pending'
		);
	}
	function uncertainties(story: CuratorReviewPackage['visitor_stories'][number]) {
		const draft = report?.entities.find(
			(entry) => entry.candidate?.id === story.candidate_id
		)?.proposed;
		return Array.isArray(draft?.uncertainties)
			? draft.uncertainties.flatMap((value) =>
					value && typeof value === 'object' && 'text' in value && typeof value.text === 'string'
						? [value.text]
						: []
				)
			: [];
	}
</script>

<section
	aria-label="Curator package review"
	class="mt-8 min-w-0 border-t border-byl-border pt-6 text-sm"
>
	<h2 class="text-xl font-semibold">Review and publish a tour package</h2>
	<p class="mt-2 max-w-3xl text-byl-muted">
		Compare the visitor stories with their retained evidence, then review the selected path and
		every affected record. Unsaved brief text stays in the form above.
	</p>
	<div class="mt-4 flex flex-wrap items-end gap-3">
		<div class="max-w-full min-w-0">
			<label for="package-tour" class="block font-medium">Prepared tour</label>
			<select
				id="package-tour"
				bind:value={tourID}
				disabled={!!busy}
				onchange={() => (confirmed = false)}
				class="mt-1 w-full max-w-md rounded-md border border-byl-border-control bg-byl-surface px-3 py-2"
			>
				<option value="">Inspect research without a prepared tour</option>
				{#each tours as tour (tour.id)}
					<option value={tour.id}
						>{tour.title} · {tour.status}{tour.owner_account_id ? ' · personal route' : ''}</option
					>
				{/each}
			</select>
		</div>
		<button
			type="button"
			class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
			disabled={!!busy}
			onclick={inspect}>{busy === 'load' ? 'Loading package…' : 'Reload review package'}</button
		>
	</div>
	{#if error}<p role="alert" class="mt-3 text-byl-danger">{error}</p>{/if}
	{#if message}<p role="status" class="mt-3">{message}</p>{/if}
	{#if report && report.brief.id === brief.id}
		<p class="mt-4 font-medium">Package state: {report.state.replaceAll('_', ' ')}</p>
		{#if !current && !published}<p class="mt-2 text-byl-warning">
				This view needs a reload before any decision. Your form edits and decision note are
				retained.
			</p>{/if}
		{#if report.tasks.length}
			<section aria-label="Blocking review tasks" class="mt-4 border-l-2 border-byl-warning pl-3">
				<h3 class="font-semibold">Resolve before publication</h3>
				<ul class="mt-2 list-disc space-y-2 pl-5">
					{#each report.tasks as task, index (`${task.code}:${task.entity_key}:${index}`)}<li>
							{task.detail} <span class="text-byl-muted">({task.code.replaceAll('_', ' ')})</span>
						</li>{/each}
				</ul>
			</section>
		{/if}
		{#if report.tour}
			<section aria-label="Visitor itinerary preview" class="mt-6 min-w-0 space-y-3">
				<h3 class="text-lg font-semibold">Visitor preview: {report.tour.title}</h3>
				<p>{report.tour.summary}</p>
				<label class="block"
					>Preview travel mode
					<select
						bind:value={mode}
						class="ml-2 rounded-md border border-byl-border-control bg-byl-surface px-3 py-2"
					>
						{#each report.tour.modes as m (m)}<option value={m}
								>{m === 'walk' ? 'Walking' : 'Cycling'}</option
							>{/each}
					</select>
				</label>
				<Map
					{markers}
					line={(route?.geometry?.coordinates ?? []) as [number, number][]}
					height="22rem"
				/>
				<p>
					<strong>Selected mode:</strong>
					{route?.journey?.state.replaceAll('_', ' ') ?? 'No computed path'}. {route?.quality
						?.input_current
						? 'Assessment matches the current inputs.'
						: 'Current path assessment is missing or stale.'}
				</p>
				{#if route?.journey?.usable}<p>
						Travel: {Math.ceil((route.journey.travel_duration_s ?? 0) / 60)} min. Stop and narration assumptions
						still apply.
					</p>{/if}
				{#each Object.entries(report.path_reports) as [travelMode, assessment] (travelMode)}
					{#if assessment}<details>
							<summary class="cursor-pointer font-medium"
								>{travelMode === 'walk' ? 'Walking' : 'Cycling'} retained path evidence</summary
							>
							<p class="mt-2">
								{assessment.provider} · {assessment.status} · {assessment.checked_at}
							</p>
							{#each assessment.inspections as inspection, index (`${inspection.segment_id}:${index}`)}<blockquote
									class="mt-2 border-l-2 border-byl-border pl-3"
								>
									<p>{inspection.quote}</p>
									<a
										href={href(inspection.url)}
										target="_blank"
										rel="noreferrer"
										class="mt-1 block break-all underline">Inspected route source</a
									>
									<p class="text-byl-muted">
										{inspection.segment_id} · {inspection.purpose} · {inspection.mode}
									</p>
								</blockquote>{/each}
							{#each assessment.issues as issue, index (`${issue.code}:${index}`)}<p class="mt-2">
									{issue.detail}
								</p>{/each}
						</details>{/if}
				{/each}
				<ol class="list-decimal space-y-1 pl-5">
					{#each report.tour.stops as stop (stop.id)}<li>
							{stop.place.name} · {stop.featured_moment?.title ?? 'Historical account missing'}
						</li>{/each}
				</ol>
				<a
					href="/admin/tours/{report.tour.id}"
					target="_blank"
					rel="noreferrer"
					class="inline-block underline">Edit prepared tour in a separate tab</a
				>
			</section>
		{/if}
		<section aria-label="Stories and evidence" class="mt-6">
			<h3 class="text-lg font-semibold">Visitor stories and evidence</h3>
			{#each report.visitor_stories as story (story.position)}
				{@const image = storyImage(story.moment_id)}
				<article class="mt-4 min-w-0 border-t border-byl-border pt-4">
					<h4 class="text-lg font-semibold">{story.position}. {story.title}</h4>
					{#if story.text_source === 'captured_partial'}<p class="mt-2 text-byl-warning">
							Captured partial text. The current story needs fresh review before release.
						</p>{/if}
					<div class="mt-3 grid min-w-0 gap-5 xl:grid-cols-2">
						<div class="min-w-0">
							{#if image}<figure class="mb-3">
									<img
										src={image.public_url}
										alt={image.caption}
										class="max-h-96 w-full object-contain"
										loading="lazy"
									/>
									<figcaption class="mt-2 text-xs text-byl-muted">
										{image.caption} · {image.credit} · {image.license}
									</figcaption>
								</figure>{/if}
							<p class="break-words whitespace-pre-wrap">{story.body}</p>
							<div class="mt-3 flex flex-wrap gap-3">
								{#if story.moment_id}<a
										href="/admin/moments/{story.moment_id}"
										target="_blank"
										rel="noreferrer"
										class="underline">Edit this history in a separate tab</a
									>{/if}
								{#if story.candidate_id}<a
										href="/admin/queue/{story.candidate_id}"
										target="_blank"
										rel="noreferrer"
										class="underline">Review this candidate in a separate tab</a
									>{/if}
							</div>
						</div>
						<aside
							aria-label={`Evidence for story ${story.position}`}
							class="min-w-0 border-l-2 border-byl-border pl-3"
						>
							<h5 class="font-semibold">Sources and uncertainty</h5>
							{#each story.source_urls ?? [] as url, index (url)}<a
									href={href(url)}
									target="_blank"
									rel="noreferrer"
									class="mt-2 block break-all underline">Historical source {index + 1}</a
								>{/each}
							{#each uncertainties(story) as uncertainty, index (index)}<p
									class="mt-2 text-byl-warning"
								>
									{uncertainty}
								</p>{/each}
							{#each report.tasks.filter( (task) => [story.moment_id, story.candidate_id, `moment:${story.moment_id}`, `candidate:${story.candidate_id}`].includes(task.entity_key) ) as task, index (index)}<p
									class="mt-2 text-byl-warning"
								>
									{task.detail}
								</p>{/each}
							{#each story.evidence_ids ?? [] as id (id)}
								{@const source = report.evidence.find((item) => item.id === id)}
								{#if source?.result}<div class="mt-3 space-y-2">
										<a
											href={href(source.url)}
											target="_blank"
											rel="noreferrer"
											class="font-medium break-all underline">{source.result.title || source.url}</a
										>
										<p class="text-byl-muted">
											{source.result.publisher} · {source.result.outcome.replaceAll('_', ' ')} · {source
												.result.retrieved_at}
										</p>
										{#each source.result.passages as passage (passage.reference)}<blockquote
												class="border-l-2 border-byl-border pl-2"
											>
												<p class="break-words whitespace-pre-wrap">{passage.text}</p>
												<p class="text-xs text-byl-muted">{passage.reference}</p>
											</blockquote>{/each}
										<p class="text-xs text-byl-muted">
											Retained passages are untrusted source material. Retrieval does not grant
											media reuse rights.
										</p>
										{#if pendingCandidate(story.candidate_id) && source.result.outcome === 'inspected_page'}<label
												class="flex gap-2"
												><input
													type="checkbox"
													bind:group={selectedEvidence[story.candidate_id]}
													value={id}
												/>Use this inspected source for targeted research</label
											>{/if}
									</div>{:else}<p class="mt-2 text-byl-warning">
										Selected evidence is unavailable.
									</p>{/if}
							{:else}<p class="mt-2 text-byl-muted">
									No retained passages are linked to this story. Check its historical citations and
									the package tasks.
								</p>{/each}
							{#if pendingCandidate(story.candidate_id)}<button
									type="button"
									disabled={!!busy || !current || !selectedEvidence[story.candidate_id]?.length}
									onclick={() => research(story.candidate_id)}
									class="mt-3 underline disabled:opacity-50"
									>Request targeted history research</button
								>{/if}
						</aside>
					</div>
				</article>
			{:else}<p class="mt-3 text-byl-muted">
					No assembled visitor stories yet. Use the research controls above and resolve the blocking
					tasks.
				</p>{/each}
		</section>
		<details class="mt-6">
			<summary class="cursor-pointer text-lg font-semibold"
				>Affected records and proposed changes</summary
			>
			<p class="mt-2">
				Selected prepared drafts become public together. Already published records are reused.
				Contextual candidates and proposed shared edits are not applied by package approval.
			</p>
			<ul class="mt-3 space-y-3">
				{#each report.entities as entity (entity.key)}<li
						class="min-w-0 border-l-2 border-byl-border pl-3"
					>
						<p class="font-medium break-words">
							{String(
								entity.current?.title ??
									entity.current?.name ??
									entity.current?.caption ??
									entity.proposed?.title ??
									entity.kind
							)} · {entity.required ? 'Selected dependency' : 'Contextual lead'}
						</p>
						<p>{descriptions[entity.disposition]}</p>
						{#if entity.candidate}<a
								href="/admin/queue/{entity.candidate.id}"
								target="_blank"
								rel="noreferrer"
								class="underline">Open candidate review</a
							>{/if}
						{#each entity.changes as change (change.field)}<details class="mt-2">
								<summary class="cursor-pointer"
									>Proposed {change.field.replaceAll('_', ' ')} change</summary
								>
								<p class="mt-2 break-words whitespace-pre-wrap">
									<strong>Current:</strong>
									{JSON.stringify(change.before)}
								</p>
								<p class="mt-2 break-words whitespace-pre-wrap">
									<strong>Proposed:</strong>
									{JSON.stringify(change.after)}
								</p>
							</details>{/each}
					</li>
				{/each}
			</ul>
		</details>
		<section aria-label="Package decision" class="mt-6 max-w-3xl border-t border-byl-border pt-4">
			<h3 class="text-lg font-semibold">Curator decision</h3>
			<label for="package-note" class="mt-3 block font-medium"
				>Decision note (visible to the brief owner)</label
			>
			<textarea
				id="package-note"
				bind:value={note}
				maxlength="2000"
				rows="3"
				class="mt-1 w-full rounded-md border border-byl-border-control bg-byl-surface px-3 py-2"
			></textarea>
			<label class="mt-3 flex items-start gap-2"
				><input
					type="checkbox"
					bind:checked={confirmed}
					disabled={!current || !report.ready_to_publish || !!busy}
				/>I reviewed the stories, evidence, route quality and selected dependencies in this package.</label
			>
			<div class="mt-4 flex flex-wrap gap-3">
				<button
					type="button"
					disabled={!!busy || !current || !report.ready_to_publish || !confirmed}
					onclick={() => decide('publish')}
					class="rounded-md bg-byl-action px-3 py-2 text-byl-action-text disabled:opacity-50"
					>Approve and publish reviewed package</button
				>
				<button
					type="button"
					disabled={!!busy ||
						!current ||
						!['draft', 'processing', 'review'].includes(report.brief.status)}
					onclick={() => decide('needs_research')}
					class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
					>Return for research</button
				>
				<button
					type="button"
					disabled={!!busy ||
						!current ||
						!['draft', 'needs_research', 'review'].includes(report.brief.status)}
					onclick={() => decide('declined')}
					class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
					>Decline brief</button
				>
				{#if report.research && report.brief.status === 'needs_research' && ['needs_research', 'cancelled'].includes(report.research.status)}<button
						type="button"
						disabled={!!busy || !current}
						onclick={() => research()}
						class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
						>Resume incomplete research</button
					>{/if}
			</div>
			{#if !report.ready_to_publish}<p class="mt-3 text-byl-muted">
					Publication stays disabled until every review task is resolved and a prepared tour is
					selected.
				</p>{/if}
			{#if published && report.tour}<a
					href="/t/{report.tour.slug}"
					class="mt-3 inline-block underline">Open the published tour</a
				>{/if}
		</section>
	{/if}
</section>
