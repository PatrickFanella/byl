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

	const rawText = $derived(String(payload.raw_text ?? ''));
	const readOnly = $derived(
		readOnlyEntries(candidate.kind, payload).filter(([k]) => k !== 'raw_text')
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
			const edited = applyFields(placeId ? { ...payload, place_id: placeId } : payload, fields);
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

	async function draft() {
		busy = 'draft';
		notice = { kind: 'info', message: 'Asking the model for a draft…' };
		try {
			const queued = await draftCandidate(candidate.id);
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
				const fresh = await getCandidate(candidate.id);
				if (fresh.origin === 'draft' && fresh.updated_at !== candidate.updated_at) {
					await invalidateAll();
					notice = { kind: 'success', message: 'Draft ready. Review the text before approving.' };
					return;
				}
			}
			notice = {
				kind: 'info',
				message: 'The draft is still running; reload this page in a minute or check Jobs.'
			};
		} catch (e) {
			if (e instanceof ApiError && e.status === 503)
				notice = {
					kind: 'error',
					message: 'No LLM provider is configured on the server (LLM_PROVIDER).'
				};
			else fail(e, 'Could not queue the draft.');
		} finally {
			busy = '';
		}
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
	<a href="/admin/queue" class="text-sm text-stone-500 hover:underline">← Queue</a>
	<h1 class="text-2xl font-semibold">
		{String(payload.title ?? payload.name ?? payload.caption ?? 'Candidate')}
	</h1>
	<StatusPill status={candidate.kind} />
	<StatusPill status={candidate.origin} />
	<StatusPill status={candidate.status} />
</div>
<p class="mt-1 text-sm text-stone-500">
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
	<p class="mt-2 rounded-md bg-stone-100 p-3 text-sm">Review note: {candidate.review_note}</p>
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
				<span class="text-stone-700">Place id</span>
				<input
					class="mt-1 w-full rounded-md border-stone-300 font-mono text-xs"
					bind:value={placeId}
				/>
			</label>
		{/if}
		<PayloadForm bind:fields errors={fieldErrors} />
		{#if readOnly.length}
			<details class="rounded-md border border-stone-200 bg-white p-3 text-sm">
				<summary class="cursor-pointer">Other payload fields</summary>
				<dl class="mt-2 grid grid-cols-[max-content_1fr] gap-x-4 gap-y-1 font-mono text-xs">
					{#each readOnly as [k, v] (k)}
						<dt class="text-stone-500">{k}</dt>
						<dd class="break-all">{typeof v === 'string' ? v : JSON.stringify(v)}</dd>
					{/each}
				</dl>
			</details>
		{/if}
	</div>
	<aside class="space-y-3">
		<div class="rounded-xl border border-stone-200 bg-white p-4">
			<div class="flex flex-wrap gap-2">
				<button
					class="rounded-md bg-emerald-700 px-3 py-1.5 text-sm text-white disabled:opacity-50"
					disabled={!isPending || !!busy}
					onclick={approve}>{busy === 'approve' ? 'Approving…' : 'Approve'}</button
				>
				<button
					class="rounded-md border border-stone-300 px-3 py-1.5 text-sm disabled:opacity-50"
					disabled={!isPending || !!busy}
					onclick={() => (rejectOpen = true)}>Reject</button
				>
				{#if candidate.kind === 'moment'}
					<button
						class="rounded-md border border-indigo-300 bg-indigo-50 px-3 py-1.5 text-sm text-indigo-900 disabled:opacity-50"
						disabled={!isPending || !!busy}
						onclick={draft}>{busy === 'draft' ? 'Drafting…' : 'Draft with AI'}</button
					>
				{/if}
			</div>
			<p class="mt-2 text-xs text-stone-500">
				Approving creates a draft {candidate.kind}; nothing is published until you publish it.
			</p>
		</div>
		{#if rawText}
			<div class="rounded-xl border border-stone-200 bg-white p-4">
				<h2 class="text-sm font-semibold">Source text</h2>
				<pre
					class="mt-2 max-h-[32rem] overflow-auto text-xs whitespace-pre-wrap text-stone-700">{rawText}</pre>
			</div>
		{/if}
		{#if payload.image_url || payload.media_url}
			<div class="rounded-xl border border-stone-200 bg-white p-2">
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
		>Note<textarea class="mt-1 w-full rounded-md border-stone-300" rows="3" bind:value={rejectNote}
		></textarea></label
	>
	<div class="mt-4 flex justify-end gap-2">
		<button
			class="rounded-md border border-stone-300 px-3 py-1.5 text-sm"
			onclick={() => (rejectOpen = false)}>Cancel</button
		>
		<button
			class="rounded-md bg-stone-900 px-3 py-1.5 text-sm text-white"
			onclick={reject}
			disabled={!!busy}>Reject</button
		>
	</div>
</Dialog>
