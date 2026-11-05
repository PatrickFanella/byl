<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import { retryJob } from '#lib/admin.ts';
	import { when } from '#lib/format.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let busy = $state('');
	async function retry(id: string) {
		busy = id;
		try {
			await retryJob(id);
			await invalidateAll();
		} finally {
			busy = '';
		}
	}
</script>

<div class="flex items-center gap-4">
	<h1 class="text-2xl font-semibold">Jobs</h1>
	<nav class="flex gap-1 text-sm">
		{#each ['', 'queued', 'running', 'failed', 'done'] as s (s)}
			<a
				href="/admin/jobs{s ? `?status=${s}` : ''}"
				class="rounded-md px-2 py-1 {data.status === s
					? 'bg-byl-action text-byl-action-text'
					: 'hover:bg-byl-surface'}">{s || 'all'}</a
			>
		{/each}
	</nav>
</div>
<table class="mt-4 w-full rounded-lg border border-byl-border bg-byl-background text-sm">
	<thead class="text-left text-xs tracking-wide text-byl-muted"
		><tr
			><th class="px-3 py-2">Kind</th><th class="px-3 py-2">Status</th><th class="px-3 py-2"
				>Attempts</th
			><th class="px-3 py-2">Note</th><th class="px-3 py-2">When</th><th class="px-3 py-2"></th></tr
		></thead
	>
	<tbody class="divide-y divide-byl-border">
		{#each data.jobs as j (j.id)}
			<tr>
				<td class="px-3 py-2 font-mono text-xs">{j.kind}</td>
				<td class="px-3 py-2"><StatusPill status={j.status} /></td>
				<td class="px-3 py-2">{j.attempts}/{j.max_attempts}</td>
				<td
					class="max-w-md truncate px-3 py-2 text-xs {j.status === 'failed'
						? 'text-byl-danger'
						: 'text-byl-muted'}"
					title={j.last_error}>{j.last_error}</td
				>
				<td class="px-3 py-2 text-xs text-byl-muted">{when(j.created_at)}</td>
				<td class="px-3 py-2 text-right"
					>{#if j.status === 'failed'}<button
							class="text-xs underline"
							disabled={busy === j.id}
							onclick={() => retry(j.id)}>Retry</button
						>{/if}</td
				>
			</tr>
		{:else}
			<tr><td colspan="6" class="px-3 py-4 text-byl-muted">No jobs.</td></tr>
		{/each}
	</tbody>
</table>
