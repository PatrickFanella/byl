<script lang="ts">
	import StatusPill from '#lib/components/StatusPill.svelte';
	import { when } from '#lib/format.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	const pendingTotal = $derived(
		Object.values(data.stats.pending_candidates ?? {}).reduce((a, b) => a + (b ?? 0), 0)
	);
</script>

<h1 class="text-2xl font-semibold">Dashboard</h1>

<div class="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
	<a
		href="/admin/queue"
		class="rounded-xl border border-stone-200 bg-white p-4 hover:border-stone-400"
	>
		<div class="text-xs tracking-wide text-stone-500 uppercase">Pending review</div>
		<div class="mt-1 text-3xl font-semibold">{pendingTotal}</div>
		<div class="mt-1 text-xs text-stone-500">
			{#each Object.entries(data.stats.pending_candidates ?? {}) as [k, v] (k)}<span class="mr-2"
					>{v} {k}</span
				>{/each}
		</div>
	</a>
	<div class="rounded-xl border border-stone-200 bg-white p-4">
		<div class="text-xs tracking-wide text-stone-500 uppercase">Published</div>
		<div class="mt-1 text-sm">
			<div><strong>{data.stats.published?.places ?? 0}</strong> places</div>
			<div><strong>{data.stats.published?.moments ?? 0}</strong> moments</div>
			<div><strong>{data.stats.published?.tours ?? 0}</strong> tours</div>
		</div>
	</div>
	<a
		href="/admin/jobs"
		class="rounded-xl border border-stone-200 bg-white p-4 hover:border-stone-400"
	>
		<div class="text-xs tracking-wide text-stone-500 uppercase">Jobs</div>
		<div class="mt-1 text-sm">
			{#each ['queued', 'running', 'failed', 'done'] as s (s)}
				<div class="flex justify-between">
					<span>{s}</span><strong>{data.stats.jobs?.[s] ?? 0}</strong>
				</div>
			{/each}
		</div>
	</a>
	<div class="rounded-xl border border-stone-200 bg-white p-4">
		<div class="text-xs tracking-wide text-stone-500 uppercase">People</div>
		<div class="mt-1 text-sm">
			<div><strong>{data.stats.accounts ?? 0}</strong> accounts</div>
			<div><strong>{data.stats.user_routes ?? 0}</strong> user routes</div>
		</div>
	</div>
</div>

<section class="mt-8 grid gap-6 lg:grid-cols-2">
	<div>
		<h2 class="font-semibold">Waiting for review</h2>
		<ul class="mt-2 divide-y divide-stone-200 rounded-xl border border-stone-200 bg-white">
			{#each data.pending as c (c.id)}
				<li>
					<a
						href="/admin/queue/{c.id}"
						class="flex items-center gap-3 px-4 py-2 text-sm hover:bg-stone-50"
					>
						<StatusPill status={c.kind} />
						<span class="flex-1 truncate"
							>{(c.payload as { title?: string; name?: string; caption?: string })?.title ??
								(c.payload as { name?: string })?.name ??
								(c.payload as { caption?: string })?.caption ??
								c.id}</span
						>
						<span class="text-xs text-stone-500"
							>{c.source_name ?? c.origin} · {when(c.created_at)}</span
						>
					</a>
				</li>
			{:else}
				<li class="px-4 py-3 text-sm text-stone-500">
					Nothing pending. <a class="underline" href="/admin/imports">Run an import</a>.
				</li>
			{/each}
		</ul>
	</div>
	<div>
		<h2 class="font-semibold">Recent failed jobs</h2>
		<ul class="mt-2 divide-y divide-stone-200 rounded-xl border border-stone-200 bg-white">
			{#each data.failedJobs as j (j.id)}
				<li class="px-4 py-2 text-sm">
					<div class="flex items-center gap-2">
						<StatusPill status={j.status} /><span class="font-mono text-xs">{j.kind}</span><span
							class="ml-auto text-xs text-stone-500">{when(j.created_at)}</span
						>
					</div>
					<div class="mt-1 truncate text-xs text-red-700">{j.last_error}</div>
				</li>
			{:else}
				<li class="px-4 py-3 text-sm text-stone-500">No failed jobs.</li>
			{/each}
		</ul>
	</div>
</section>
