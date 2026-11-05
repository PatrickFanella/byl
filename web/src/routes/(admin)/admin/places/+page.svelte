<script lang="ts">
	import { goto } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import { createPlace, errorMessage } from '#lib/admin.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let name = $state('');
	let lat = $state('');
	let lng = $state('');
	let err = $state('');

	async function add(e: SubmitEvent) {
		e.preventDefault();
		err = '';
		try {
			const cityId = data.cities.find((c) => c.slug === data.city)?.id;
			const p = await createPlace({
				city_id: cityId,
				name,
				latitude: Number(lat),
				longitude: Number(lng)
			});
			goto(`/admin/places/${p.id}`);
		} catch (e2) {
			err = errorMessage(e2, 'Could not create the place.');
		}
	}
</script>

<div class="flex flex-wrap items-end justify-between gap-4">
	<h1 class="text-2xl font-semibold">Places</h1>
	<label class="text-sm"
		>City
		<select
			class="ml-2 rounded-md border-stone-300"
			value={data.city}
			onchange={(e) => goto(`/admin/places?city=${(e.currentTarget as HTMLSelectElement).value}`)}
		>
			{#each data.cities as c (c.id)}<option value={c.slug}>{c.name}</option>{/each}
		</select>
	</label>
</div>

<table class="mt-4 w-full rounded-xl border border-stone-200 bg-white text-sm">
	<thead class="text-left text-xs tracking-wide text-stone-500 uppercase"
		><tr
			><th class="px-3 py-2">Name</th><th class="px-3 py-2">Kind</th><th class="px-3 py-2"
				>Moments</th
			><th class="px-3 py-2">Status</th></tr
		></thead
	>
	<tbody class="divide-y divide-stone-200">
		{#each data.places as p (p.id)}
			<tr>
				<td class="px-3 py-2"
					><a class="font-medium hover:underline" href="/admin/places/{p.id}">{p.name}</a>
					<div class="text-xs text-stone-500">{p.address}</div></td
				>
				<td class="px-3 py-2">{p.kind}</td>
				<td class="px-3 py-2">{p.moment_count}</td>
				<td class="px-3 py-2"><StatusPill status={p.status} /></td>
			</tr>
		{:else}
			<tr><td colspan="4" class="px-3 py-4 text-stone-500">No places in this city yet.</td></tr>
		{/each}
	</tbody>
</table>

<form
	class="mt-6 flex max-w-2xl flex-wrap items-end gap-3 rounded-xl border border-stone-200 bg-white p-4 text-sm"
	onsubmit={add}
>
	<span class="w-full font-semibold">New place</span>
	<label
		>Name<input class="mt-1 block rounded-md border-stone-300" bind:value={name} required /></label
	>
	<label
		>Latitude<input
			class="mt-1 block w-32 rounded-md border-stone-300"
			bind:value={lat}
			required
		/></label
	>
	<label
		>Longitude<input
			class="mt-1 block w-32 rounded-md border-stone-300"
			bind:value={lng}
			required
		/></label
	>
	<button class="rounded-md bg-stone-900 px-3 py-2 text-white" type="submit">Create draft</button>
	<div class="w-full"><Notice kind="error" message={err} /></div>
</form>
