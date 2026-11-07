<script lang="ts">
	import {
		getPathQuality,
		errorMessage,
		type PathQualityReport,
		type PathQualitySummary
	} from '#lib/admin.ts';
	let {
		tourId,
		mode,
		summary
	}: { tourId: string; mode: 'walk' | 'bike'; summary?: PathQualitySummary } = $props();
	let report = $state<PathQualityReport | null>(null);
	let loadedId = $state('');
	let busy = $state(false);
	let error = $state('');
	const labels: Record<string, string> = {
		inspected_source: 'Inspected source',
		provider_assertion: 'Routing provider',
		curator_assertion: 'Curator decision',
		unknown: 'Unresolved evidence'
	};
	const status = $derived(
		!summary
			? 'Awaiting assessment'
			: !summary.input_current
				? 'Assessment out of date'
				: summary.status === 'pass'
					? 'Ready for curator review'
					: summary.status === 'blocked'
						? 'Publication blocked'
						: 'Research needed'
	);
	async function inspect() {
		const id = summary?.report_id;
		if (!id) return;
		busy = true;
		error = '';
		try {
			const result = await getPathQuality(tourId, mode, id);
			if (summary?.report_id === id) {
				report = result.report;
				loadedId = id;
			}
		} catch (e) {
			error = errorMessage(e, 'Could not load the retained assessment.');
		} finally {
			busy = false;
		}
	}
</script>

<div class="mt-3 border-t border-byl-border pt-3">
	<h3 class="font-semibold">{mode === 'walk' ? 'Walking' : 'Cycling'} path quality</h3>
	<p class="mt-1">{status}</p>
	{#if summary && !summary.input_current}<p class="mt-1 text-byl-warning">
			Recompute this route after reviewing the changed inputs.
		</p>{/if}
	{#if summary?.issues.length}
		<ul class="mt-2 space-y-2">
			{#each summary.issues as issue, i (issue.code + issue.segment_id + i)}
				<li class="text-xs break-words">
					<strong
						>{labels[issue.origin] ?? issue.origin}{issue.segment_id
							? ` · ${issue.segment_id}`
							: ''}:</strong
					>
					{issue.detail}
				</li>
			{/each}
		</ul>
	{/if}
	{#if summary?.report_id}
		<button
			type="button"
			class="mt-2 underline disabled:opacity-50"
			disabled={busy}
			onclick={inspect}>{busy ? 'Loading assessment…' : 'Inspect retained evidence'}</button
		>
	{/if}
	{#if error}<p role="alert" class="mt-2 text-byl-danger">{error}</p>{/if}
	{#if report && loadedId === summary?.report_id}
		<div class="mt-3 space-y-3 text-xs">
			<p>
				Checked {new Date(report.checked_at).toLocaleString()}. A passing assessment still requires
				curator approval and does not certify current conditions.
			</p>
			<p>
				Provider: {report.provider || 'Unavailable'}. {report.attribution?.failure
					? 'Exact network attribution unavailable.'
					: `${report.attribution?.legs?.length ?? 0} attributed story legs.`}
			</p>
			{#each report.inspections as inspection, i (inspection.segment_id + inspection.purpose + inspection.mode + i)}
				<blockquote class="border-l-2 border-byl-border pl-2">
					<p class="font-medium">
						{inspection.segment_id} · {inspection.purpose === 'network'
							? 'Network identity'
							: `${inspection.mode} access`}
					</p>
					<p class="mt-1 break-words">“{inspection.quote}”</p>
					<a
						class="mt-1 block break-all underline"
						href={inspection.url}
						target="_blank"
						rel="noreferrer">Inspected source · {inspection.reference}</a
					>
					<p class="mt-1">Retrieved {new Date(inspection.retrieved_at).toLocaleString()}</p>
				</blockquote>
			{:else}<p>No independently inspected source passages were retained.</p>{/each}
		</div>
	{/if}
</div>
