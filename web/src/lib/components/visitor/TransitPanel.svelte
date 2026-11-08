<script lang="ts">
	import { onMount, untrack } from 'svelte';
	import { session } from '#lib/auth.ts';
	import { itineraryCapabilities, requestTransitPlan, type TourRevision } from '#lib/visitor.ts';
	import { TransitTrip } from '#lib/tour/transit-trip.svelte.ts';
	import { transitPlanStatus, planCanGuide } from '#lib/tour/transit.ts';
	import { walkingHandoffURL } from '#lib/tour/directions.ts';
	import { modal } from '#lib/modal.ts';
	let { revision, controller }: { revision: TourRevision; controller?: TransitTrip } = $props();
	const owner = untrack(() => session.account()?.id ?? 'anonymous');
	const own = untrack(() => !controller);
	const selected = untrack(() => revision.snapshot!.tour.transit!);
	const tour = untrack(() => revision.snapshot!.tour);
	const control = untrack(
		() =>
			controller ??
			new TransitTrip(
				revision,
				{
					base: import.meta.env.VITE_API_BASE_URL || window.location.origin,
					enabled: async () => (await itineraryCapabilities()).scheduled_transit_v1,
					plan: requestTransitPlan
				},
				localStorage,
				owner,
				() => (session.account()?.id ?? 'anonymous') === owner,
				null
			)
	);
	let now = $state(Date.now());
	const localInput = (time: number) =>
		new Date(time - new Date(time).getTimezoneOffset() * 60000).toISOString().slice(0, 16);
	let departure = $state(localInput(Date.now() + 3600000));
	let handoff = $state<{ label: string; url: string } | null>(null);
	const from = tour.stops.find((s) => s.id === selected.intent.from_stop_id);
	const to = tour.stops.find((s) => s.id === selected.intent.to_stop_id);
	const zone = Intl.DateTimeFormat().resolvedOptions().timeZone;
	onMount(() => {
		const refresh = async () => {
			await control.refreshGate();
			now = Date.now();
		};
		void refresh();
		const update = () => {
			now = Date.now();
			if (document.visibilityState === 'visible') void refresh();
			else control.gate = null;
		};
		const timer = setInterval(update, 30_000);
		document.addEventListener('visibilitychange', update);
		return () => {
			clearInterval(timer);
			document.removeEventListener('visibilitychange', update);
			if (own) control.end();
		};
	});
	function time(value: string | undefined) {
		return value
			? new Intl.DateTimeFormat(undefined, {
					timeZone: 'America/Chicago',
					dateStyle: 'medium',
					timeStyle: 'short'
				}).format(new Date(value))
			: 'Unavailable';
	}
	function label(kind: string) {
		return (
			(
				{
					walk: 'Walk between story stops',
					walk_to_stop: 'Walk to boarding platform',
					boarding_wait: 'Wait and check station signs',
					transit: 'Scheduled CTA Blue Line ride',
					walk_from_stop: 'Walk from alighting platform to story stop'
				} as Record<string, string>
			)[kind] ?? 'Unavailable section'
		);
	}
	function stopName(id: string | undefined) {
		return tour.stops.find((s) => s.id === id)?.place.name ?? '';
	}
	async function propose(event: SubmitEvent) {
		event.preventDefault();
		const value = new Date(departure);
		await control.request(Number.isFinite(value.getTime()) ? value.toISOString() : '');
		now = Date.now();
	}
	function openDirections(board: boolean) {
		if (!control.isAdmitted(now) || !planCanGuide(control.selected, now)) return;
		const point = board
			? { latitude: selected.board.latitude, longitude: selected.board.longitude }
			: to?.place.arrival_point?.point;
		if (point?.latitude === undefined || point?.longitude === undefined) return;
		handoff = {
			label: board
				? `boarding platform ${selected.board.name}`
				: `reviewed arrival at ${to?.place.name}`,
			url: walkingHandoffURL(
				{ lat: point.latitude, lng: point.longitude },
				navigator.userAgent,
				board
					? from?.place.arrival_point?.point
						? {
								lat: from.place.arrival_point.point.latitude,
								lng: from.place.arrival_point.point.longitude
							}
						: undefined
					: { lat: selected.alight.latitude, lng: selected.alight.longitude }
			)
		};
	}
</script>

<section
	aria-label="Scheduled transit connection"
	class="space-y-3 rounded-lg border border-byl-border bg-byl-surface p-4 text-sm"
>
	<h2 class="text-lg font-semibold">Walk + scheduled CTA connection</h2>
	<p>
		Leave <strong>{from?.place.name ?? 'the connector’s first story stop'}</strong>, board at
		<strong>{selected.board.name}</strong>
		(platform {selected.board.id}), alight at <strong>{selected.alight.name}</strong> (platform {selected
			.alight.id}), then walk to <strong>{to?.place.name ?? 'the next story stop'}</strong>.
	</p>
	<p class="text-byl-muted">
		CTA times are scheduled, not live arrivals or disruption information. Platform coordinates come
		from the timetable and are not reviewed street entrances or an accessibility certificate.
	</p>
	<p>
		Walking assumptions: {selected.intent.walking_assumptions} Allow {Math.ceil(
			selected.intent.walk_to_stop_seconds / 60
		)} minutes to the platform, {Math.ceil(selected.intent.boarding_buffer_seconds / 60)} minutes boarding
		buffer, and {Math.ceil(selected.intent.walk_from_stop_seconds / 60)} minutes from the platform.
	</p>
	<a
		class="inline-block text-byl-action underline"
		href="https://www.transitchicago.com/blueline/"
		target="_blank"
		rel="noreferrer">Check current CTA Blue Line service and alerts ↗</a
	>
	{#if control.gate !== true}<p role="status" class="text-byl-warning">
			{control.gate === false
				? 'Scheduled transit is disabled or unavailable.'
				: 'Scheduled transit admission could not be checked.'} The retained timetable is unchanged; no
			train is selected automatically.
		</p>{/if}
	{#if control.message}<p role="status" class="text-byl-warning">{control.message}</p>{/if}
	<form onsubmit={propose} class="space-y-2 border-t border-byl-border pt-3">
		<label class="block font-semibold"
			>When will you leave {from?.place.name ?? 'the connector’s first story stop'}?<input
				class="mt-1 block w-full rounded border border-byl-border-control bg-byl-background p-2"
				type="datetime-local"
				bind:value={departure}
				required
				min={localInput(now)}
				max={localInput(now + 7 * 86400000)}
			/></label
		>
		<p class="text-byl-muted">
			Enter your device time ({zone}). Earlier stories, stops and narration are outside this
			departure time. Results below use America/Chicago.
		</p>
		<button
			type="submit"
			disabled={control.busy}
			class="rounded border border-byl-border-control px-3 py-2 disabled:opacity-50"
			>{control.busy ? 'Checking timetable…' : 'Request a timetable to review'}</button
		>
	</form>
	{#if control.proposed}
		{@const result = control.proposed}
		<div
			class="space-y-2 rounded border border-byl-border-control bg-byl-background p-3"
			aria-label="Timetable proposal"
		>
			<h3 class="font-semibold">Timetable to review</h3>
			<p>{transitPlanStatus(result, now)}</p>
			{#if result.state === 'scheduled'}
				<p>
					{selected.board.name}: {time(result.service.departure_at)} → {selected.alight.name}: {time(
						result.service.arrival_at
					)} · America/Chicago
				</p>
				<p>
					From {stopName(result.departure_context_stop_id)} at {time(
						result.requested_departure_at
					)}. The walking estimates and boarding buffer have been added.
				</p>
				<button
					class="rounded bg-byl-action px-3 py-2 text-byl-action-text disabled:opacity-50"
					disabled={!control.isAdmitted(now) || !planCanGuide(result, now)}
					onclick={() => control.accept()}>Use this scheduled plan</button
				>
			{/if}
		</div>
	{/if}
	{#if control.selected}
		{@const saved = control.selected}
		<div aria-label="Retained scheduled plan" class="space-y-3 border-t border-byl-border pt-3">
			<h3 class="font-semibold">Retained scheduled plan · Revision {revision.number}</h3>
			<p role="status">{transitPlanStatus(saved, now)}</p>
			<p class="text-byl-muted">
				This is the plan you explicitly chose. Reopening and route-update checks do not select a new
				train. Stories and stop visits stay on the pinned revision.
			</p>
			<ol class="list-decimal space-y-2 pl-5">
				{#each saved.segments as segment, index (index)}<li>
						<strong>{label(segment.kind)}</strong> · {Math.ceil(segment.duration_s / 60)} min
						{#if segment.kind === 'walk'}<span>
								· {stopName(segment.from_stop_id)} → {stopName(segment.to_stop_id)} · Retained qualified
								walking route</span
							>
						{:else if segment.kind === 'transit'}<span>
								· {selected.board.name} → {selected.alight.name} · {time(
									segment.departure_at
								)}–{time(segment.arrival_at)} America/Chicago · Static rail timetable</span
							>
						{:else}<span>
								· {segment.kind === 'boarding_wait'
									? 'Schedule and curator buffer'
									: 'Curator walking estimate; external directions required'}</span
							>{/if}
					</li>{/each}
			</ol>
			{#if saved.service.source}<p class="text-byl-muted">
					{saved.service.source.publisher} static timetable, retrieved {time(
						saved.service.source.retrieved_at
					)} America/Chicago.
				</p>{/if}
			{#if control.isAdmitted(now) && planCanGuide(saved, now)}
				<div class="flex flex-wrap gap-2">
					<button
						class="rounded border border-byl-border-control px-3 py-2"
						onclick={() => openDirections(true)}>Walking directions to boarding platform</button
					>
					{#if to?.place.arrival_point?.point}<button
							class="rounded border border-byl-border-control px-3 py-2"
							onclick={() => openDirections(false)}
							>Walking directions from alighting station to story stop</button
						>{/if}
				</div>{/if}
		</div>
	{:else}<p>
			No timetable has been retained for this revision. Story stops and progress remain available;
			the rail section has no continuous walking line.
		</p>{/if}
	<button class="text-byl-action underline" onclick={() => control.remove()}
		>Remove saved timetable from this device</button
	>
</section>
{#if handoff}
	<div class="bg-byl-scrim fixed inset-0 z-50 flex items-center justify-center p-4">
		<div
			role="dialog"
			aria-labelledby="transit-handoff-title"
			use:modal={() => (handoff = null)}
			class="max-w-md space-y-3 rounded-lg bg-byl-background p-5 text-byl-text"
		>
			<h2 id="transit-handoff-title" class="text-lg font-semibold">Hand off walking directions?</h2>
			<p>
				Open another app for walking directions to {handoff.label}. That app chooses its own route
				and may use your location. Byl does not guide the station walk or verify entrances, access,
				train operations or live service. Your Byl stop progress stays separate.
			</p>
			<a
				href={handoff.url}
				target="_blank"
				rel="noreferrer"
				class="inline-block rounded bg-byl-action px-3 py-2 text-byl-action-text"
				onclick={() => (handoff = null)}>Open external walking directions ↗</a
			>
			<button
				class="rounded border border-byl-border-control px-3 py-2"
				onclick={() => (handoff = null)}>Stay in Byl</button
			>
		</div>
	</div>
{/if}
