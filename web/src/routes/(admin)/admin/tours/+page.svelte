<script lang="ts">
	import { goto } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import { createTour, errorMessage } from '#lib/admin.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let title = $state('');
	let topic = $state('');
	let err = $state('');

	async function add(e: SubmitEvent) {
		e.preventDefault();
		err = '';
		try {
			const t = await createTour({ city_id: data.cityId, title, topic_id: topic || null });
			goto(`/admin/tours/${t.id}`);
		} catch (e2) {
			err = errorMessage(e2, 'Could not create the tour.');
		}
	}
</script>

<div class="flex flex-wrap items-end justify-between gap-4">
	<h1 class="text-2xl font-semibold">Tours</h1>
	<label class="text-sm"
		>City
		<select
			class="ml-2 rounded-md border-stone-300"
			value={data.city}
			onchange={(e) => goto(`/admin/tours?city=${(e.currentTarget as HTMLSelectElement).value}`)}
		>
			{#each data.cities as c (c.id)}<option value={c.slug}>{c.name}</option>{/each}
		</select>
	</label>
</div>

<table class="mt-4 w-full rounded-xl border border-stone-200 bg-white text-sm">
	<thead class="text-left text-xs tracking-wide text-stone-500 uppercase"
		><tr
			><th class="px-3 py-2">Title</th><th class="px-3 py-2">Topic</th><th class="px-3 py-2"
				>Stops</th
			><th class="px-3 py-2">Routes</th><th class="px-3 py-2">Status</th></tr
		></thead
	>
	<tbody class="divide-y divide-stone-200">
		{#each data.tours as t (t.id)}
			<tr>
				<td class="px-3 py-2"
					><a class="font-medium hover:underline" href="/admin/tours/{t.id}">{t.title}</a
					>{#if t.owner_account_id}<span
							class="ml-2 rounded bg-violet-100 px-1.5 text-xs text-violet-800">user route</span
						>{/if}</td
				>
				<td class="px-3 py-2">{t.topic?.name ?? '—'}</td>
				<td class="px-3 py-2">{t.stop_count}</td>
				<td class="px-3 py-2 text-xs"
					>{#each Object.values(t.routes ?? {}) as r (r.mode)}<span class="mr-2"
							>{r.mode}: {r.status}</span
						>{/each}</td
				>
				<td class="px-3 py-2"
					><StatusPill status={t.status} /> <StatusPill status={t.visibility} /></td
				>
			</tr>
		{:else}
			<tr><td colspan="5" class="px-3 py-4 text-stone-500">No tours in this city yet.</td></tr>
		{/each}
	</tbody>
</table>

<form
	class="mt-6 flex max-w-2xl flex-wrap items-end gap-3 rounded-xl border border-stone-200 bg-white p-4 text-sm"
	onsubmit={add}
>
	<span class="w-full font-semibold">New curated tour</span>
	<label
		>Title<input
			class="mt-1 block rounded-md border-stone-300"
			bind:value={title}
			required
		/></label
	>
	<label
		>Topic<select class="mt-1 block rounded-md border-stone-300" bind:value={topic}
			><option value="">— pick later —</option>{#each data.topics as tp (tp.id)}<option
					value={tp.id}>{tp.name}</option
				>{/each}</select
		></label
	>
	<button class="rounded-md bg-stone-900 px-3 py-2 text-white" type="submit">Create draft</button>
	<div class="w-full"><Notice kind="error" message={err} /></div>
</form>
