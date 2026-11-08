<script lang="ts">
	import { untrack, onDestroy } from 'svelte';
	import { getPlaceHappenings, type Happening } from '#lib/visitor.ts';
	import {
		noticeStatus,
		noticeLabels,
		noticeDates,
		noticeChecked,
		noticeSourceURL,
		noticeShape
	} from '#lib/happenings/presentation.ts';
	let { placeID, tripContext = false }: { placeID: string; tripContext?: boolean } = $props();
	let enabled = $state(false);
	let busy = $state(false);
	let items = $state<Happening[]>([]);
	let cursor = $state('');
	let message = $state('');
	let now = $state(Date.now());
	let generation = 0;
	onDestroy(() => {
		generation++;
	});
	$effect(() => {
		void placeID;
		untrack(() => {
			generation++;
			enabled = false;
			busy = false;
			items = [];
			cursor = '';
			message = '';
		});
	});
	$effect(() => {
		if (!enabled) return;
		const timer = window.setInterval(() => {
			now = Date.now();
		}, 60_000);
		return () => window.clearInterval(timer);
	});
	async function load(more = false) {
		const requestedPlace = placeID;
		const request = ++generation;
		const requestedCursor = more ? cursor : '';
		busy = true;
		message = '';
		if (!more) {
			items = [];
			cursor = '';
		}
		try {
			const page = await getPlaceHappenings(requestedPlace, requestedCursor);
			if (request !== generation || !enabled || placeID !== requestedPlace) return;
			if (
				!Array.isArray(page.items) ||
				typeof page.nextCursor !== 'string' ||
				page.items.length > 50 ||
				page.items.some((h) => !noticeShape(h, requestedPlace)) ||
				(page.nextCursor &&
					(!/^\d+$/.test(page.nextCursor) ||
						Number(page.nextCursor) <= Number(requestedCursor || 0) ||
						Number(page.nextCursor) > 10000))
			)
				throw new Error('Invalid notice page');
			items = [...(more ? items : []), ...page.items].filter(
				(h, i, all) => all.findIndex((v) => v.id === h.id) === i
			);
			cursor = page.nextCursor;
			now = Date.now();
		} catch {
			if (request !== generation || !enabled) return;
			items = [];
			cursor = '';
			message = 'Current notices are unavailable. The historical story and tour still work.';
		} finally {
			if (request === generation) busy = false;
		}
	}
	function toggle() {
		enabled = !enabled;
		if (enabled) void load();
		else {
			generation++;
			busy = false;
			items = [];
			cursor = '';
			message = '';
		}
	}
</script>

<section
	aria-label="Optional current notices"
	class="min-w-0 space-y-3 rounded-lg border border-byl-border p-4 text-sm [overflow-wrap:anywhere]"
>
	<button
		class="rounded border border-byl-border-control px-3 py-2"
		aria-expanded={enabled}
		onclick={toggle}>{enabled ? 'Hide current notices' : 'Show current notices'}</button
	>
	{#if enabled}
		<p class="text-byl-muted">
			Current announcements are separate from {tripContext
				? 'this started historical itinerary'
				: 'the historical stories'}. Dates do not provide an exact start time. Confirm plans with
			the organizer.
		</p>
		{#if message}<p role="status">{message}</p>{:else if !busy && items.length === 0}<p
				role="status"
			>
				No current notices have been published here.
			</p>{/if}
		{#each items as h (h.id)}
			{@const status = noticeStatus(h, now)}
			<article class="space-y-2 border-t border-byl-border pt-3">
				<p class="font-semibold">{noticeLabels[status]}</p>
				<h3 class="text-lg">{h.title}</h3>
				<p>{noticeDates(h)}</p>
				<p>{h.summary}</p>
				<p>Organizer: {h.organizer}</p>
				<p class="text-byl-muted">
					{noticeChecked(h)}. Source status: {h.source_health.replaceAll('_', ' ')}.
				</p>
				<p class="text-byl-muted">Review by {h.review_on}; expires {h.expires_on}.</p>
				<ul class="space-y-1" aria-label="Announcement sources">
					{#if h.sources.length === 0}<li>Source attribution unavailable.</li>{/if}
					{#each h.sources as source, index (index)}<li>
							{#if noticeSourceURL(source.url)}<a
									class="text-byl-action underline"
									href={noticeSourceURL(source.url)!}
									target="_blank"
									rel="noreferrer noopener"
									>{source.title || source.publisher || 'Organizer announcement'}</a
								>{:else}<span>Source link unavailable</span>{/if}
							<span class="block text-xs text-byl-muted"
								>{source.publisher} · captured {source.retrieved_on} · media reuse {source.license}</span
							>
						</li>{/each}
				</ul>
			</article>
		{/each}
		{#if busy}<p role="status">Loading current notices…</p>{/if}
		<div class="flex flex-wrap gap-3">
			<button
				class="rounded border border-byl-border-control px-3 py-2 disabled:opacity-50"
				disabled={busy}
				onclick={() => load()}>Refresh current notices</button
			>
			{#if cursor}<button
					class="rounded border border-byl-border-control px-3 py-2 disabled:opacity-50"
					disabled={busy}
					onclick={() => load(true)}>Show more notices</button
				>{/if}
		</div>
	{/if}
</section>
