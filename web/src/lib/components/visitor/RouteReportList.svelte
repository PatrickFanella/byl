<script lang="ts">
	import { Loader } from '#lib/loader.svelte.ts';
	import { myRouteReports, getTourRevision } from '#lib/visitor.ts';
	import { reportReasons, reportStatus } from '#lib/routes/reports.ts';
	import LoadState from './LoadState.svelte';
	let offset = $state(0);
	const reports = new Loader(
		async () => {
			const rows = (await myRouteReports(offset)).items;
			const titles: Record<string, string> = {};
			const refs = new Map(
				rows.slice(0, 20).map((r) => [r.submitted.revision_id, r.submitted.tour_id])
			);
			await Promise.all(
				[...refs].map(async ([revision, tour]) => {
					try {
						titles[revision] =
							(await getTourRevision(tour, revision)).snapshot?.tour.title ?? 'Reported route';
					} catch {
						titles[revision] = 'Reported route';
					}
				})
			);
			return { rows, titles };
		},
		(page) => page.rows.length === 0
	);
	$effect(() => {
		void offset;
		reports.run();
	});
</script>

<div class="mt-6 max-w-3xl">
	<button
		class="mb-4 rounded-md border border-byl-border-control px-3 py-2 text-sm"
		onclick={() => reports.run()}>Refresh reports</button
	>
	<LoadState
		status={reports.status}
		message={reports.message}
		emptyTitle="No reports on this page"
		emptyMessage="Report a problem from a tour or active trip. Your private reports and curator decisions will appear here."
		onretry={() => reports.run()}
	>
		{#if reports.data}<ul class="divide-y divide-byl-border border-y border-byl-border">
				{#each reports.data.rows.slice(0, 20) as report (report.id)}<li class="py-4">
						<a class="font-semibold text-byl-action underline" href={`/routes/reports/${report.id}`}
							>{reports.data.titles[report.submitted.revision_id]}</a
						>
						<p class="mt-1 text-sm text-byl-muted">
							{reportReasons.find((r) => r.value === report.submitted.reason)?.label ??
								report.submitted.reason} · {report.submitted.mode === 'bike'
								? 'Cycling'
								: 'Walking'} · {reportStatus[report.status]}
						</p>
						{#if report.reason}<p class="mt-2 line-clamp-2 text-sm">{report.reason}</p>{/if}
					</li>{/each}
			</ul>{/if}
	</LoadState>
	{#if offset > 0 || (reports.data?.rows.length ?? 0) > 20}<nav
			class="mt-4 flex items-center gap-4 text-sm"
			aria-label="Report pages"
		>
			<button
				class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
				disabled={offset === 0 || reports.status === 'loading'}
				onclick={() => (offset = Math.max(0, offset - 20))}>Previous reports</button
			><span>Page {offset / 20 + 1}</span><button
				class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
				disabled={(reports.data?.rows.length ?? 0) <= 20 || reports.status === 'loading'}
				onclick={() => (offset += 20)}>Next reports</button
			>
		</nav>{/if}
</div>
