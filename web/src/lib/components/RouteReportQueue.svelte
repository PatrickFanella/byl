<script lang="ts">
	import { Loader } from '#lib/loader.svelte.ts';
	import { listRouteReports } from '#lib/admin.ts';
	import { reportReasons, reportStatus } from '#lib/routes/reports.ts';
	import LoadState from './visitor/LoadState.svelte';
	let status = $state('');
	let offset = $state(0);
	const reports = new Loader(() => listRouteReports(status, offset));
	$effect(() => {
		void status;
		void offset;
		reports.run();
	});
</script>

<h1 class="text-2xl font-semibold">Route reports</h1>
<p class="mt-3 max-w-3xl text-sm text-byl-muted">
	Unresolved closures, blocked routes and unsafe areas appear first. Observations remain private
	leads until reviewed; report counts do not establish independent witnesses.
</p>
<div class="mt-5 flex flex-wrap items-end gap-4">
	<label
		>Status<select
			class="mt-1 block rounded-md border-byl-border-control"
			bind:value={status}
			onchange={() => {
				offset = 0;
			}}
			><option value="">All</option
			>{#each Object.entries(reportStatus) as [value, label] (value)}<option {value}>{label}</option
				>{/each}</select
		></label
	>
	<button
		class="rounded-md border border-byl-border-control px-3 py-2"
		onclick={() => reports.run()}>Refresh reports</button
	>
	<a class="text-sm underline" href="/admin/queue">Content review queue</a>
</div>
<div class="mt-5">
	<LoadState status={reports.status} message={reports.message} onretry={() => reports.run()}>
		<ul class="divide-y divide-byl-border border border-byl-border">
			{#each reports.data?.items ?? [] as report (report.id)}<li>
					<a
						class="block space-y-2 p-4 hover:bg-byl-surface"
						href={`/admin/queue/reports/${report.id}`}
						><p>
							<strong
								>{reportReasons.find((r) => r.value === report.submitted.reason)?.label}</strong
							>
							· {reportStatus[report.status]} · {report.submitted.mode === 'walk'
								? 'Walking'
								: 'Cycling'}
						</p>
						<p class="break-words whitespace-pre-wrap">
							{report.submitted.details || 'No additional description.'}
						</p>
						<p class="text-xs text-byl-muted">
							{new Date(report.created_at).toLocaleString()} · {report.submitted.affected.kind}
						</p></a
					>
				</li>{:else}<li class="p-4 text-sm text-byl-muted">
					No reports match this selection.
				</li>{/each}
		</ul>
		<div class="mt-4 flex flex-wrap gap-4">
			<button
				class="underline disabled:opacity-50"
				disabled={offset === 0 || reports.status === 'loading'}
				onclick={() => {
					offset = Math.max(0, offset - 50);
				}}>Previous reports</button
			><button
				class="underline disabled:opacity-50"
				disabled={(reports.data?.items.length ?? 0) < 50 || reports.status === 'loading'}
				onclick={() => {
					offset += 50;
				}}>Next reports</button
			>
		</div>
	</LoadState>
</div>
