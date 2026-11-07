<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import PayloadForm from '#lib/components/PayloadForm.svelte';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import { fieldsFor, applyFields, readOnlyEntries, type Field } from '#lib/payload.ts';
	import {
		approveCandidate,
		rejectCandidate,
		draftCandidate,
		getCandidate,
		getJob,
		listRouteBriefs,
		listBriefEvidence,
		type RouteBrief,
		type SourceEvidence,
		type HistoricalDraft,
		sleep
	} from '#lib/admin.ts';
	import { ApiError } from '#lib/api/index.ts';
	import { when } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let candidate = $derived(data.candidate);
	let payload = $derived((candidate.payload ?? {}) as Record<string, unknown>);
	let fields: Field[] = $derived(fieldsFor(candidate.kind, payload));
	// Held apart from payload so typing a place id never recomputes (and discards) the field edits.
	// Writable derived: it follows the candidate until the curator overrides it.
	let placeId = $derived(String(payload.place_id ?? candidate.target_place_id ?? ''));

	let busy = $state('');
	let notice = $state({ kind: 'info' as 'info' | 'error' | 'success', message: '' });
	let fieldErrors: Record<string, string> = $state({});
	let rejectOpen = $state(false);
	let rejectNote = $state('');
	let approvedId: string | null = $state(null);
	let draftOpen = $state(false);
	let loadingSources = $state(false);
	let briefs = $state<RouteBrief[]>([]);
	let evidence = $state<SourceEvidence[]>([]);
	let briefId = $state('');
	let selectedEvidence = $state<string[]>([]);
	let newProposal = $state<HistoricalDraft | null>(null);
	let proposalExtras = $state<Record<string, unknown>>({});
	const storedProposals = $derived(
		Array.isArray(payload.generated_drafts) ? (payload.generated_drafts as HistoricalDraft[]) : []
	);
	const proposal = $derived(newProposal ?? storedProposals.at(-1) ?? null);

	const rawText = $derived(String(payload.raw_text ?? ''));
	const readOnly = $derived(
		readOnlyEntries(candidate.kind, { ...payload, ...proposalExtras }).filter(
			([k]) => k !== 'raw_text'
		)
	);
	const isPending = $derived(candidate.status === 'pending');

	function fail(e: unknown, fallback: string) {
		if (e instanceof ApiError) {
			notice = { kind: 'error', message: e.message };
			fieldErrors = e.fields ?? {};
		} else {
			notice = { kind: 'error', message: fallback };
		}
	}

	async function approve() {
		busy = 'approve';
		notice = { kind: 'info', message: '' };
		fieldErrors = {};
		try {
			const edited = applyFields(
				{ ...payload, ...proposalExtras, ...(placeId ? { place_id: placeId } : {}) },
				fields
			);
			const result = await approveCandidate(candidate.id, edited);
			approvedId = result.approved_entity_id ?? null;
			notice = {
				kind: 'success',
				message: `Approved. A draft ${candidate.kind} was created; publish it from its page.`
			};
			await invalidateAll();
		} catch (e) {
			fail(e, 'Approval failed.');
		} finally {
			busy = '';
		}
	}

	async function reject() {
		busy = 'reject';
		try {
			await rejectCandidate(candidate.id, rejectNote);
			rejectOpen = false;
			notice = { kind: 'success', message: 'Rejected.' };
			await invalidateAll();
		} catch (e) {
			fail(e, 'Rejection failed.');
		} finally {
			busy = '';
		}
	}

	async function openDraft() {
		draftOpen = true;
		loadingSources = true;
		notice = { kind: 'info', message: '' };
		try {
			briefs = (await listRouteBriefs()).items;
		} catch (e) {
			fail(e, 'Could not load research briefs.');
		} finally {
			loadingSources = false;
		}
	}
	async function loadEvidence() {
		selectedEvidence = [];
		evidence = [];
		if (!briefId) return;
		const selectedBrief = briefId;
		loadingSources = true;
		try {
			const result = await listBriefEvidence(selectedBrief);
			if (briefId === selectedBrief)
				evidence = result.items.filter(
					(e) =>
						['inspected_page', 'catalog_only'].includes(e.result?.outcome ?? '') &&
						!!e.result?.passages?.length &&
						!!e.result?.content_sha256
				);
		} catch (e) {
			fail(e, 'Could not load inspected sources.');
		} finally {
			loadingSources = false;
		}
	}
	async function draft() {
		if (!selectedEvidence.length || selectedEvidence.length > 5) return;
		busy = 'draft';
		draftOpen = false;
		notice = { kind: 'info', message: 'Preparing a proposal from the selected inspected sources…' };
		try {
			const queued = await draftCandidate(candidate.id, selectedEvidence);
			for (let i = 0; i < 45; i++) {
				await sleep(2000);
				const job = await getJob(queued.job.id);
				if (job.status === 'failed') {
					notice = {
						kind: 'error',
						message: job.last_error || 'Drafting failed. Check Jobs for details.'
					};
					return;
				}
				if (job.status === 'done') {
					const fresh = await getCandidate(candidate.id);
					const history = (fresh.payload as Record<string, unknown>)?.generated_drafts;
					if (Array.isArray(history) && history.length) {
						newProposal = history.at(-1) as HistoricalDraft;
						notice = {
							kind: 'success',
							message:
								'Proposal ready below. Your editor text is unchanged; compare the evidence before applying it.'
						};
					} else
						notice = {
							kind: 'error',
							message: 'No draft proposal was retained. Check Jobs before retrying.'
						};
					return;
				}
			}
			notice = {
				kind: 'info',
				message:
					'The proposal is still running; check Jobs. Save or copy your editor text before reloading.'
			};
		} catch (e) {
			fail(e, 'Could not queue the inspected draft.');
		} finally {
			busy = '';
		}
	}
	function applyProposal() {
		if (!proposal) return;
		const current = applyFields({ ...payload, ...proposalExtras }, fields);
		const sources = Array.isArray(current.sources) ? [...current.sources] : [];
		for (const source of proposal.sources) {
			if (!sources.some((s) => s.url === source.url)) sources.push(source);
		}
		proposalExtras = { sources, topic_ids: proposal.topic_ids };
		const notes = [
			current.draft_notes,
			proposal.notes,
			...proposal.uncertainties.map((u) => u.text)
		]
			.filter(Boolean)
			.join('\n');
		fields = fieldsFor(candidate.kind, {
			...current,
			title: proposal.title,
			body: proposal.body,
			date_start: proposal.date_start,
			date_end: proposal.date_end,
			date_precision: proposal.date_precision,
			date_display: proposal.date_display,
			draft_notes: notes
		});
		notice = {
			kind: 'info',
			message:
				'Proposal applied to the editor. Review and edit it before approval; it is still unpublished.'
		};
	}

	function entityLink(): string {
		if (!approvedId && !candidate.approved_entity_id) return '';
		const id = approvedId ?? candidate.approved_entity_id ?? '';
		if (candidate.kind === 'place') return `/admin/places/${id}`;
		if (candidate.kind === 'moment') return `/admin/moments/${id}`;
		return `/admin/media`;
	}
</script>

<div class="flex flex-wrap items-center gap-3">
	<a href="/admin/queue" class="text-sm text-byl-muted hover:underline">← Queue</a>
	<h1 class="text-2xl font-semibold">
		{String(payload.title ?? payload.name ?? payload.caption ?? 'Candidate')}
	</h1>
	<StatusPill status={candidate.kind} />
	<StatusPill status={candidate.origin} />
	<StatusPill status={candidate.status} />
</div>
<p class="mt-1 text-sm text-byl-muted">
	{candidate.source_name ?? 'manual'}{candidate.external_id ? ` · ${candidate.external_id}` : ''} · created
	{when(candidate.created_at)}
	{#if candidate.target_place_id}
		· target place <a class="underline" href="/admin/places/{candidate.target_place_id}"
			>{candidate.target_place_id}</a
		>
	{/if}
	{#if payload.source_url}
		· <a class="underline" href={String(payload.source_url)} target="_blank" rel="noreferrer"
			>source</a
		>
	{/if}
</p>

<div class="mt-4"><Notice kind={notice.kind} message={notice.message} /></div>
{#if entityLink()}
	<p class="mt-2 text-sm">
		<a class="underline" href={entityLink()}>Open the created {candidate.kind}</a>
	</p>
{/if}
{#if candidate.review_note}
	<p class="mt-2 rounded-md bg-byl-surface p-3 text-sm">Review note: {candidate.review_note}</p>
{/if}

<div class="mt-6 grid gap-6 lg:grid-cols-3">
	<div class="space-y-4 lg:col-span-2">
		{#if candidate.kind === 'moment' && !placeId}
			<Notice
				kind="error"
				message="This moment has no place. Set place_id below (a published place id) before approving, or approve the matching place candidate first."
			/>
		{/if}
		{#if candidate.kind === 'moment'}
			<label class="block text-sm">
				<span class="text-byl-text">Place id</span>
				<input
					class="mt-1 w-full rounded-md border-byl-border-control font-mono text-xs"
					bind:value={placeId}
				/>
			</label>
		{/if}
		<PayloadForm bind:fields errors={fieldErrors} />
		{#if proposal}
			<section
				class="min-w-0 rounded-lg border border-byl-border bg-byl-background p-4 text-sm break-words"
				aria-label="Generated proposal"
			>
				<h2 class="text-lg font-semibold">Generated proposal</h2>
				<p class="mt-1 text-byl-muted">
					Compare these claims with their inspected passages. Citation checks do not establish
					historical truth or image rights.
				</p>
				<h3 class="mt-3 font-semibold">{proposal.title}</h3>
				<p class="text-byl-muted">{proposal.date_display || 'Date unresolved'}</p>
				<details class="mt-2 text-byl-muted">
					<summary class="cursor-pointer">Title and date evidence</summary>
					{#each [...proposal.title_citations, ...proposal.date_citations] as citation, i (i)}
						{@const source = proposal.evidence_manifest.find((e) => e.id === citation.evidence_id)}
						<blockquote class="mt-2 border-l-2 border-byl-border pl-3">
							<p>{citation.quote}</p>
							<p>
								<a class="underline" href={source?.url} target="_blank" rel="noreferrer"
									>{source?.title || 'Inspected source'}</a
								>
								· {citation.passage}
							</p>
						</blockquote>
					{/each}
				</details>
				{#each proposal.claims as claim, i (i)}
					<div class="mt-4">
						<p>{claim.text}</p>
						{#if claim.kind === 'testimony'}<p class="mt-1 text-byl-muted">
								Attributed account: {claim.attribution}
							</p>{/if}
						{#each claim.citations as citation, j (j)}
							{@const source = proposal.evidence_manifest.find(
								(e) => e.id === citation.evidence_id
							)}
							<blockquote class="mt-2 border-l-2 border-byl-border pl-3 text-byl-muted">
								<p>{citation.quote}</p>
								<p>
									<a class="underline" href={source?.url} target="_blank" rel="noreferrer"
										>{source?.title || 'Inspected source'}</a
									>
									· {citation.passage}
								</p>
							</blockquote>
						{/each}
					</div>
				{/each}
				{#if proposal.uncertainties.length || proposal.notes}
					<h3 class="mt-4 font-semibold">Uncertainty and research gaps</h3>
					{#each proposal.uncertainties as uncertainty, i (i)}
						<p class="mt-2">{uncertainty.text}</p>
						{#each uncertainty.citations ?? [] as citation, j (j)}
							{@const source = proposal.evidence_manifest.find(
								(e) => e.id === citation.evidence_id
							)}
							<blockquote class="mt-2 border-l-2 border-byl-border pl-3 text-byl-muted">
								<p>{citation.quote}</p>
								<p>
									<a class="underline" href={source?.url} target="_blank" rel="noreferrer"
										>{source?.title || 'Inspected source'}</a
									>
									· {citation.passage}
								</p>
							</blockquote>
						{/each}
					{/each}
					<p class="mt-2 whitespace-pre-wrap text-byl-muted">{proposal.notes}</p>
				{/if}
				<button
					class="mt-4 rounded-md border border-byl-border-control px-3 py-1.5 disabled:opacity-50"
					disabled={!isPending || !!busy}
					onclick={applyProposal}>Apply proposal to editor</button
				>
			</section>
		{/if}

		{#if readOnly.length}
			<details class="rounded-md border border-byl-border bg-byl-background p-3 text-sm">
				<summary class="cursor-pointer">Other payload fields</summary>
				<dl class="mt-2 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 font-mono text-xs">
					{#each readOnly as [k, v] (k)}
						<dt class="text-byl-muted">{k}</dt>
						<dd class="break-all">{typeof v === 'string' ? v : JSON.stringify(v)}</dd>
					{/each}
				</dl>
			</details>
		{/if}
	</div>
	<aside class="space-y-3">
		{#if candidate.kind === 'moment' || candidate.kind === 'place'}
			<section class="rounded-lg border border-byl-border bg-byl-background p-4 text-sm">
				<h2 class="font-semibold">Before this place can go live</h2>
				<ul class="mt-2 list-disc space-y-2 pl-5 text-byl-muted">
					<li>
						A picture with a precise caption, creator credit, original source, and reusable license.
					</li>
					<li>
						A history with at least 250 words and three paragraphs, supported by named, linked
						sources.
					</li>
					<li>One place record for the physical location; separate moments for distinct events.</li>
					<li>
						Check uncertain dates and claims against the sources. Short evidence stays a draft.
					</li>
				</ul>
				{#if Array.isArray(payload.review_gaps) && payload.review_gaps.length}
					<h3 class="mt-3 font-semibold">Draft research gaps</h3>
					<ul class="mt-1 list-disc pl-5 text-byl-muted">
						{#each payload.review_gaps as gap, i (i)}<li>{String(gap)}</li>{/each}
					</ul>
				{/if}
			</section>
		{/if}
		<div class="rounded-lg border border-byl-border bg-byl-background p-4">
			<div class="flex flex-wrap gap-2">
				<button
					class="rounded-md bg-byl-success-surface px-3 py-1.5 text-sm text-byl-action-text disabled:opacity-50"
					disabled={!isPending || !!busy}
					onclick={approve}>{busy === 'approve' ? 'Approving…' : 'Approve'}</button
				>
				<button
					class="rounded-md border border-byl-border-control px-3 py-1.5 text-sm disabled:opacity-50"
					disabled={!isPending || !!busy}
					onclick={() => (rejectOpen = true)}>Reject</button
				>
				{#if candidate.kind === 'moment'}
					<button
						class="rounded-md border border-byl-info bg-byl-info-surface px-3 py-1.5 text-sm text-byl-info disabled:opacity-50"
						disabled={!isPending || !!busy}
						onclick={openDraft}>{busy === 'draft' ? 'Drafting…' : 'Draft with AI'}</button
					>
				{/if}
			</div>
			<p class="mt-2 text-xs text-byl-muted">
				Approving creates a draft {candidate.kind}; nothing is published until you publish it.
			</p>
		</div>
		{#if rawText}
			<div class="rounded-lg border border-byl-border bg-byl-background p-4">
				<h2 class="text-sm font-semibold">Submitted or imported text</h2>
				<pre
					class="mt-2 max-h-[32rem] overflow-auto text-xs whitespace-pre-wrap text-byl-text">{rawText}</pre>
			</div>
		{/if}
		{#if payload.image_url || payload.media_url}
			<div class="rounded-lg border border-byl-border bg-byl-background p-2">
				<img
					src={String(payload.media_url ?? payload.image_url)}
					alt=""
					class="max-h-64 w-full rounded object-contain"
				/>
			</div>
		{/if}
	</aside>
</div>

<Dialog bind:open={rejectOpen} title="Reject this candidate">
	<label class="block text-sm"
		>Note<textarea
			class="mt-1 w-full rounded-md border-byl-border-control"
			rows="3"
			bind:value={rejectNote}></textarea></label
	>
	<div class="mt-4 flex justify-end gap-2">
		<button
			class="rounded-md border border-byl-border-control px-3 py-1.5 text-sm"
			onclick={() => (rejectOpen = false)}>Cancel</button
		>
		<button
			class="rounded-md bg-byl-action px-3 py-1.5 text-sm text-byl-action-text"
			onclick={reject}
			disabled={!!busy}>Reject</button
		>
	</div>
</Dialog>

<Dialog bind:open={draftOpen} title="Choose inspected sources">
	<p class="mb-3 text-sm text-byl-muted">
		Select up to five inspected records from a research brief. Submitted notes and search snippets
		are excluded.
	</p>
	<label class="block text-sm"
		>Research brief
		<select
			class="mt-1 w-full rounded-md border-byl-border-control"
			bind:value={briefId}
			onchange={loadEvidence}
			disabled={loadingSources}
		>
			<option value="">Choose a brief</option>
			{#each briefs as brief (brief.id)}<option value={brief.id}
					>{brief.submitted?.topic} · {brief.submitted?.area}</option
				>{/each}
		</select>
	</label>
	{#if briefId && !loadingSources && !evidence.length}
		<p class="mt-3 text-sm">
			No inspected passages are available. <a class="underline" href="/admin/briefs/{briefId}"
				>Inspect source pages in this brief</a
			> before drafting.
		</p>
	{/if}
	{#if loadingSources}<p class="mt-3 text-sm" role="status">Loading research records…</p>{/if}
	<div class="mt-3 max-h-72 space-y-3 overflow-auto">
		{#each evidence as source (source.id)}
			<label class="flex items-start gap-2 text-sm">
				<input
					type="checkbox"
					value={source.id}
					bind:group={selectedEvidence}
					disabled={!!busy ||
						(selectedEvidence.length >= 5 && !selectedEvidence.includes(source.id ?? ''))}
				/>
				<span
					>{source.result?.title || source.url}<span class="block text-xs text-byl-muted"
						>{source.result?.outcome === 'catalog_only'
							? 'Catalogue description'
							: 'Inspected page'} · {when(source.result?.retrieved_at)}</span
					></span
				>
			</label>
		{/each}
	</div>
	<div class="mt-4 flex justify-end gap-2">
		<button
			class="rounded-md border border-byl-border-control px-3 py-1.5"
			onclick={() => (draftOpen = false)}>Cancel</button
		>
		<button
			class="rounded-md bg-byl-action px-3 py-1.5 text-byl-action-text disabled:opacity-50"
			disabled={loadingSources || !selectedEvidence.length}
			onclick={draft}>Generate proposal</button
		>
	</div>
</Dialog>
