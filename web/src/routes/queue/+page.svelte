<script lang="ts">
	import { goto } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import { when, excerpt } from '#lib/format.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();

	function apply(e: SubmitEvent) {
		e.preventDefault();
		const form = new FormData(e.currentTarget as HTMLFormElement);
		const parts: string[] = [];
		for (const k of ['status', 'kind', 'origin', 'source']) {
			const v = String(form.get(k) ?? '');
			if (v) parts.push(`${k}=${encodeURIComponent(v)}`);
		}
		goto(`/queue?${parts.join('&')}`);
	}

	function title(c: (typeof data.candidates)[number]): string {
		const p = c.payload as Record<string, unknown>;
		return String(p.title ?? p.name ?? p.caption ?? c.external_id ?? c.id);
	}
	function summary(c: (typeof data.candidates)[number]): string {
		const p = c.payload as Record<string, unknown>;
		return excerpt(String(p.body ?? p.raw_text ?? p.summary ?? p.credit ?? ''));
	}
</script>

<div class="flex items-end justify-between gap-4">
	<h1 class="text-2xl font-semibold">Review queue</h1>
	<a href="/imports" class="rounded-md bg-stone-900 px-3 py-1.5 text-sm text-white">Run an import</a
	>
</div>

<form class="mt-4 flex flex-wrap items-end gap-3 text-sm" onsubmit={apply}>
	<label
		>Status<select
			name="status"
			class="ml-2 rounded-md border-stone-300"
			value={data.filters.status}
			><option value="pending">pending</option><option value="approved">approved</option><option
				value="rejected">rejected</option
			><option value="">all</option></select
		></label
	>
	<label
		>Kind<select name="kind" class="ml-2 rounded-md border-stone-300" value={data.filters.kind}
			><option value="">any</option><option value="place">place</option><option value="moment"
				>moment</option
			><option value="media">media</option></select
		></label
	>
	<label
		>Origin<select
			name="origin"
			class="ml-2 rounded-md border-stone-300"
			value={data.filters.origin}
			><option value="">any</option><option value="import">import</option><option value="draft"
				>draft</option
			><option value="user">user</option></select
		></label
	>
	<label
		>Source<select
			name="source"
			class="ml-2 rounded-md border-stone-300"
			value={data.filters.source}
			><option value="">any</option>{#each data.sources as s (s.name)}<option value={s.name}
					>{s.name}</option
				>{/each}</select
		></label
	>
	<button class="rounded-md border border-stone-300 px-3 py-1.5" type="submit">Filter</button>
</form>

<ul class="mt-4 divide-y divide-stone-200 rounded-xl border border-stone-200 bg-white">
	{#each data.candidates as c (c.id)}
		<li>
			<a href="/queue/{c.id}" class="block px-4 py-3 hover:bg-stone-50">
				<div class="flex items-center gap-2 text-sm">
					<StatusPill status={c.kind} />
					<StatusPill status={c.origin} />
					<span class="font-medium">{title(c)}</span>
					<span class="ml-auto text-xs text-stone-500"
						>{c.source_name ?? ''} · {when(c.created_at)}</span
					>
					<StatusPill status={c.status} />
				</div>
				<div class="mt-1 text-sm text-stone-600">{summary(c)}</div>
			</a>
		</li>
	{:else}
		<li class="px-4 py-6 text-sm text-stone-500">No candidates match these filters.</li>
	{/each}
</ul>
{#if data.nextCursor}
	<div class="mt-3 text-right">
		<a
			class="text-sm underline"
			href="/queue?status={data.filters.status}&kind={data.filters.kind}&origin={data.filters
				.origin}&source={data.filters.source}&cursor={data.nextCursor}">Next page →</a
		>
	</div>
{/if}
