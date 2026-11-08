<script lang="ts">
	import { onMount, onDestroy, untrack } from 'svelte';
	import { session } from '#lib/auth.ts';
	import { ApiError, type Schemas } from '#lib/api/index.ts';
	import {
		getRouteReport,
		relatedRouteReports,
		listRouteInvestigations,
		queueRouteInvestigation,
		getRouteCorrectionComparison,
		approveRouteCorrection,
		setRouteReportState,
		getTour,
		errorMessage,
		type RouteReport,
		type RouteInvestigation,
		type RouteCorrectionComparison,
		type RouteCorrection,
		type TourDetail,
		type BriefStatus
	} from '#lib/admin.ts';
	import { reportReasons, reportStatus } from '#lib/routes/reports.ts';
	import MapView from './Map.svelte';
	import RouteNavigationReview from './RouteNavigationReview.svelte';
	let { id }: { id: string } = $props();
	const actor = untrack(() => session.account()?.id);
	let active = true;
	onDestroy(() => {
		active = false;
	});
	const current = () => active && session.account()?.id === actor;
	let report = $state<RouteReport | null>(null);
	let related = $state<RouteReport[]>([]);
	let investigations = $state<RouteInvestigation[]>([]);
	let selected = $state<string[]>([]);
	let investigationID = $state('');
	let comparison = $state<RouteCorrectionComparison | null>(null);
	let tour = $state<TourDetail | null>(null);
	let released = $state<RouteCorrection | null>(null);
	let busy = $state('');
	let error = $state('');
	let message = $state('');
	let note = $state('');
	let confirmed = $state(false);
	let temporary = $state(false);
	let reviewOn = $state('');
	let pendingNavigation = $state(false);
	type Pending = { key: string; body: Schemas['RouteInvestigationRequest'] };
	let pending = $state<Pending | null>(null);
	let retryError = $state('');
	const storageKey = untrack(() => `byl:report-investigation:${actor}:${id}`);
	const investigation = $derived(investigations.find((i) => i.id === investigationID));
	const result = $derived(investigation?.result);
	const mode = $derived(report?.submitted.mode ?? 'walk');
	const canRelease = $derived(
		!!comparison?.ready_to_publish && !pendingNavigation && !pending && !released
	);
	function invalidate() {
		comparison = null;
		confirmed = false;
	}
	function fail(e: unknown, fallback: string) {
		if (e instanceof ApiError && e.status === 409) {
			invalidate();
			error =
				'This review changed. Reload the report and comparison, then review again. Your decision note is kept.';
		} else error = errorMessage(e, fallback);
		if (e instanceof ApiError && Object.keys(e.fields).length)
			error +=
				' ' +
				Object.entries(e.fields)
					.map(([k, v]) => `${k}: ${v}`)
					.join('; ');
	}
	async function refresh() {
		invalidate();
		const saved = await getRouteReport(id);
		const [observations, history, working] = await Promise.all([
			relatedRouteReports(id),
			listRouteInvestigations(id),
			getTour(saved.submitted.tour_id)
		]);
		if (!current()) return;
		report = saved;
		related = observations.items;
		investigations = history.items;
		tour = working;
		if (!investigations.some((i) => i.id === investigationID))
			investigationID = investigations[0]?.id ?? '';
		selected = selected.filter((item) => related.some((r) => r.id === item));
	}
	async function reload() {
		if (busy) return;
		busy = 'reload';
		error = '';
		try {
			await refresh();
		} catch (e) {
			if (current()) fail(e, 'Could not load the private report.');
		} finally {
			if (current()) busy = '';
		}
	}
	onMount(() => {
		try {
			const raw = sessionStorage.getItem(storageKey);
			if (raw) {
				const parsed = JSON.parse(raw) as Pending;
				if (
					typeof parsed.key !== 'string' ||
					!parsed.key ||
					!Number.isInteger(parsed.body?.version) ||
					!Array.isArray(parsed.body.related_report_ids) ||
					parsed.body.related_report_ids.length > 19 ||
					parsed.body.related_report_ids.some((value) => typeof value !== 'string')
				)
					throw new Error('Invalid saved retry');
				pending = parsed;
			}
		} catch {
			retryError =
				'The saved investigation retry could not be read. Discard the local retry before starting a new request; discarding does not cancel a job.';
		}
		void reload();
	});
	async function investigate() {
		if (busy || !report || selected.length > 19 || retryError) return;
		busy = 'investigate';
		error = '';
		message = '';
		invalidate();
		try {
			const request = pending ?? {
				key: crypto.randomUUID(),
				body: { version: report.version, related_report_ids: [...selected] }
			};
			// Save exact body/key before sending. An uncertain response can only replay this request.
			pending = request;
			try {
				sessionStorage.setItem(storageKey, JSON.stringify(request));
			} catch {
				error =
					'Could not save the investigation retry in this browser. No investigation was sent. Retry when storage is available.';
				return;
			}
			const saved = await queueRouteInvestigation(id, request.body, request.key);
			if (!current()) return;
			investigationID = saved.id;
			try {
				sessionStorage.removeItem(storageKey);
				pending = null;
			} catch {
				message =
					'Investigation saved. Browser storage could not clear its retry; an exact retry returns the same investigation.';
			}
			await refresh();
			message ||=
				'Investigation queued privately. Refresh after the job completes; proposals do not approve access or publish changes.';
		} catch (e) {
			if (current())
				fail(e, 'Could not confirm the investigation. Retry keeps the same body and key.');
		} finally {
			if (current()) busy = '';
		}
	}
	function discardRetry() {
		try {
			sessionStorage.removeItem(storageKey);
			pending = null;
			retryError = '';
			error = '';
		} catch {
			error = 'Could not clear the browser retry. Nothing new was sent.';
		}
	}
	async function inspect() {
		if (busy || !investigationID) return;
		const selectedID = investigationID;
		busy = 'compare';
		error = '';
		invalidate();
		try {
			const fresh = await getRouteCorrectionComparison(id, selectedID);
			if (!current() || selectedID !== investigationID) return;
			comparison = fresh;
			report = fresh.report;
			investigations = investigations.map((i) =>
				i.id === fresh.investigation.id ? fresh.investigation : i
			);
		} catch (e) {
			if (current()) fail(e, 'Could not load the current comparison.');
		} finally {
			if (current()) busy = '';
		}
	}
	async function decide(status: BriefStatus) {
		if (busy || !report || !note.trim() || pending) return;
		busy = 'decision';
		error = '';
		message = '';
		invalidate();
		try {
			await setRouteReportState(id, report.version, status, note);
			if (!current()) return;
			await refresh();
			message = 'Decision and reason saved privately. No route was published.';
		} catch (e) {
			if (current()) fail(e, 'Could not save this decision. Your note is kept.');
		} finally {
			if (current()) busy = '';
		}
	}
	async function release() {
		if (
			busy ||
			!comparison ||
			!canRelease ||
			!confirmed ||
			!note.trim() ||
			(temporary && !reviewOn)
		)
			return;
		const reviewed = comparison;
		busy = 'release';
		error = '';
		message = '';
		try {
			const saved = await approveRouteCorrection(id, {
				version: reviewed.version,
				investigation_id: reviewed.investigation.id,
				note,
				temporary,
				review_on: temporary ? reviewOn : null
			});
			if (!current()) return;
			released = saved;
			invalidate();
			await refresh();
			message =
				'Correction published as a new revision. The reporter can review it; started trips stay on their chosen revision.';
		} catch (e) {
			if (current())
				fail(
					e,
					'Could not confirm publication. Reload the report before retrying; your note is kept.'
				);
		} finally {
			if (current()) busy = '';
		}
	}
	function markers(snapshot?: Schemas['TourSnapshot'] | null) {
		return (
			snapshot?.tour.stops.map((s) => ({
				lat: s.place.arrival_point?.point?.latitude ?? s.place.latitude,
				lng: s.place.arrival_point?.point?.longitude ?? s.place.longitude,
				label: s.place.name,
				position: s.position
			})) ?? []
		);
	}
	function coords(route?: Schemas['TourRoute'] | null) {
		return (route?.geometry?.coordinates ?? []) as [number, number][];
	}
	const signed = (value: number | null | undefined, unit: string) =>
		value == null ? 'Unavailable' : `${value > 0 ? '+' : ''}${Math.round(value)} ${unit}`;
	const delta = (after: number | undefined, before: number | undefined) =>
		after === undefined || before === undefined ? null : after - before;
	const safeURL = (value: string) => (/^https?:\/\//i.test(value) ? value : undefined);
</script>

<div class="max-w-6xl space-y-6">
	<a class="text-sm underline" href="/admin/queue/reports">← Route reports</a>
	<h1 class="text-2xl font-semibold">Review a route observation</h1>
	<p class="text-sm text-byl-muted">
		Observations and investigation paths are unverified leads. A correction needs reviewed working
		navigation, current path quality and an explicit release.
	</p>
	{#if error}<p role="alert" class="border border-byl-danger p-3 break-words text-byl-danger">
			{error}
		</p>{/if}
	{#if retryError}<p role="alert" class="border border-byl-danger p-3 text-byl-danger">
			{retryError}
		</p>{/if}
	{#if message}<p role="status" class="border border-byl-border p-3">{message}</p>{/if}
	<button
		class="rounded-md border border-byl-border-control px-3 py-2"
		disabled={!!busy}
		onclick={reload}>Reload report and investigations</button
	>
	{#if report}
		<section aria-label="Original report" class="space-y-3 border border-byl-border p-4">
			<h2 class="text-xl font-semibold">{tour?.title ?? 'Reported tour'}</h2>
			<p>
				<strong>{reportReasons.find((r) => r.value === report!.submitted.reason)?.label}</strong> · {reportStatus[
					report.status
				]} · {mode === 'walk' ? 'Walking' : 'Cycling'}
			</p>
			<p class="break-words whitespace-pre-wrap">
				{report.submitted.details || 'No additional description.'}
			</p>
			{#if report.submitted.better}<p class="break-words">
					Suggested improvement: {report.submitted.better}
				</p>{/if}
			<p class="text-sm">
				Direction: {report.submitted.direction} · Duration: {report.submitted.duration} · Observed: {report
					.submitted.observed_on ?? 'date unknown'}
			</p>
			<p class="text-sm break-words">
				Affected {report.submitted.affected.kind}: {report.submitted.affected.stop_id ||
					`${report.submitted.affected.from_stop_id} → ${report.submitted.affected.to_stop_id}`}.
				Retained revision: {report.submitted.revision_id}.
			</p>
			{#if report.submitted.source_url}<p class="break-words">
					<a
						class="underline"
						href={safeURL(report.submitted.source_url)}
						target="_blank"
						rel="noreferrer">Submitted source lead</a
					>
				</p>{/if}
			{#if report.reason}<p class="break-words whitespace-pre-wrap">
					Last curator reason: {report.reason}
				</p>{/if}
		</section>
		<section aria-label="Investigation observations" class="space-y-3 border border-byl-border p-4">
			<h2 class="text-xl font-semibold">Investigate the reported area</h2>
			<p class="text-sm text-byl-muted">
				Select up to 19 related reports. Shared accounts and duplicate submissions do not establish
				independent corroboration. Each selected version is retained.
			</p>
			<fieldset disabled={!!busy || !!pending} class="min-w-0 space-y-3">
				<legend class="sr-only">Related observations</legend>
				{#each related as item (item.id)}<label class="flex items-start gap-3"
						><input
							type="checkbox"
							value={item.id}
							bind:group={selected}
							disabled={selected.length >= 19 && !selected.includes(item.id)}
						/><span class="min-w-0 break-words"
							>{item.submitted.reason} · {item.submitted.details || 'No description'}
							<span class="block text-xs text-byl-muted"
								>{reportStatus[item.status]} · {new Date(item.created_at).toLocaleString()}</span
							></span
						></label
					>{:else}<p class="text-sm">No other reports for this area.</p>{/each}
			</fieldset>
			<div class="flex flex-wrap gap-3">
				<button
					class="rounded-md border border-byl-border-control px-3 py-2"
					disabled={!!busy || selected.length > 19 || !!retryError}
					onclick={investigate}
					>{pending ? 'Retry exact investigation' : 'Queue retained investigation'}</button
				>{#if pending || retryError}<button
						class="text-sm underline"
						disabled={!!busy}
						onclick={discardRetry}>Discard local retry</button
					>{/if}
			</div>
			{#if pending}<p class="text-sm text-byl-warning">
					A request may have reached the server. Retry sends the saved selection and key; discard
					does not cancel a queued job.
				</p>{/if}
			<label class="block"
				>Investigation<select
					class="mt-1 w-full rounded-md border-byl-border-control"
					bind:value={investigationID}
					onchange={invalidate}
					disabled={!!busy}
					><option value="">Select an investigation</option
					>{#each investigations as item (item.id)}<option value={item.id}
							>{new Date(item.created_at).toLocaleString()} · {item.result
								? 'Result retained'
								: 'Awaiting result'}</option
						>{/each}</select
				></label
			>
			{#if investigation}<p class="text-sm">
					<a class="underline" href={`/admin/briefs/${investigation.input.brief_id}`}
						>Open evidence and story package workspace</a
					>
					· <a class="underline" href="/admin/jobs">Inspect job progress</a>
				</p>{/if}
		</section>
		{#if result}
			<section
				aria-label="Retained investigation result"
				class="space-y-4 border border-byl-border p-4"
			>
				<h2 class="text-xl font-semibold">Investigation proposal</h2>
				<p>
					Compared in the published itinerary direction. Distance change: {signed(
						result.distance_change_m,
						'm'
					)}; duration change: {signed(result.duration_change_s, 's')}.
				</p>
				<MapView
					legs={[
						{ coords: coords(result.before), state: 'future' as const },
						{ coords: coords(result.after), state: 'active' as const }
					].filter((l) => l.coords.length > 1)}
				/>
				<p class="text-xs text-byl-muted">
					Dashed: reported route. Solid: unapproved computed proposal, when available. This proposal
					is separate from the prepared release below.
				</p>
				<p class="text-sm">
					Retained stories stay unchanged by investigation. No independent corroboration is
					inferred.
				</p>
				{#each result.issues as issue, index (index)}<p
						class="text-sm break-words text-byl-warning"
					>
						{issue.origin} · {issue.segment_id || 'Route'}: {issue.detail}
					</p>{/each}
				<h3 class="font-semibold">Frozen observations</h3>
				{#each investigation!.input.observations as observation (observation.report.id)}<div
						class="border-l-2 border-byl-border pl-3 text-sm"
					>
						<p class="break-words whitespace-pre-wrap">
							{observation.report.submitted.details || observation.report.submitted.reason}
						</p>
						<p>
							Direction: {observation.report.submitted.direction}. {observation.duplicate_of
								? 'Grouped duplicate; do not count as an independent witness.'
								: 'Unverified observation.'}
						</p>
					</div>{/each}
				<h3 class="font-semibold">Retained source evidence</h3>
				{#each result.evidence as evidence (evidence.id)}<details
						class="min-w-0 border-t border-byl-border pt-2"
					>
						<summary class="cursor-pointer break-words"
							>{evidence.result?.title || evidence.reference || 'Source lead'} · {evidence.result
								?.outcome ?? 'Awaiting inspection'}</summary
						>
						<p class="mt-2 text-xs">
							Retrieved: {evidence.result?.retrieved_at ?? 'not inspected'} · Media rights remain separate.
						</p>
						<a
							class="mt-2 block text-sm break-all underline"
							href={safeURL(evidence.result?.final_url || evidence.url)}
							target="_blank"
							rel="noreferrer">{evidence.url}</a
						>{#each evidence.result?.passages ?? [] as passage (passage.reference)}<blockquote
								class="mt-3 border-l-2 border-byl-border pl-3 text-sm"
							>
								<p>{passage.text}</p>
								<p class="text-xs text-byl-muted">{passage.reference}</p>
							</blockquote>{/each}
					</details>{:else}<p class="text-sm">
						No inspected source snapshots were retained. Resolve evidence gaps before releasing a
						correction.
					</p>{/each}
			</section>
		{/if}
		{#if tour}<RouteNavigationReview
				{tour}
				{report}
				onchanged={refresh}
				onpending={(value) => {
					pendingNavigation = value;
					if (value) invalidate();
				}}
			/>{/if}
		<section aria-label="Release comparison" class="space-y-4 border border-byl-border p-4">
			<h2 class="text-xl font-semibold">Compare the prepared release</h2>
			<button
				class="rounded-md border border-byl-border-control px-3 py-2"
				disabled={!!busy || !investigationID || pendingNavigation}
				onclick={inspect}>Reload current release comparison</button
			>
			{#if comparison}
				<div class="grid gap-5 lg:grid-cols-2">
					<div>
						<h3 class="mb-2 font-semibold">
							Reported published revision {comparison.before.number}
						</h3>
						<MapView
							markers={markers(comparison.before.snapshot)}
							line={coords(comparison.before.snapshot?.routes[mode])}
						/>
					</div>
					<div>
						<h3 class="mb-2 font-semibold">Prepared working route</h3>
						<MapView
							markers={markers(comparison.prepared)}
							line={coords(comparison.prepared?.routes[mode])}
						/>
					</div>
				</div>
				<p>
					Prepared distance change: {signed(
						delta(
							comparison.prepared?.routes[mode]?.distance_m,
							comparison.before.snapshot?.routes[mode]?.distance_m
						),
						'm'
					)}; duration change: {signed(
						delta(
							comparison.prepared?.routes[mode]?.duration_s,
							comparison.before.snapshot?.routes[mode]?.duration_s
						),
						's'
					)}.
				</p>
				<h3 class="font-semibold">Stops and story impacts</h3>
				<ul class="space-y-2 text-sm">
					{#each comparison.prepared?.tour.stops ?? [] as stop (stop.id)}{@const before =
							comparison.before.snapshot?.tour.stops.find((s) => s.id === stop.id)}
						<li class="break-words">
							{stop.position}. {stop.place.name} · {before
								? before.position === stop.position
									? 'Retained stop'
									: 'Order changed'
								: 'Added stop'}{result?.affected_stop_ids.includes(stop.id)
								? ' · Reported area'
								: ''}
							<details class="mt-1">
								<summary class="cursor-pointer underline">Review prepared visitor stories</summary
								>{#each stop.featured_moment_id ? [stop.featured_moment_id] : [] as momentID (momentID)}{@const moment =
										comparison.prepared?.moments[momentID]}
									<div class="mt-2 border-l-2 border-byl-border pl-3">
										<h4 class="font-semibold">{moment?.title ?? 'Story unavailable'}</h4>
										<p class="whitespace-pre-wrap">
											{moment?.body ?? 'Resolve missing story before publication.'}
										</p>
									</div>{:else}<p>No linked visitor story.</p>{/each}
							</details>
						</li>{/each}{#each comparison.before.snapshot?.tour.stops.filter((s) => !comparison!.prepared?.tour.stops.some((p) => p.id === s.id)) ?? [] as removed (removed.id)}<li
						>
							Removed: {removed.place.name}
						</li>{/each}
				</ul>
				{#if comparison.potentially_affected_tours.length}<h3 class="font-semibold">
						Other tours requiring manual assessment
					</h3>
					<p class="text-sm">
						Shared place/reference identity is a review lead, not a detected connection or an
						automatic change.
					</p>
					<ul class="space-y-1 text-sm">
						{#each comparison.potentially_affected_tours as other (other)}<li>
								<a class="underline" href={`/admin/tours/${other}`}>Review shared tour {other}</a>
							</li>{/each}
					</ul>{/if}
				<h3 class="font-semibold">Path quality and unresolved restrictions</h3>
				{#each Object.entries(comparison.path_reports) as [travelMode, quality] (travelMode)}{#if quality}<details
						>
							<summary class="cursor-pointer"
								>{travelMode === 'walk' ? 'Walking' : 'Cycling'} · {quality.status}</summary
							>
							<p class="mt-2 text-xs">
								Checked {new Date(quality.checked_at).toLocaleString()} · {quality.provider}
							</p>
							{#each quality.issues as issue, index (index)}<p class="mt-2 text-sm break-words">
									{issue.origin}: {issue.detail}
								</p>{/each}{#each quality.inspections as inspection, index (index)}<blockquote
									class="mt-2 border-l-2 border-byl-border pl-3 text-sm"
								>
									<p>{inspection.quote}</p>
									<a
										class="break-words underline"
										href={safeURL(inspection.url)}
										target="_blank"
										rel="noreferrer"
										>{inspection.segment_id} · {inspection.purpose} · {inspection.mode} · {inspection.reference}</a
									>
								</blockquote>{/each}
						</details>{/if}{/each}
				{#if comparison.blockers.length}<ul class="space-y-2 text-sm text-byl-warning">
						{#each comparison.blockers as blocker, index (index)}<li class="break-words">
								{blocker}
							</li>{/each}
					</ul>{:else}<p class="text-sm">
						No publication blockers in this comparison. Review sources and current conditions before
						deciding.
					</p>{/if}
			{:else}<p class="text-sm text-byl-muted">
					Load the current comparison after investigation and adjustments. Every change requires a
					fresh review.
				</p>{/if}
		</section>
		<section aria-label="Curator decision" class="space-y-4 border border-byl-border p-4">
			<h2 class="text-xl font-semibold">Record a decision</h2>
			<label class="block"
				>Decision reason (visible to the reporter)<textarea
					class="mt-1 w-full rounded-md border-byl-border-control"
					rows="3"
					maxlength="2000"
					bind:value={note}></textarea></label
			>
			<div class="flex flex-wrap gap-3">
				{#if ['draft', 'processing', 'review'].includes(report.status)}<button
						class="rounded-md border border-byl-border-control px-3 py-2"
						disabled={!!busy || !!pending || !note.trim()}
						onclick={() => decide('needs_research')}>Request information or research</button
					>{/if}{#if ['draft', 'processing', 'needs_research'].includes(report.status)}<button
						class="rounded-md border border-byl-border-control px-3 py-2"
						disabled={!!busy || !!pending || !note.trim()}
						onclick={() => decide('review')}>Mark ready for review</button
					>{/if}{#if ['draft', 'needs_research', 'review'].includes(report.status)}<button
						class="rounded-md border border-byl-border-control px-3 py-2"
						disabled={!!busy || !!pending || !note.trim()}
						onclick={() => decide('declined')}>Decline report</button
					>{/if}
			</div>
			<label class="flex items-start gap-2"
				><input type="checkbox" bind:checked={temporary} disabled={!!busy} />Temporary detour;
				retain a reassessment date without inferring reopening.</label
			>
			{#if temporary}<label class="block"
					>Review detour on<input
						class="mt-1 block rounded-md border-byl-border-control"
						type="date"
						bind:value={reviewOn}
						disabled={!!busy}
					/></label
				>{/if}
			<label class="flex items-start gap-2"
				><input type="checkbox" bind:checked={confirmed} disabled={!!busy || !canRelease} />I
				reviewed the current paths, source/observation evidence, stop/story impacts and restrictions
				for this release.</label
			>
			<button
				class="rounded-md bg-byl-action px-4 py-2 text-byl-action-text disabled:opacity-50"
				disabled={!!busy || !canRelease || !confirmed || !note.trim() || (temporary && !reviewOn)}
				onclick={release}>Approve and publish this correction</button
			>
			{#if released}<p class="text-sm">
					<a class="underline" href={`/tours/${released.tour_id}`}>Open the newly published tour</a
					>. Started trips require an explicit visitor update.
				</p>{/if}
		</section>
	{:else if busy}<p role="status">Loading private report…</p>{/if}
</div>
