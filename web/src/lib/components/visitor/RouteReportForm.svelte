<script lang="ts">
	import { onDestroy, tick, untrack } from 'svelte';
	import { session } from '#lib/auth.ts';
	import { ApiError } from '#lib/api/index.ts';
	import {
		submitRouteReport,
		type RouteReport,
		type RouteReportInput,
		type TourRevision
	} from '#lib/visitor.ts';
	import { reportReasons, reportTargets } from '#lib/routes/reports.ts';
	import Map from '#lib/components/Map.svelte';
	let {
		revision,
		mode,
		ownerID,
		initialTarget = '',
		onsaved
	}: {
		revision: TourRevision;
		mode: 'walk' | 'bike';
		ownerID: string;
		initialTarget?: string;
		onsaved: (report: RouteReport) => Promise<void>;
	} = $props();
	type Values = {
		target: string;
		reason: RouteReportInput['reason'];
		details: string;
		better: string;
		source: string;
		direction: 'forward' | 'reverse' | 'unknown';
		duration: 'permanent' | 'temporary' | 'unknown';
		observed: string;
		annotation: { lat: number; lng: number } | null;
		alternative: [number, number][];
	};
	type Attempt = { key: string; submitted: RouteReportInput };
	const targets = $derived(reportTargets(revision, mode));
	const storageKey = untrack(
		() =>
			`byl.route-report.v1:${ownerID}:${import.meta.env.VITE_API_BASE_URL ?? location.origin}:${revision.id}:${mode}`
	);
	const fresh = (): Values => ({
		target: targets.some((t) => t.key === initialTarget) ? initialTarget : (targets[0]?.key ?? ''),
		reason: 'blocked',
		details: '',
		better: '',
		source: '',
		direction: 'unknown',
		duration: 'unknown',
		observed: '',
		annotation: null,
		alternative: []
	});
	function restore(): { values: Values; pending: Attempt | null } {
		const empty = { values: fresh(), pending: null };
		try {
			const raw = sessionStorage.getItem(storageKey);
			if (!raw) return empty;
			const saved = JSON.parse(raw);
			if (
				!saved.values ||
				![
					'target',
					'reason',
					'details',
					'better',
					'source',
					'direction',
					'duration',
					'observed'
				].every((k) => typeof saved.values[k] === 'string') ||
				!Array.isArray(saved.values.alternative) ||
				saved.values.alternative.length > 64
			)
				return empty;
			const attempt = saved.pending;
			if (
				attempt &&
				(!/^[0-9a-f-]{36}$/.test(attempt.key) ||
					attempt.submitted?.tour_id !== revision.tour_id ||
					attempt.submitted?.revision_id !== revision.id ||
					attempt.submitted?.mode !== mode)
			)
				return empty;
			return { values: saved.values, pending: attempt ?? null };
		} catch {
			return empty;
		}
	}
	const initial = untrack(restore);
	let values = $state<Values>(initial.values);
	let pending = $state<Attempt | null>(initial.pending);
	let busy = $state(false);
	let completed = $state(false);
	let needsSignIn = $state(false);
	let message = $state('');
	let errors = $state<Record<string, string>>({});
	let confirmed = $state<RouteReport | null>(null);
	let pick = $state<'annotation' | 'alternative' | null>(null);
	let latitude = $state<number | undefined>(undefined);
	let longitude = $state<number | undefined>(undefined);
	let active = true;
	let form: HTMLFormElement;
	onDestroy(() => {
		active = false;
	});
	function persist(nextValues = values, nextPending = pending) {
		try {
			sessionStorage.setItem(
				storageKey,
				JSON.stringify({ values: nextValues, pending: nextPending })
			);
			return true;
		} catch {
			return false;
		}
	}
	$effect(() => {
		if (!completed) persist();
	});
	const locked = $derived(busy || pending !== null || confirmed !== null);
	const selected = $derived(targets.find((t) => t.key === values.target));
	const snapshot = $derived(revision.snapshot!);
	const baseLine = $derived(
		(snapshot.routes[mode]?.geometry?.coordinates ?? []) as [number, number][]
	);
	const markers = $derived([
		...snapshot.tour.stops.map((s) => ({
			lat: s.place.arrival_point?.point?.latitude ?? s.place.latitude,
			lng: s.place.arrival_point?.point?.longitude ?? s.place.longitude,
			label: `${s.position}. ${s.place.name}`,
			position: s.position
		})),
		...(values.annotation ? [{ ...values.annotation, label: 'Reported location' }] : [])
	]);
	const legs = $derived([
		{ coords: baseLine, state: 'future' as const },
		...(values.alternative.length > 1
			? [{ coords: values.alternative, state: 'active' as const }]
			: [])
	]);
	function addPoint(point: { lat: number; lng: number }) {
		if (locked) return;
		if (
			!Number.isFinite(point.lat) ||
			!Number.isFinite(point.lng) ||
			Math.abs(point.lat) > 90 ||
			Math.abs(point.lng) > 180
		) {
			message = 'Enter a valid latitude and longitude.';
			return;
		}
		if (pick === 'annotation') {
			values.annotation = point;
			pick = null;
		} else if (pick === 'alternative') {
			if (values.alternative.length >= 64) {
				message = 'An alternative can contain at most 64 points.';
				return;
			}
			values.alternative = [...values.alternative, [point.lng, point.lat]];
		}
		latitude = undefined;
		longitude = undefined;
	}
	function addCoordinate() {
		if (latitude === undefined || longitude === undefined) {
			message = 'Enter both latitude and longitude.';
			return;
		}
		addPoint({ lat: Number(latitude), lng: Number(longitude) });
	}
	function discard() {
		const empty = fresh();
		if (!persist(empty, null)) {
			message = 'Could not clear the saved retry details. Your draft is kept.';
			return;
		}
		values = empty;
		pending = null;
		errors = {};
		message = '';
		needsSignIn = false;
		pick = null;
	}
	async function focusError() {
		await tick();
		form.querySelector<HTMLElement>('[aria-invalid="true"], [data-invalid="true"]')?.focus();
	}
	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (busy || confirmed || needsSignIn || session.account()?.id !== ownerID) return;
		errors = {};
		message = '';
		pick = null;
		let attempt = pending;
		if (!attempt) {
			if (!selected) {
				errors.affected = 'Select the affected stop, entrance or path.';
				await focusError();
				return;
			}
			if (values.alternative.length === 1) {
				errors.alternative =
					'An alternative needs at least two points, or remove it to send a report only.';
				await focusError();
				return;
			}
			attempt = {
				key: crypto.randomUUID(),
				submitted: {
					tour_id: revision.tour_id,
					revision_id: revision.id,
					mode,
					affected: selected.affected,
					reason: values.reason,
					details: values.details.trim(),
					better: values.better.trim(),
					direction: values.direction,
					duration: values.duration,
					observed_on: values.observed || null,
					source_url: values.source.trim(),
					annotation: values.annotation,
					alternative: values.alternative.length
						? { type: 'LineString', coordinates: values.alternative }
						: null
				}
			};
			pending = attempt;
		}
		if (!persist(values, attempt)) {
			message =
				'Could not save the retry details in this browser. No report was sent. Retry when storage is available.';
			return;
		}
		busy = true;
		let saved: RouteReport | null = null;
		try {
			saved = await submitRouteReport(attempt.submitted, attempt.key);
			if (!active || session.account()?.id !== ownerID) return;
			if (saved.owner_account_id !== ownerID) throw new Error('Report owner was not confirmed');
			confirmed = saved;
			completed = true;
			try {
				sessionStorage.removeItem(storageKey);
			} catch {
				/* Retained key still resolves to this confirmed report. */
			}
		} catch (error) {
			if (!active || session.account()?.id !== ownerID) return;
			if (error instanceof ApiError && error.status === 401) {
				needsSignIn = true;
				message =
					'Sign in again to send this report. The original input and retry key are kept in this tab.';
			} else if (
				error instanceof ApiError &&
				error.status >= 400 &&
				error.status < 500 &&
				error.status !== 409
			) {
				if (!persist(values, null)) {
					message =
						'The report was rejected, but local retry details could not be cleared. Your input and retry identity are kept.';
					return;
				}
				pending = null;
				errors = error.fields;
				message = Object.keys(errors).length
					? 'Check the marked fields and submit again.'
					: error.message;
				await focusError();
			} else {
				message =
					'We could not confirm the report. Retry sends the same report. Check your reports before discarding it.';
			}
		} finally {
			if (active) busy = false;
		}
		if (saved && confirmed && active && session.account()?.id === ownerID) {
			try {
				await onsaved(saved);
			} catch {
				message = 'Report saved privately. Open its status using the link below.';
			}
		}
	}
</script>

<form bind:this={form} onsubmit={submit} class="mt-6 max-w-3xl space-y-5">
	<p class="text-sm text-byl-muted">
		Reporting revision {revision.number} · {mode === 'bike' ? 'Cycling' : 'Walking'}. Your
		observation is private. An alternative is optional; reporting does not change this trip.
	</p>
	{#if message}<p role="alert" class="border border-byl-warning bg-byl-surface p-3 text-sm">
			{message}
		</p>{/if}
	{#if needsSignIn}<a href="/signin" class="underline">Sign in again</a>{/if}
	{#if confirmed}<p role="status">
			Report saved privately. <a class="underline" href={`/routes/reports/${confirmed.id}`}
				>Open saved report</a
			>
		</p>{/if}
	<div>
		<label for="report-target" class="block font-medium">Affected stop, entrance or path</label>
		<select
			id="report-target"
			class="mt-2 w-full rounded-md border-byl-border-control"
			bind:value={values.target}
			disabled={locked}
			required
			aria-invalid={errors.affected ? 'true' : undefined}
			aria-describedby={errors.affected ? 'report-target-error' : undefined}
		>
			{#each targets as target (target.key)}<option value={target.key}>{target.label}</option
				>{/each}</select
		>
		{#if errors.affected}<p id="report-target-error" class="mt-1 text-sm text-byl-warning">
				{errors.affected}
			</p>{/if}
	</div>
	<div>
		<label for="report-reason" class="block font-medium">What is the problem?</label><select
			id="report-reason"
			class="mt-2 w-full rounded-md border-byl-border-control"
			bind:value={values.reason}
			disabled={locked}
			required
			>{#each reportReasons as reason (reason.value)}<option value={reason.value}
					>{reason.label}</option
				>{/each}</select
		>
	</div>
	<div>
		<label for="report-details" class="block font-medium">What did you observe?</label><textarea
			id="report-details"
			class="mt-2 w-full rounded-md border-byl-border-control"
			rows="4"
			maxlength="4000"
			bind:value={values.details}
			required={values.reason === 'other'}
			disabled={locked}
			aria-invalid={errors.details ? 'true' : undefined}
			aria-describedby={errors.details ? 'report-details-error' : undefined}
		></textarea>{#if errors.details}<p id="report-details-error" class="text-sm text-byl-warning">
				{errors.details}
			</p>{/if}
	</div>
	{#if values.reason === 'improvement'}<div>
			<label for="report-better" class="block font-medium">What would be better?</label><textarea
				id="report-better"
				class="mt-2 w-full rounded-md border-byl-border-control"
				maxlength="1000"
				rows="3"
				bind:value={values.better}
				disabled={locked}
				required
				aria-invalid={errors.better ? 'true' : undefined}
				aria-describedby={errors.better ? 'report-better-error' : undefined}
			></textarea>{#if errors.better}<p id="report-better-error" class="text-sm text-byl-warning">
					{errors.better}
				</p>{/if}
		</div>{/if}
	<details>
		<summary class="cursor-pointer font-medium">Observation details and optional source</summary>
		<div class="mt-3 grid gap-4 sm:grid-cols-2">
			<label
				>Travel direction<select
					class="mt-2 w-full rounded-md border-byl-border-control"
					bind:value={values.direction}
					disabled={locked}
					><option value="unknown">Not sure</option><option value="forward"
						>Following the displayed itinerary</option
					><option value="reverse">Opposite the displayed itinerary</option></select
				></label
			>
			<label
				>How long might it last?<select
					class="mt-2 w-full rounded-md border-byl-border-control"
					bind:value={values.duration}
					disabled={locked}
					><option value="unknown">Not sure</option><option value="temporary">Temporary</option
					><option value="permanent">Permanent</option></select
				></label
			>
			<label
				>Date observed, if known<input
					class="mt-2 w-full rounded-md border-byl-border-control"
					type="date"
					bind:value={values.observed}
					disabled={locked}
				/></label
			>
			<label
				>Source link, if available<input
					class="mt-2 w-full rounded-md border-byl-border-control"
					type="url"
					maxlength="2000"
					bind:value={values.source}
					disabled={locked}
					aria-invalid={errors.source_url ? 'true' : undefined}
					aria-describedby={errors.source_url ? 'report-source-error' : undefined}
				/>{#if errors.source_url}<span id="report-source-error" class="text-sm text-byl-warning"
						>{errors.source_url}</span
					>{/if}</label
			>
		</div>
	</details>
	<fieldset
		class="space-y-3"
		tabindex="-1"
		data-invalid={errors.annotation || errors.alternative ? 'true' : undefined}
		aria-describedby={errors.annotation || errors.alternative ? 'report-geometry-error' : undefined}
	>
		<legend class="font-medium">Optional location or alternative path</legend>
		<p class="text-sm text-byl-muted">
			Choose an affected area above to send a report only. You can also mark one location or draw a
			local alternative. Points are suggestions for review, not approved access.
		</p>
		{#if errors.annotation || errors.alternative}<p
				id="report-geometry-error"
				class="text-sm text-byl-warning"
			>
				{errors.annotation ?? errors.alternative}
			</p>{/if}
		<div class="flex flex-wrap gap-2">
			<button
				type="button"
				class="rounded-md border border-byl-border-control px-3 py-2"
				disabled={locked}
				aria-pressed={pick === 'annotation'}
				onclick={() => (pick = pick === 'annotation' ? null : 'annotation')}
				>Mark problem location</button
			><button
				type="button"
				class="rounded-md border border-byl-border-control px-3 py-2"
				disabled={locked}
				aria-pressed={pick === 'alternative'}
				onclick={() => (pick = pick === 'alternative' ? null : 'alternative')}
				>Draw an alternative</button
			>
		</div>
		{#if pick}<p role="status" class="text-sm">
				{pick === 'annotation'
					? 'Choose one point on the map.'
					: 'Choose points on the map in travel order.'} You can enter a coordinate below instead.
			</p>{/if}
		<Map {markers} {legs} height="20rem" onpoint={pick && !locked ? addPoint : null} />
		<p class="text-xs text-byl-muted">
			Dashed: published path. Solid: suggested alternative. Keep points close to the selected area;
			alternatives must connect to its endpoints.
		</p>
		{#if pick}<div class="flex flex-wrap items-end gap-3">
				<label
					>Latitude<input
						class="mt-1 block w-36 rounded-md border-byl-border-control"
						type="number"
						step="any"
						min="-90"
						max="90"
						bind:value={latitude}
					/></label
				><label
					>Longitude<input
						class="mt-1 block w-36 rounded-md border-byl-border-control"
						type="number"
						step="any"
						min="-180"
						max="180"
						bind:value={longitude}
					/></label
				><button
					type="button"
					class="rounded-md border border-byl-border-control px-3 py-2"
					onclick={addCoordinate}>Add coordinate</button
				>
			</div>{/if}
		{#if values.annotation}<p class="text-sm">
				Marked location: {values.annotation.lat.toFixed(5)}, {values.annotation.lng.toFixed(5)}
				<button
					type="button"
					disabled={locked}
					class="ml-2 underline"
					onclick={() => (values.annotation = null)}>Remove marker</button
				>
			</p>{/if}
		{#if values.alternative.length}<ol class="space-y-1 text-sm">
				{#each values.alternative as point, index (index)}<li>
						Alternative point {index + 1}: {point[1].toFixed(5)}, {point[0].toFixed(5)}
						<button
							type="button"
							disabled={locked}
							class="ml-2 underline"
							aria-label={`Remove alternative point ${index + 1}`}
							onclick={() =>
								(values.alternative = values.alternative.filter((_, i) => i !== index))}
							>Remove</button
						>
					</li>{/each}
			</ol>{/if}
	</fieldset>
	{#if pending && !confirmed}<p class="text-sm">
			Input is locked while this report has a retry identity. Retry preserves the selected revision
			and original observation.
		</p>{/if}
	<div class="flex flex-wrap gap-3">
		{#if !confirmed}<button
				type="submit"
				disabled={busy || needsSignIn}
				class="rounded-md bg-byl-action px-4 py-3 text-byl-action-text"
				>{busy ? 'Sending…' : pending ? 'Retry report' : 'Send private report'}</button
			><button
				type="button"
				disabled={busy}
				class="rounded-md border border-byl-border-control px-4 py-3"
				onclick={discard}>Discard local draft</button
			>{/if}<a href="/routes/reports" class="self-center underline">Your reports</a>
	</div>
</form>
