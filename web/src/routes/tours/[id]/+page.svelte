<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import Dialog from '#lib/components/Dialog.svelte';
	import Map from '#lib/components/Map.svelte';
	import {
		updateTour,
		setTourStops,
		setTourStatus,
		recomputeRoutes,
		deleteTour,
		suggestFeatured,
		errorMessage,
		type MomentSummary
	} from '#lib/admin.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let tour = $derived(data.tour);
	let title = $derived(tour.title);
	let summary = $derived(tour.summary);
	let topic = $derived(tour.topic_id ?? '');
	let visibility = $derived(tour.visibility);
	let walk = $derived(tour.modes.includes('walk'));
	let bike = $derived(tour.modes.includes('bike'));
	type StopDraft = {
		place_id: string;
		place_name: string;
		featured_moment_id: string;
		note: string;
	};
	let stops: StopDraft[] = $derived(
		tour.stops.map((s) => ({
			place_id: s.place.id,
			place_name: s.place.name,
			featured_moment_id: s.featured_moment_id ?? '',
			note: s.note
		}))
	);
	let addPlace = $state('');
	let notice = $state({ kind: 'info' as 'info' | 'error' | 'success', message: '' });
	let busy = $state('');
	let suggestOpen = $state(false);
	let suggestFor = $state(-1);
	let suggestions: { moment: MomentSummary; reason: string }[] = $state([]);

	const locked = $derived(tour.status === 'published' && !tour.owner_account_id);
	const markers = $derived(
		tour.stops.map((s) => ({
			lat: s.place.latitude,
			lng: s.place.longitude,
			label: `${s.position}. ${s.place.name}`
		}))
	);
	const line = $derived((data.route?.geometry?.coordinates ?? []) as [number, number][]);

	function move(i: number, d: number) {
		const j = i + d;
		if (j < 0 || j >= stops.length) return;
		const copy = [...stops];
		[copy[i], copy[j]] = [copy[j], copy[i]];
		stops = copy;
	}

	function addStop() {
		const p = data.places.find((x) => x.id === addPlace);
		if (!p) return;
		stops = [...stops, { place_id: p.id, place_name: p.name, featured_moment_id: '', note: '' }];
		addPlace = '';
	}

	async function run(label: string, fn: () => Promise<unknown>, ok: string) {
		busy = label;
		try {
			await fn();
			notice = { kind: 'success', message: ok };
			await invalidateAll();
		} catch (e) {
			notice = { kind: 'error', message: errorMessage(e, `${label} failed.`) };
		} finally {
			busy = '';
		}
	}

	const saveDetails = () =>
		run(
			'save',
			() =>
				updateTour(tour.id, {
					title,
					summary,
					topic_id: topic,
					visibility,
					modes: [...(walk ? ['walk'] : []), ...(bike ? ['bike'] : [])]
				}),
			'Saved.'
		);
	const saveStops = () =>
		run(
			'stops',
			() =>
				setTourStops(
					tour.id,
					stops.map((s) => ({
						place_id: s.place_id,
						featured_moment_id: s.featured_moment_id || null,
						note: s.note
					}))
				),
			'Stops saved; routes are being recomputed.'
		);
	const publish = () => run('publish', () => setTourStatus(tour.id, 'publish'), 'Published.');
	const unpublish = () =>
		run('unpublish', () => setTourStatus(tour.id, 'unpublish'), 'Back to draft.');
	const recompute = () => run('recompute', () => recomputeRoutes(tour.id), 'Routes queued.');

	async function remove() {
		if (!confirm('Delete this tour? This cannot be undone.')) return;
		await deleteTour(tour.id);
		goto('/tours');
	}

	async function suggest(i: number) {
		busy = 'suggest';
		suggestFor = i;
		try {
			const r = await suggestFeatured(tour.id, i + 1);
			suggestions = r.ordered;
			suggestOpen = true;
		} catch (e) {
			notice = { kind: 'error', message: errorMessage(e, 'Suggestion failed.') };
		} finally {
			busy = '';
		}
	}

	function choose(id: string) {
		if (suggestFor >= 0) stops[suggestFor].featured_moment_id = id;
		suggestOpen = false;
	}
</script>

<div class="flex flex-wrap items-center gap-3">
	<a href="/tours" class="text-sm text-stone-500 hover:underline">← Tours</a>
	<h1 class="text-2xl font-semibold">{tour.title}</h1>
	<StatusPill status={tour.status} />
	<StatusPill status={tour.visibility} />
	{#if tour.owner_account_id}<span class="rounded bg-violet-100 px-1.5 text-xs text-violet-800"
			>user route</span
		>{/if}
	<span class="text-xs text-stone-500">/tours/{tour.slug}</span>
</div>
<div class="mt-3"><Notice kind={notice.kind} message={notice.message} /></div>
{#if locked}
	<p class="mt-2 rounded-md bg-amber-50 p-2 text-sm text-amber-900">
		This tour is published. Unpublish it to change stops, modes, or routes; title and summary can
		still be edited.
	</p>
{/if}

<div class="mt-4 grid gap-6 lg:grid-cols-3">
	<div class="space-y-6 lg:col-span-2">
		<section
			class="grid gap-3 rounded-xl border border-stone-200 bg-white p-4 text-sm md:grid-cols-2"
		>
			<label class="md:col-span-2"
				>Title<input class="mt-1 w-full rounded-md border-stone-300" bind:value={title} /></label
			>
			<label class="md:col-span-2"
				>Summary<textarea
					class="mt-1 w-full rounded-md border-stone-300"
					rows="3"
					bind:value={summary}></textarea></label
			>
			<label
				>Topic<select class="mt-1 w-full rounded-md border-stone-300" bind:value={topic}
					><option value="">— none —</option>{#each data.topics as tp (tp.id)}<option value={tp.id}
							>{tp.name}</option
						>{/each}</select
				></label
			>
			<label
				>Visibility<select class="mt-1 w-full rounded-md border-stone-300" bind:value={visibility}
					><option value="public">public</option><option value="unlisted">unlisted</option><option
						value="private">private</option
					></select
				></label
			>
			<div class="flex gap-4 md:col-span-2">
				<label class="flex items-center gap-1"
					><input type="checkbox" bind:checked={walk} disabled={locked} />Walk</label
				>
				<label class="flex items-center gap-1"
					><input type="checkbox" bind:checked={bike} disabled={locked} />Bike</label
				>
			</div>
			<div class="md:col-span-2">
				<button
					class="rounded-md bg-stone-900 px-3 py-1.5 text-white disabled:opacity-50"
					disabled={!!busy}
					onclick={saveDetails}>Save details</button
				>
			</div>
		</section>

		<section class="rounded-xl border border-stone-200 bg-white p-4 text-sm">
			<h2 class="font-semibold">Stops</h2>
			<ol class="mt-2 space-y-3">
				{#each stops as s, i (s.place_id + i)}
					<li class="rounded-md border border-stone-200 p-3">
						<div class="flex items-center gap-2">
							<span class="w-6 text-right font-mono text-xs text-stone-500">{i + 1}.</span>
							<a class="font-medium hover:underline" href="/places/{s.place_id}">{s.place_name}</a>
							<span class="ml-auto flex gap-1 text-xs">
								<button
									class="rounded border px-1"
									disabled={locked || i === 0}
									onclick={() => move(i, -1)}>↑</button
								>
								<button
									class="rounded border px-1"
									disabled={locked || i === stops.length - 1}
									onclick={() => move(i, 1)}>↓</button
								>
								<button
									class="rounded border px-1 text-red-700"
									disabled={locked}
									onclick={() => (stops = stops.filter((_, j) => j !== i))}>✕</button
								>
							</span>
						</div>
						<div class="mt-2 grid gap-2 md:grid-cols-2">
							<label class="text-xs"
								>Featured moment
								<div class="flex gap-1">
									<select
										class="mt-1 w-full rounded-md border-stone-300 text-sm"
										bind:value={s.featured_moment_id}
										disabled={locked}
									>
										<option value="">place's primary moment</option>
										{#each data.momentsByPlace[s.place_id] ?? [] as m (m.id)}<option value={m.id}
												>{m.title}</option
											>{/each}
									</select>
									<button
										class="mt-1 rounded-md border border-indigo-300 bg-indigo-50 px-2 text-xs text-indigo-900 disabled:opacity-50"
										disabled={locked || !!busy}
										onclick={() => suggest(i)}
										title="Rank this place's moments against the tour topic">Suggest</button
									>
								</div>
							</label>
							<label class="text-xs"
								>Curator note<input
									class="mt-1 w-full rounded-md border-stone-300 text-sm"
									bind:value={s.note}
									disabled={locked}
								/></label
							>
						</div>
					</li>
				{:else}
					<li class="text-stone-500">No stops yet.</li>
				{/each}
			</ol>
			<div class="mt-3 flex flex-wrap gap-2">
				<select class="rounded-md border-stone-300" bind:value={addPlace} disabled={locked}>
					<option value="">Add a published place…</option>
					{#each data.places as p (p.id)}<option value={p.id}>{p.name}</option>{/each}
				</select>
				<button
					class="rounded-md border border-stone-300 px-2"
					disabled={locked || !addPlace}
					onclick={addStop}>Add stop</button
				>
				<button
					class="rounded-md bg-stone-900 px-3 py-1.5 text-white disabled:opacity-50"
					disabled={locked || !!busy}
					onclick={saveStops}>Save stops</button
				>
			</div>
		</section>
	</div>

	<aside class="space-y-3 text-sm">
		<Map {markers} {line} height="22rem" />
		<div class="rounded-xl border border-stone-200 bg-white p-4">
			<h2 class="font-semibold">Routes</h2>
			<ul class="mt-1">
				{#each Object.values(tour.routes ?? {}) as r (r.mode)}
					<li class="flex items-center gap-2">
						<span class="w-10">{r.mode}</span><StatusPill
							status={r.status}
						/>{#if r.estimated_minutes}<span class="text-xs text-stone-500"
								>{r.estimated_minutes} min · {Math.round(r.distance_m / 100) / 10} km</span
							>{/if}{#if r.error}<span class="truncate text-xs text-red-700" title={r.error}
								>{r.error}</span
							>{/if}
					</li>
				{/each}
			</ul>
			<button
				class="mt-2 text-xs underline disabled:opacity-50"
				disabled={locked || !!busy}
				onclick={recompute}>Recompute routes</button
			>
		</div>
		<div class="flex flex-wrap gap-2">
			{#if tour.status === 'published'}
				<button
					class="rounded-md border border-stone-300 px-3 py-1.5 disabled:opacity-50"
					disabled={!!busy}
					onclick={unpublish}>Unpublish</button
				>
			{:else}
				<button
					class="rounded-md bg-emerald-700 px-3 py-1.5 text-white disabled:opacity-50"
					disabled={!!busy}
					onclick={publish}>Publish</button
				>
			{/if}
			<button
				class="rounded-md border border-red-300 px-3 py-1.5 text-red-700"
				disabled={!!busy}
				onclick={remove}>Delete</button
			>
		</div>
	</aside>
</div>

<Dialog bind:open={suggestOpen} title="Suggested featured moments">
	<ol class="space-y-2 text-sm">
		{#each suggestions as s, i (s.moment.id)}
			<li class="rounded-md border border-stone-200 p-2">
				<div class="flex items-center gap-2">
					<span class="font-mono text-xs text-stone-500">{i + 1}.</span><span class="font-medium"
						>{s.moment.title}</span
					><span class="text-xs text-stone-500">{s.moment.date_display}</span><button
						class="ml-auto rounded-md bg-stone-900 px-2 py-0.5 text-xs text-white"
						onclick={() => choose(s.moment.id)}>Use</button
					>
				</div>
				<p class="mt-1 text-xs text-stone-600">{s.reason}</p>
			</li>
		{:else}
			<li class="text-stone-500">No published moments at this place.</li>
		{/each}
	</ol>
</Dialog>
