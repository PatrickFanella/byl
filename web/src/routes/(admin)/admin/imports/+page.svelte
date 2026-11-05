<script lang="ts">
	import { untrack } from 'svelte';
	import { invalidateAll } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import { createImport } from '#lib/admin.ts';
	import { ApiError } from '#lib/api/index.ts';
	import { when } from '#lib/format.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let source = $state(untrack(() => data.sources[0]?.name ?? 'wikipedia'));
	let city = $state(untrack(() => data.cities[0]?.slug ?? ''));
	let place = $state('');
	let near = $state('');
	let radius = $state('2000');
	let query = $state('');
	let limit = $state('25');
	let busy = $state(false);
	let notice = $state({ kind: 'info' as 'info' | 'error' | 'success', message: '' });
	let errors: Record<string, string> = $state({});

	const selected = $derived(data.sources.find((s) => s.name === source));

	async function submit(e: SubmitEvent) {
		e.preventDefault();
		busy = true;
		errors = {};
		try {
			const body: Record<string, unknown> = { source, city };
			if (place) body.place = place;
			if (near) body.near = near;
			if (radius) body.radius_m = Number(radius);
			if (query) body.query = query;
			if (limit) body.limit = Number(limit);
			const { job } = await createImport(body);
			notice = {
				kind: 'success',
				message: `Import queued (job ${job.id.slice(0, 8)}). Results appear in the review queue as the worker runs.`
			};
			await invalidateAll();
		} catch (err) {
			if (err instanceof ApiError) {
				errors = err.fields ?? {};
				notice = { kind: 'error', message: err.message };
			} else {
				notice = { kind: 'error', message: 'Could not reach the API.' };
			}
		} finally {
			busy = false;
		}
	}
</script>

<h1 class="text-2xl font-semibold">Imports</h1>
<p class="mt-1 text-sm text-stone-600">
	Pull candidates from an archive into the review queue. Nothing is published automatically.
</p>

<form
	class="mt-4 grid max-w-2xl gap-4 rounded-xl border border-stone-200 bg-white p-4 text-sm md:grid-cols-2"
	onsubmit={submit}
>
	<label class="block"
		>Source
		<select class="mt-1 w-full rounded-md border-stone-300" bind:value={source}>
			{#each data.sources as s (s.name)}<option value={s.name}>{s.name}</option>{/each}
		</select>
		{#if selected}<span class="mt-1 block text-xs text-stone-500">{selected.description}</span>{/if}
		{#if errors.source}<span class="text-xs text-red-700">{errors.source}</span>{/if}
	</label>
	<label class="block"
		>City
		<select class="mt-1 w-full rounded-md border-stone-300" bind:value={city}>
			{#each data.cities as c (c.id)}<option value={c.slug}>{c.name}</option>{/each}
		</select>
	</label>
	{#if selected?.supports_near}
		<label class="block"
			>Place (id or slug, optional)<input
				class="mt-1 w-full rounded-md border-stone-300"
				bind:value={place}
				placeholder="tivoli-theatre"
			/></label
		>
		<label class="block"
			>Or a point (lat,lng)<input
				class="mt-1 w-full rounded-md border-stone-300"
				bind:value={near}
				placeholder="41.7959,-88.0092"
			/>{#if errors.near}<span class="text-xs text-red-700">{errors.near}</span>{/if}</label
		>
		<label class="block"
			>Radius (m)<input
				class="mt-1 w-full rounded-md border-stone-300"
				type="number"
				bind:value={radius}
			/>{#if errors.radius_m}<span class="text-xs text-red-700">{errors.radius_m}</span>{/if}</label
		>
	{/if}
	{#if selected?.supports_query}
		<label class="block md:col-span-2"
			>Query<input
				class="mt-1 w-full rounded-md border-stone-300"
				bind:value={query}
				placeholder="downers grove"
			/></label
		>
	{/if}
	<label class="block"
		>Limit<input
			class="mt-1 w-full rounded-md border-stone-300"
			type="number"
			bind:value={limit}
		/></label
	>
	<div class="md:col-span-2">
		<button
			class="rounded-md bg-stone-900 px-4 py-2 text-white disabled:opacity-50"
			disabled={busy}
			type="submit">{busy ? 'Queuing…' : 'Queue import'}</button
		>
	</div>
	<div class="md:col-span-2"><Notice kind={notice.kind} message={notice.message} /></div>
</form>

<h2 class="mt-8 font-semibold">Recent imports</h2>
<ul class="mt-2 divide-y divide-stone-200 rounded-xl border border-stone-200 bg-white text-sm">
	{#each data.jobs as j (j.id)}
		<li class="flex items-center gap-3 px-4 py-2">
			<StatusPill status={j.status} />
			<span class="font-mono text-xs"
				>{(j.payload as { source?: string }).source} · {(j.payload as { query?: string }).query ??
					''}</span
			>
			<span class="ml-auto text-xs text-stone-500">{when(j.created_at)}</span>
			<span
				class="max-w-xs truncate text-xs {j.status === 'failed'
					? 'text-red-700'
					: 'text-stone-600'}">{j.last_error}</span
			>
		</li>
	{:else}
		<li class="px-4 py-3 text-stone-500">No imports yet.</li>
	{/each}
</ul>
