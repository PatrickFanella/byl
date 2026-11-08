<script lang="ts">
	import { page } from '$app/state';
	import { ApiError } from '#lib/api/index.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import {
		getRouteReport,
		getRouteCorrectionOutcome,
		getTourRevision,
		type TourRevision,
		type RouteCorrectionOutcome
	} from '#lib/visitor.ts';
	import {
		reportReasons,
		reportStatus,
		reportTargets,
		reportTargetKey
	} from '#lib/routes/reports.ts';
	import LoadState from './LoadState.svelte';
	import Map from '#lib/components/Map.svelte';
	let { id }: { id: string } = $props();
	const report = new Loader(async () => {
		let saved;
		try {
			saved = await getRouteReport(id);
		} catch (error) {
			if (error instanceof ApiError && error.status === 404)
				throw new ApiError(404, 'not_found', 'This report is not available for this account.');
			throw error;
		}
		let revision: TourRevision | null = null;
		let outcome: RouteCorrectionOutcome | null = null;
		let updateError = '';
		try {
			revision = await getTourRevision(saved.submitted.tour_id, saved.submitted.revision_id);
		} catch {
			/* Private submission stays readable after a revision is withdrawn. */
		}
		try {
			outcome = await getRouteCorrectionOutcome(id);
		} catch (error) {
			if (!(error instanceof ApiError && error.status === 404))
				updateError =
					'The report is available, but its approved-update status could not be read. Refresh to check again.';
		}
		return { saved, revision, outcome, updateError };
	});
	$effect(() => {
		void id;
		report.run();
	});
	const submitted = $derived(report.data?.saved.submitted);
	const label = $derived(
		report.data?.revision && submitted
			? reportTargets(report.data.revision, submitted.mode).find(
					(t) => t.key === reportTargetKey(submitted.affected)
				)?.label
			: null
	);
	const markers = $derived([
		...(report.data?.revision?.snapshot?.tour.stops.map((s) => ({
			lat: s.place.arrival_point?.point?.latitude ?? s.place.latitude,
			lng: s.place.arrival_point?.point?.longitude ?? s.place.longitude,
			label: s.place.name,
			position: s.position
		})) ?? []),
		...(submitted?.annotation
			? [
					{
						lat: submitted.annotation.lat,
						lng: submitted.annotation.lng,
						label: 'Your reported problem location'
					}
				]
			: [])
	]);
	const lines = $derived(
		submitted && report.data?.revision?.snapshot
			? [
					{
						coords: (report.data.revision.snapshot.routes[submitted.mode]?.geometry?.coordinates ??
							[]) as [number, number][],
						state: 'future' as const
					},
					...(submitted.alternative
						? [
								{
									coords: submitted.alternative.coordinates as [number, number][],
									state: 'active' as const
								}
							]
						: [])
				]
			: []
	);
</script>

<div class="mt-6 max-w-3xl">
	<LoadState status={report.status} message={report.message} onretry={() => report.run()}>
		{#if report.data && submitted}
			{@const saved = report.data.saved}
			{#if page.url.searchParams.get('submitted') === '1'}<p
					role="status"
					class="mb-5 border border-byl-border bg-byl-surface p-4"
				>
					Report saved privately. A curator will review it before any route changes.
				</p>{/if}
			<h2 class="text-2xl font-semibold">
				{report.data.revision?.snapshot?.tour.title ?? 'Reported route'}
			</h2>
			<p class="mt-2 text-byl-muted">
				{label ?? `Reported ${submitted.affected.kind}`} · {submitted.mode === 'bike'
					? 'Cycling'
					: 'Walking'}
			</p>
			<section
				aria-label="Report decision"
				class="mt-5 space-y-3 border border-byl-border bg-byl-surface p-5"
			>
				<h3 class="text-lg font-semibold">Status: {reportStatus[saved.status]}</h3>
				{#if saved.reason}<h4 class="font-semibold">Curator update</h4>
					<p class="break-words whitespace-pre-wrap">{saved.reason}</p>{:else}<p
						class="text-sm text-byl-muted"
					>
						No curator decision has been recorded yet.
					</p>{/if}
				{#if report.data.outcome}
					<p>
						An approved route update is available. Your started trip stays on its original revision.
					</p>
					<a
						class="inline-block font-semibold text-byl-action underline"
						href={`/tours/${submitted.tour_id}/walk?mode=${submitted.mode}`}
						>Open trip to review the update</a
					>
					<p class="text-sm text-byl-muted">
						Choose “Check for route updates” in the trip, review the newer itinerary, then choose
						whether to use it.
					</p>
					{#if report.data.outcome.temporary}<p class="text-sm">
							Temporary detour · review due {report.data.outcome.review_on}.{report.data.outcome
								.reassessment_required
								? ' Reassessment is required; this does not mean the original route has reopened.'
								: ''}
						</p>{/if}
				{/if}
				{#if report.data.updateError}<p role="status" class="text-sm text-byl-warning">
						{report.data.updateError}
					</p>{/if}
				<button
					class="rounded-md border border-byl-border-control px-3 py-2 text-sm"
					onclick={() => report.run()}>Refresh report status</button
				>
			</section>
			<section aria-label="Your original observation" class="mt-7 space-y-3">
				<h3 class="text-xl font-semibold">Your original observation</h3>
				<p>{reportReasons.find((r) => r.value === submitted.reason)?.label ?? submitted.reason}</p>
				{#if submitted.details}<p class="break-words whitespace-pre-wrap">
						{submitted.details}
					</p>{/if}
				{#if submitted.better}<p class="break-words whitespace-pre-wrap">
						Suggested improvement: {submitted.better}
					</p>{/if}
				{#if submitted.annotation}<p class="text-sm">
						Reported location: {submitted.annotation.lat.toFixed(6)}, {submitted.annotation.lng.toFixed(
							6
						)}.
					</p>{/if}
				<p class="text-sm text-byl-muted">
					Direction: {submitted.direction ?? 'unknown'} · Duration: {submitted.duration ??
						'unknown'} · Observation date: {submitted.observed_on ?? 'not provided'}
				</p>
				{#if submitted.source_url}<p class="text-sm break-words">
						Source lead: {#if /^https?:\/\//i.test(submitted.source_url)}<a
								class="underline"
								target="_blank"
								rel="noreferrer"
								href={submitted.source_url}>{submitted.source_url}</a
							>{:else}{submitted.source_url}{/if}
					</p>{/if}
				{#if report.data.revision?.snapshot}<Map {markers} legs={lines} />
					<p class="text-xs text-byl-muted">
						Dashed: the reported revision. Solid: your unverified alternative, when provided.
					</p>{:else}<p class="text-sm text-byl-muted">
						The reported revision is no longer available to this account. Your private observation
						and decision remain here.
					</p>{/if}
			</section>
		{/if}
	</LoadState>
</div>
