<script lang="ts">
	import { safeHref } from '#lib/format.ts';
	import { onDestroy, untrack } from 'svelte';
	import { session } from '#lib/auth.ts';
	import { ApiError, type Schemas } from '#lib/api/index.ts';
	import {
		getPlace,
		proposeArrival,
		reviewArrival,
		proposeCorridor,
		reviewCorridor,
		recomputeRoutes,
		errorMessage,
		type ArrivalProposal,
		type CorridorProposal,
		type RouteReport
	} from '#lib/admin.ts';
	import MapView from './Map.svelte';
	let {
		tour,
		report,
		onchanged,
		onpending = () => {}
	}: {
		tour: Schemas['TourDetail'];
		report: RouteReport;
		onchanged: () => Promise<void>;
		onpending?: (pending: boolean) => void;
	} = $props();
	const actor = untrack(() => session.account()?.id);
	let active = true;
	onDestroy(() => {
		active = false;
		onpending(false);
	});
	const current = () => active && session.account()?.id === actor;
	const places = $derived([...new Map(tour.stops.map((s) => [s.place.id, s.place])).values()]);
	let placeID = $state(
		untrack(
			() =>
				tour.stops.find((s) => s.id === report.submitted.affected.stop_id)?.place.id ??
				tour.stops[0]?.place.id ??
				''
		)
	);
	const selected = $derived(places.find((p) => p.id === placeID));
	type Point = Schemas['ArrivalPoint'];
	function pointDraft(place: Schemas['PlaceSummary'] | undefined): Point {
		const old = place?.arrival_point?.point;
		return {
			name: old?.name ?? place?.name ?? '',
			kind: old?.kind ?? 'entrance',
			latitude: old?.latitude ?? place?.latitude ?? 0,
			longitude: old?.longitude ?? place?.longitude ?? 0,
			notes: old?.notes ?? '',
			access: (['walk', 'bike'] as const).map((mode) => ({
				...old?.access.find((a) => a.mode === mode),
				mode,
				status: old?.access.find((a) => a.mode === mode)?.status ?? 'unknown',
				surface: old?.access.find((a) => a.mode === mode)?.surface ?? 'unknown'
			}))
		};
	}
	let point = $state<Point>(untrack(() => pointDraft(selected)));
	let arrivalVersion = $state(untrack(() => selected?.arrival_point?.version ?? 0));
	let arrival = $state<ArrivalProposal | null>(null);
	let arrivalNote = $state('');
	let arrivalChecked = $state(false);
	let corridor = $state<CorridorProposal | null>(null);
	let corridorNote = $state('');
	let corridorChecked = $state(false);
	let corridorVersion = $state(untrack(() => tour.corridor?.version ?? 0));
	let referenceFile = $state<File | null>(null);
	let busy = $state('');
	let message = $state('');
	let error = $state('');
	const pending = $derived(arrival?.status === 'pending' || corridor?.status === 'pending');
	$effect(() => {
		onpending(pending);
	});
	const sourceLegs = $derived(
		(corridor?.intent?.segments ?? []).map((segment) => ({
			coords: segment.geometry.coordinates as [number, number][],
			state: (corridor?.intent?.routes.some(
				(r) => r.mode === report.submitted.mode && r.path.some((p) => p.segment_id === segment.id)
			)
				? 'active'
				: 'future') as 'active' | 'future'
		}))
	);
	function failed(e: unknown, fallback: string) {
		error = errorMessage(e, fallback);
		if (e instanceof ApiError && Object.keys(e.fields).length)
			error +=
				' ' +
				Object.entries(e.fields)
					.map(([field, detail]) => `${field}: ${detail}`)
					.join('; ');
	}
	function resetAccess() {
		point.access = point.access.map((a) => ({ ...a, status: 'unknown', surface: 'unknown' }));
		arrivalChecked = false;
	}
	function changePlace() {
		point = pointDraft(selected);
		arrivalVersion = selected?.arrival_point?.version ?? 0;
		arrival = null;
		arrivalChecked = false;
		arrivalNote = '';
		error = '';
		message = '';
	}
	async function loadArrival() {
		if (busy || arrival?.status === 'pending') return;
		busy = 'arrival-load';
		error = '';
		try {
			const fresh = await getPlace(placeID);
			if (!current()) return;
			point = pointDraft(fresh);
			arrivalVersion = fresh.arrival_point?.version ?? 0;
			arrival = null;
			arrivalChecked = false;
		} catch (e) {
			if (current()) failed(e, 'Could not load current arrival.');
		} finally {
			if (current()) busy = '';
		}
	}
	async function queueArrival() {
		if (busy || arrival?.status === 'pending') return;
		busy = 'arrival-propose';
		error = '';
		message = '';
		try {
			const saved = await proposeArrival(placeID, { version: arrivalVersion, point });
			if (!current()) return;
			arrival = saved;
			arrivalChecked = false;
			message =
				'Arrival proposal retained privately. Review its source and both travel modes before approval.';
			await onchanged();
		} catch (e) {
			if (current()) failed(e, 'Could not prepare the arrival. Your fields are kept.');
		} finally {
			if (current()) busy = '';
		}
	}
	async function decideArrival(action: 'approve' | 'reject') {
		const proposal = arrival;
		if (
			busy ||
			!proposal?.id ||
			proposal.base_version === undefined ||
			!arrivalNote.trim() ||
			(action === 'approve' && !arrivalChecked)
		)
			return;
		busy = 'arrival-review';
		error = '';
		message = '';
		try {
			const saved = await reviewArrival(placeID, proposal.id, {
				version: proposal.base_version,
				action,
				note: arrivalNote
			});
			if (!current()) return;
			arrival = saved;
			arrivalChecked = false;
			if (saved.status === 'approved') arrivalVersion = proposal.base_version + 1;
			message =
				action === 'approve'
					? 'Arrival approved for the working data. Recompute affected paths and reload the comparison.'
					: 'Arrival proposal declined; the approved point is unchanged.';
			await onchanged();
		} catch (e) {
			if (current())
				failed(
					e,
					'Arrival review changed. Reload current data and prepare a fresh proposal; your note is kept.'
				);
		} finally {
			if (current()) busy = '';
		}
	}
	async function queueReference() {
		if (busy || !referenceFile || corridor?.status === 'pending') return;
		if (referenceFile.size > 256 * 1024) {
			error = 'Reference files allow at most 256 KiB.';
			return;
		}
		busy = 'corridor-propose';
		error = '';
		message = '';
		try {
			const raw: unknown = JSON.parse(await referenceFile.text());
			if (!current()) return;
			if (!raw || typeof raw !== 'object' || Array.isArray(raw))
				throw new Error(
					'Choose a corridor reference object or a bundle with intent and quality_evidence.'
				);
			const imported = raw as Record<string, unknown>;
			const saved = await proposeCorridor(tour.id, {
				version: corridorVersion,
				intent: (imported.intent ?? raw) as Schemas['CorridorIntent'],
				remove: false,
				quality_evidence: (imported.quality_evidence ?? []) as Schemas['CorridorEvidenceRef'][]
			});
			if (!current()) return;
			corridor = saved;
			corridorChecked = false;
			message =
				'Reference retained privately. Inspect its selected branches, direction, sources and restrictions before approval.';
			await onchanged();
		} catch (e) {
			if (current()) failed(e, 'Could not import this reviewed reference.');
		} finally {
			if (current()) busy = '';
		}
	}
	async function decideReference(action: 'approve' | 'reject') {
		const proposal = corridor;
		if (
			busy ||
			!proposal?.id ||
			proposal.base_version === undefined ||
			!corridorNote.trim() ||
			(action === 'approve' && !corridorChecked)
		)
			return;
		busy = 'corridor-review';
		error = '';
		message = '';
		try {
			const saved = await reviewCorridor(tour.id, proposal.id, {
				version: proposal.base_version,
				action,
				note: corridorNote
			});
			if (!current()) return;
			corridor = saved;
			corridorChecked = false;
			if (saved.status === 'approved') corridorVersion = proposal.base_version + 1;
			message =
				action === 'approve'
					? 'Routing intent approved for this working tour. Recompute its paths, inspect their quality and reload the release comparison.'
					: 'Reference proposal declined; approved intent is unchanged.';
			await onchanged();
		} catch (e) {
			if (current())
				failed(
					e,
					'The tour or reference changed. Prepare a fresh reference proposal; your note is kept.'
				);
		} finally {
			if (current()) busy = '';
		}
	}
	async function recompute() {
		if (busy || pending) return;
		busy = 'recompute';
		error = '';
		try {
			await recomputeRoutes(tour.id);
			if (!current()) return;
			message =
				'Path computation queued. Reload the comparison after it completes; publication stays blocked while quality or access is unresolved.';
			await onchanged();
		} catch (e) {
			if (current()) failed(e, 'Could not queue path computation.');
		} finally {
			if (current()) busy = '';
		}
	}
</script>

<section
	aria-label="Prepare reviewed navigation"
	class="mt-6 space-y-5 border border-byl-border bg-byl-surface p-4"
>
	<h2 class="text-xl font-semibold">Prepare an adjustment</h2>
	<p class="text-sm text-byl-muted">
		Arrival and reference approvals change working navigation inputs. The release comparison is
		checked again before a new tour revision is published. A shared place can affect several working
		tours; their published revisions stay retained.
	</p>
	{#if error}<p role="alert" class="text-sm break-words text-byl-danger">
			{error}
		</p>{/if}{#if message}<p role="status" class="text-sm">{message}</p>{/if}
	<details>
		<summary class="cursor-pointer font-semibold">Review an entrance or arrival point</summary>
		<div class="mt-4 space-y-4">
			<label class="block"
				>Place<select
					class="mt-2 w-full rounded-md border-byl-border-control"
					bind:value={placeID}
					onchange={changePlace}
					disabled={!!busy || arrival?.status === 'pending'}
					>{#each places as place (place.id)}<option value={place.id}>{place.name}</option
						>{/each}</select
				></label
			>
			<button
				type="button"
				class="text-sm underline"
				disabled={!!busy || arrival?.status === 'pending'}
				onclick={loadArrival}>Load current approved arrival</button
			>
			<p class="text-xs text-byl-muted">
				Approved arrival version {arrivalVersion}. Known access needs a source for each mode. Moving
				the point resets access and surface to unknown for a fresh review.
			</p>
			<fieldset disabled={!!busy || arrival?.status === 'pending'} class="min-w-0 space-y-3">
				<div class="grid gap-3 sm:grid-cols-2">
					<label
						>Name<input
							class="mt-1 w-full rounded-md border-byl-border-control"
							bind:value={point.name}
							maxlength="300"
						/></label
					><label
						>Arrival kind<select
							class="mt-1 w-full rounded-md border-byl-border-control"
							bind:value={point.kind}
							><option value="entrance">Entrance</option><option value="viewpoint">Viewpoint</option
							></select
						></label
					><label
						>Arrival latitude<input
							class="mt-1 w-full rounded-md border-byl-border-control"
							type="number"
							step="any"
							min="-90"
							max="90"
							bind:value={point.latitude}
							oninput={resetAccess}
						/></label
					><label
						>Arrival longitude<input
							class="mt-1 w-full rounded-md border-byl-border-control"
							type="number"
							step="any"
							min="-180"
							max="180"
							bind:value={point.longitude}
							oninput={resetAccess}
						/></label
					>
				</div>
				{#if report.submitted.annotation && tour.stops.some((s) => s.id === report.submitted.affected.stop_id && s.place.id === placeID)}<button
						type="button"
						class="text-sm underline"
						onclick={() => {
							point.latitude = report.submitted.annotation!.lat;
							point.longitude = report.submitted.annotation!.lng;
							resetAccess();
						}}>Use the submitted point as an unreviewed draft</button
					>{/if}
				<label class="block"
					>Public arrival guidance<textarea
						class="mt-1 w-full rounded-md border-byl-border-control"
						rows="2"
						maxlength="4000"
						bind:value={point.notes}></textarea></label
				>
				{#each point.access as access, index (access.mode)}<fieldset
						class="min-w-0 space-y-3 border-t border-byl-border pt-3"
					>
						<legend class="font-semibold"
							>{access.mode === 'walk' ? 'Walking' : 'Cycling'} access evidence</legend
						>
						<div class="grid gap-3 sm:grid-cols-2">
							<label
								>Access status<select
									class="mt-1 w-full rounded-md border-byl-border-control"
									bind:value={point.access[index].status}
									><option value="unknown">Unknown</option><option value="allowed">Allowed</option
									><option value="restricted">Restricted</option><option value="closed"
										>Closed</option
									></select
								></label
							><label
								>Surface<select
									class="mt-1 w-full rounded-md border-byl-border-control"
									bind:value={point.access[index].surface}
									>{#each ['unknown', 'paved', 'unpaved', 'mixed', 'steps', 'other'] as surface (surface)}<option
											value={surface}>{surface}</option
										>{/each}</select
								></label
							><label
								>Access source URL<input
									class="mt-1 w-full rounded-md border-byl-border-control"
									type="url"
									maxlength="2000"
									bind:value={point.access[index].source_url}
								/></label
							><label
								>Access source title<input
									class="mt-1 w-full rounded-md border-byl-border-control"
									maxlength="500"
									bind:value={point.access[index].source_title}
								/></label
							><label
								>Source checked on<input
									class="mt-1 w-full rounded-md border-byl-border-control"
									type="date"
									bind:value={point.access[index].retrieved_at}
								/></label
							><label
								>Access notes<textarea
									class="mt-1 w-full rounded-md border-byl-border-control"
									maxlength="2000"
									rows="2"
									bind:value={point.access[index].notes}></textarea></label
							>
						</div>
					</fieldset>{/each}
				<button
					type="button"
					class="rounded-md border border-byl-border-control px-3 py-2"
					onclick={queueArrival}>Prepare arrival proposal</button
				>
			</fieldset>
			{#if arrival?.status === 'pending'}<div class="space-y-3 border-t border-byl-border pt-3">
					<p class="font-semibold">Pending arrival: {arrival.point?.name}</p>
					<p class="text-sm">
						{arrival.point?.latitude}, {arrival.point?.longitude}. Check the original source and
						each mode's restrictions before approving.
					</p>
					<label class="block"
						>Private arrival review reason<textarea
							class="mt-1 w-full rounded-md border-byl-border-control"
							maxlength="4000"
							rows="2"
							bind:value={arrivalNote}></textarea></label
					><label class="flex items-start gap-2"
						><input type="checkbox" bind:checked={arrivalChecked} />I reviewed this location,
						sources and separate walking/cycling access.</label
					>
					<div class="flex flex-wrap gap-2">
						<button
							class="rounded-md border border-byl-border-control px-3 py-2"
							disabled={!!busy || !arrivalChecked || !arrivalNote.trim()}
							onclick={() => decideArrival('approve')}>Approve arrival for working data</button
						><button
							class="rounded-md border border-byl-border-control px-3 py-2"
							disabled={!!busy || !arrivalNote.trim()}
							onclick={() => decideArrival('reject')}>Decline arrival proposal</button
						>
					</div>
				</div>{/if}
		</div>
	</details>
	<details>
		<summary class="cursor-pointer font-semibold">Import and review durable route intent</summary>
		<div class="mt-4 space-y-3">
			<p class="text-sm text-byl-muted">
				Import a bounded corridor reference JSON file, or a bundle with intent and retained
				quality_evidence references. The file must describe sourced branches, explicit connectors,
				mode selections and shaping controls. Submitted report points remain leads. <a
					class="underline"
					href={`/admin/tours/${tour.id}`}>Open the working tour editor</a
				> for story-stop adjustments.
			</p>
			<p class="text-xs text-byl-muted">
				Current approved reference version {corridorVersion}. Changing a stop or source after import
				requires a fresh proposal.
			</p>
			<label class="block"
				>Corridor reference file<input
					class="mt-1 block w-full text-sm"
					type="file"
					accept=".json,application/json"
					disabled={!!busy || corridor?.status === 'pending'}
					onchange={(event) => {
						referenceFile = event.currentTarget.files?.[0] ?? null;
					}}
				/></label
			>
			<div class="flex flex-wrap gap-3">
				<button
					class="rounded-md border border-byl-border-control px-3 py-2"
					disabled={!!busy || !referenceFile || corridor?.status === 'pending'}
					onclick={queueReference}>Import reference for review</button
				><button
					class="text-sm underline"
					disabled={!!busy || corridor?.status === 'pending'}
					onclick={() => {
						corridorVersion = tour.corridor?.version ?? 0;
						corridor = null;
						corridorChecked = false;
					}}>Use current reference version</button
				>
			</div>
			{#if corridor?.intent}<section
					class="space-y-3 border-t border-byl-border pt-3"
					aria-label="Imported routing intent"
				>
					<h3 class="font-semibold">{corridor.intent.name}</h3>
					<p class="text-sm">
						{corridor.intent.source.publisher} · {corridor.intent.source.version} · checked {corridor
							.intent.source.retrieved_on}
					</p>
					<p class="text-sm">
						<a
							class="underline"
							href={safeHref(corridor.intent.source.url)}
							target="_blank"
							rel="noreferrer">Reference source</a
						>
						·
						<a
							class="underline"
							href={safeHref(corridor.intent.source.license_url)}
							target="_blank"
							rel="noreferrer">{corridor.intent.source.license}</a
						>
					</p>
					<MapView legs={sourceLegs} />
					<p class="text-xs text-byl-muted">
						Solid: selected mode references. Dashed: other branches. A line crossing does not create
						a connection.
					</p>
					{#each corridor.intent.routes as route (route.mode)}<p class="text-sm">
							<strong>{route.mode === 'walk' ? 'Walking' : 'Cycling'}:</strong>
							{route.branch} · {route.direction}. {route.path
								.map((p) => `${p.segment_id}${p.reverse ? ' (reverse)' : ''}`)
								.join(' → ')}
						</p>{/each}
					{#if corridor.status === 'pending'}<label class="block"
							>Private reference review reason<textarea
								class="mt-1 w-full rounded-md border-byl-border-control"
								rows="2"
								maxlength="4000"
								bind:value={corridorNote}></textarea></label
						><label class="flex items-start gap-2"
							><input type="checkbox" bind:checked={corridorChecked} />I reviewed topology,
							branch/direction, sources, copying context and mode restrictions.</label
						>
						<div class="flex flex-wrap gap-2">
							<button
								class="rounded-md border border-byl-border-control px-3 py-2"
								disabled={!!busy || !corridorChecked || !corridorNote.trim()}
								onclick={() => decideReference('approve')}>Approve intent for working tour</button
							><button
								class="rounded-md border border-byl-border-control px-3 py-2"
								disabled={!!busy || !corridorNote.trim()}
								onclick={() => decideReference('reject')}>Decline reference proposal</button
							>
						</div>{/if}
				</section>{/if}
		</div>
	</details>
	<button
		class="rounded-md border border-byl-border-control px-3 py-2 disabled:opacity-50"
		disabled={!!busy || pending}
		onclick={recompute}>Recompute prepared paths</button
	>
</section>
