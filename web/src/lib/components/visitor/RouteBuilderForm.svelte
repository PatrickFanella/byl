<script lang="ts">
	import { city } from '#lib/city.svelte.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import { RouteBuilder, MAX_STOPS, shareURL } from '#lib/routes/builder.svelte.ts';
	import {
		createTour,
		listPlaces,
		updateTour,
		type PlaceSummary,
		type TourDetail
	} from '#lib/visitor.ts';
	import { distanceM } from '#lib/tour/arrival.ts';
	import LoadState from './LoadState.svelte';
	import ShareLink from './ShareLink.svelte';

	let { editing, ondone }: { editing: TourDetail | null; ondone: () => void } = $props();

	let builder = $state<RouteBuilder | null>(null);
	let created = $state<TourDetail | null>(null);
	let search = $state('');
	let here = $state<{ lat: number; lng: number } | null>(null);

	const places = new Loader<PlaceSummary[]>(
		async () => (builder ? (await listPlaces(builder.city.slug)).items : []),
		(v) => v.length === 0
	);

	$effect(() => {
		city.load();
	});
	$effect(() => {
		if (builder || city.status !== 'ready') return;
		// A route being edited keeps its own city even if the picker moved on.
		const owning = editing ? city.cities.find((c) => c.id === editing.city_id) : null;
		const c = owning ?? city.current;
		if (!c) return;
		builder = new RouteBuilder(c, editing);
		places.run();
		navigator.geolocation?.getCurrentPosition(
			(p) => (here = { lat: p.coords.latitude, lng: p.coords.longitude }),
			() => {},
			{ maximumAge: 60_000, timeout: 8_000 }
		);
	});

	const filtered = $derived.by(() => {
		const needle = search.trim().toLowerCase();
		let out = (places.data ?? []).filter(
			(p) =>
				!needle || p.name.toLowerCase().includes(needle) || p.address.toLowerCase().includes(needle)
		);
		if (here) {
			const h = here;
			out = [...out].sort(
				(a, b) =>
					distanceM({ lat: a.latitude, lng: a.longitude }, h) -
					distanceM({ lat: b.latitude, lng: b.longitude }, h)
			);
		}
		return out.slice(0, 40);
	});

	async function submit() {
		if (!builder) return;
		const tour = await builder.submit({ create: createTour, update: updateTour });
		if (tour) created = tour;
	}

	const chip = (on: boolean) =>
		`rounded px-3 py-1.5 text-sm ${on ? 'bg-byl-action text-byl-action-text' : 'border border-byl-border text-byl-muted hover:bg-byl-surface'}`;
	const origin = typeof location === 'undefined' ? '' : location.origin;
	const field =
		'mt-1 w-full rounded-lg border border-byl-border bg-byl-background px-3 py-2 text-byl-text';
</script>

{#if created}
	<div class="space-y-4">
		<div class="rounded-lg border border-byl-border bg-byl-surface p-5">
			<div class="text-xs font-bold tracking-normal text-byl-success">
				{editing ? 'Route updated' : 'Route created'}
			</div>
			<h2 class="mt-1 text-2xl font-semibold">{created.title}</h2>
			<p class="mt-1 text-sm text-byl-muted">
				{created.status !== 'published'
					? 'Saved as a draft that only you can open. The path needs review before publication or sharing.'
					: created.visibility === 'private'
						? 'Only you can open it.'
						: 'Anyone with the link can open it; it is not listed publicly.'} Distances appear once the
				route has been computed.
			</p>
		</div>
		{#if created.status === 'published'}<ShareLink
				url={shareURL(origin, created.slug)}
				title={created.title}
			/>{/if}
		<div class="flex gap-2">
			<a
				href="/tours/{created.slug}"
				class="rounded-lg border border-byl-border px-3 py-2 text-sm hover:bg-byl-surface"
				>Open the route</a
			>
			<button
				class="rounded-lg border border-byl-border px-3 py-2 text-sm hover:bg-byl-surface"
				onclick={ondone}>Done</button
			>
		</div>
	</div>
{:else if !builder}
	<LoadState
		status={city.status === 'error' ? 'error' : 'loading'}
		message={city.message}
		onretry={() => city.load()}
	/>
{:else}
	<form
		class="grid gap-6 lg:grid-cols-2"
		onsubmit={(e) => {
			e.preventDefault();
			submit();
		}}
	>
		<div class="space-y-4">
			<label class="block text-sm">
				<span class="text-byl-muted">Name</span>
				<input class={field} bind:value={builder.title} placeholder="e.g. Depot to the river" />
			</label>
			<label class="block text-sm">
				<span class="text-byl-muted">Summary</span>
				<textarea
					class={field}
					rows="3"
					bind:value={builder.summary}
					placeholder="Optional: what ties these places together?"></textarea>
			</label>
			<div class="space-y-2 text-sm">
				<div class="text-xs font-bold tracking-normal text-byl-action">Travel</div>
				<div class="flex gap-2">
					<button
						type="button"
						class={chip(builder.walk)}
						onclick={() => (builder!.walk = !builder!.walk)}>Walking</button
					>
					<button
						type="button"
						class={chip(builder.bike)}
						onclick={() => (builder!.bike = !builder!.bike)}>Biking</button
					>
				</div>
				{#if builder.modes.length === 0}<p class="text-xs text-byl-danger">
						Pick at least one way to travel.
					</p>{/if}
				<div class="pt-2 text-xs font-bold tracking-normal text-byl-action">
					Who can open it after publication
				</div>
				<div class="flex gap-2">
					<button
						type="button"
						class={chip(builder.visibility === 'unlisted')}
						onclick={() => (builder!.visibility = 'unlisted')}>Anyone with the link</button
					>
					<button
						type="button"
						class={chip(builder.visibility === 'private')}
						onclick={() => (builder!.visibility = 'private')}>Only me</button
					>
				</div>
			</div>

			<section class="space-y-2">
				<div class="flex items-baseline justify-between">
					<h2 class="font-semibold">Stops</h2>
					<span class="text-xs text-byl-muted">{builder.stops.length} of {MAX_STOPS}</span>
				</div>
				{#if builder.stops.length === 0}
					<LoadState
						status="empty"
						emptyTitle="No stops yet"
						emptyMessage="Add places from the list; the first is where the route starts."
					/>
				{/if}
				<ol class="space-y-2" data-testid="stops">
					{#each builder.stops as place, i (place.id)}
						<li
							class="flex items-center gap-2 rounded-lg border border-byl-border bg-byl-surface px-3 py-2 text-sm"
						>
							<span
								class="flex h-7 w-7 items-center justify-center rounded bg-byl-action text-xs font-bold text-byl-action-text"
								>{i + 1}</span
							>
							<span class="min-w-0 flex-1 truncate font-semibold">{place.name}</span>
							<button
								type="button"
								class="px-2 text-byl-muted hover:text-byl-text disabled:opacity-30"
								aria-label="Move {place.name} up"
								disabled={i === 0}
								onclick={() => builder!.move(i, i - 1)}>↑</button
							>
							<button
								type="button"
								class="px-2 text-byl-muted hover:text-byl-text disabled:opacity-30"
								aria-label="Move {place.name} down"
								disabled={i === builder.stops.length - 1}
								onclick={() => builder!.move(i, i + 1)}>↓</button
							>
							<button
								type="button"
								class="px-2 text-byl-danger hover:text-byl-danger"
								aria-label="Remove {place.name}"
								onclick={() => builder!.remove(place.id)}>✕</button
							>
						</li>
					{/each}
				</ol>
				<!-- Review Focus 5: the API's 422 field message sits next to the stops list. -->
				{#if builder.stopsError}
					<p class="text-sm text-byl-danger" data-testid="stops-error">{builder.stopsError}</p>
				{:else if builder.stopsHint}
					<p class="text-xs text-byl-muted">{builder.stopsHint}</p>
				{/if}
			</section>

			{#if builder.error && !builder.stopsError}<p class="text-sm text-byl-danger">
					{builder.error}
				</p>{/if}
			<button
				type="submit"
				class="w-full rounded-lg bg-byl-action px-4 py-3 text-sm font-semibold text-byl-action-text disabled:opacity-40"
				disabled={!builder.canSubmit}
			>
				{builder.busy ? 'Saving…' : editing ? 'Save changes' : 'Create route'}
			</button>
		</div>

		<section class="space-y-3">
			<h2 class="font-semibold">Places in {builder.city.name}</h2>
			<input
				class={field}
				bind:value={search}
				placeholder="Search by name or street"
				aria-label="Search places"
			/>
			<LoadState
				status={places.status === 'ready' && filtered.length === 0 ? 'empty' : places.status}
				message={places.message}
				emptyTitle="No places match"
				onretry={() => places.run()}
			>
				<ul class="space-y-2">
					{#each filtered as place (place.id)}
						{@const added = builder.contains(place)}
						<li
							class="flex items-center gap-3 rounded-lg border border-byl-border bg-byl-surface px-3 py-2 text-sm"
						>
							<span class="min-w-0 flex-1">
								<span class="block truncate font-semibold">{place.name}</span>
								{#if place.address}<span class="block truncate text-xs text-byl-muted"
										>{place.address}</span
									>{/if}
							</span>
							<button
								type="button"
								class="rounded px-3 py-1 text-xs font-semibold {added
									? 'bg-byl-success-surface text-byl-success'
									: 'bg-byl-action text-byl-action-text'} disabled:opacity-40"
								aria-label={added ? `Remove ${place.name}` : `Add ${place.name}`}
								disabled={!added && builder.stops.length >= MAX_STOPS}
								onclick={() => (added ? builder!.remove(place.id) : builder!.add(place))}
								>{added ? 'Added' : 'Add'}</button
							>
						</li>
					{/each}
				</ul>
			</LoadState>
		</section>
	</form>
{/if}
