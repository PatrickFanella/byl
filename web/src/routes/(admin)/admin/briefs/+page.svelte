<script lang="ts">
	import { when } from '#lib/format.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
</script>

<h1 class="text-2xl font-semibold">Route briefs</h1>
<p class="mt-2 max-w-2xl text-sm text-byl-muted">
	Start with a topic and area, then collect leads for research. Briefs stay private while curators
	work on them.
</p>
<a
	href="/admin/briefs/new"
	class="mt-4 inline-block rounded-md bg-byl-action px-4 py-2 text-sm text-byl-action-text"
	>New route brief</a
>
<form method="GET" class="mt-6 flex flex-wrap items-end gap-3 text-sm">
	<label
		>State<select
			name="status"
			class="ml-2 rounded-md border-byl-border-control"
			value={data.status}
		>
			<option value="">All states</option>
			{#each ['draft', 'processing', 'needs_research', 'review', 'accepted', 'declined'] as status (status)}<option
					value={status}>{status.replaceAll('_', ' ')}</option
				>{/each}
		</select></label
	>
	<button type="submit" class="rounded-md border border-byl-border-control px-3 py-2">Filter</button
	>
</form>
<ul class="mt-4 divide-y divide-byl-border border-y border-byl-border">
	{#each data.briefs as brief (brief.id)}
		<li class="py-4">
			<a href={`/admin/briefs/${brief.id}`} class="font-medium break-words underline"
				>{brief.submitted.topic}</a
			>
			<p class="mt-1 text-sm">{brief.submitted.area} · {brief.status.replaceAll('_', ' ')}</p>
			{#if brief.reason}<p class="mt-1 text-sm break-words whitespace-pre-wrap text-byl-muted">
					{brief.reason}
				</p>{/if}
			<p class="mt-1 text-xs text-byl-muted">Updated {when(brief.updated_at)}</p>
		</li>
	{:else}
		<li class="py-6 text-sm text-byl-muted">
			{data.status
				? 'No briefs in this state.'
				: 'No route briefs yet. Save a topic and area to begin.'}
		</li>
	{/each}
</ul>
<nav aria-label="Brief pages" class="mt-4 flex gap-5 text-sm">
	{#if data.offset > 0}<a
			class="underline"
			href={`?status=${data.status}&offset=${Math.max(0, data.offset - 50)}`}>Previous page</a
		>{/if}
	{#if data.briefs.length === 50}<a
			class="underline"
			href={`?status=${data.status}&offset=${data.offset + 50}`}>Next page</a
		>{/if}
</nav>
