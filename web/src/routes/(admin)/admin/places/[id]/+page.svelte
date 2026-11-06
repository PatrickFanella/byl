<script lang="ts">
	import { goto, invalidateAll } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import PayloadForm from '#lib/components/PayloadForm.svelte';
	import MediaAttachments from '#lib/components/MediaAttachments.svelte';
	import Map from '#lib/components/Map.svelte';
	import { fieldsFor, applyFields, type Field } from '#lib/payload.ts';
	import {
		updatePlace,
		setPlaceStatus,
		setPlaceMedia,
		createMoment,
		errorMessage
	} from '#lib/admin.ts';
	import { ApiError } from '#lib/api/index.ts';
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
	let place = $derived(data.place);
	let fields: Field[] = $derived(
		fieldsFor('place', place as unknown as Record<string, unknown>).filter((f) => f.key !== 'slug')
	);
	let errors: Record<string, string> = $state({});
	let notice = $state({ kind: 'info' as 'info' | 'error' | 'success', message: '' });
	let busy = $state('');
	let primary = $derived(place.primary_moment_id ?? '');
	let mediaIds = $derived(place.media.map((asset) => asset.id));

	async function save() {
		busy = 'save';
		errors = {};
		try {
			const body = applyFields({}, fields) as Record<string, unknown>;
			if (primary !== (place.primary_moment_id ?? '')) body.primary_moment_id = primary;
			await updatePlace(place.id, body);
			await setPlaceMedia(place.id, mediaIds);
			notice = { kind: 'success', message: 'Saved.' };
			await invalidateAll();
		} catch (e) {
			if (e instanceof ApiError) errors = e.fields ?? {};
			notice = { kind: 'error', message: errorMessage(e, 'Save failed.') };
		} finally {
			busy = '';
		}
	}

	async function status(action: 'publish' | 'unpublish') {
		busy = action;
		try {
			await setPlaceStatus(place.id, action);
			notice = { kind: 'success', message: action === 'publish' ? 'Published.' : 'Back to draft.' };
			await invalidateAll();
		} catch (e) {
			notice = { kind: 'error', message: errorMessage(e, 'Status change failed.') };
		} finally {
			busy = '';
		}
	}

	async function newMoment() {
		busy = 'new';
		try {
			const m = await createMoment({
				place_id: place.id,
				title: 'Untitled moment',
				date_precision: 'circa'
			});
			goto(`/admin/moments/${m.id}`);
		} catch (e) {
			notice = { kind: 'error', message: errorMessage(e, 'Could not create a moment.') };
		} finally {
			busy = '';
		}
	}
</script>

<div class="flex flex-wrap items-center gap-3">
	<a href="/admin/places" class="text-sm text-byl-muted hover:underline">← Places</a>
	<h1 class="text-2xl font-semibold">{place.name}</h1>
	<StatusPill status={place.status} />
	<span class="text-xs text-byl-muted">{place.slug}</span>
</div>
<div class="mt-3"><Notice kind={notice.kind} message={notice.message} /></div>

<div class="mt-4 grid gap-6 lg:grid-cols-3">
	<div class="space-y-4 lg:col-span-2">
		<PayloadForm bind:fields {errors} />
		<MediaAttachments library={[...data.library, ...place.media]} bind:ids={mediaIds} />
		<label class="block text-sm">
			<span class="text-byl-text">Primary moment (featured on user routes)</span>
			<select class="mt-1 w-full rounded-md border-byl-border-control" bind:value={primary}>
				<option value="">— none —</option>
				{#each data.moments as m (m.id)}<option value={m.id}>{m.title} ({m.status})</option>{/each}
			</select>
		</label>
		<div class="flex flex-wrap gap-2">
			<button
				class="rounded-md bg-byl-action px-3 py-1.5 text-sm text-byl-action-text disabled:opacity-50"
				disabled={!!busy}
				onclick={save}>Save</button
			>
			{#if place.status === 'published'}
				<button
					class="rounded-md border border-byl-border-control px-3 py-1.5 text-sm disabled:opacity-50"
					disabled={!!busy}
					onclick={() => status('unpublish')}>Unpublish</button
				>
			{:else}
				<button
					class="rounded-md bg-byl-success-surface px-3 py-1.5 text-sm text-byl-action-text disabled:opacity-50"
					disabled={!!busy}
					onclick={() => status('publish')}>Publish</button
				>
			{/if}
		</div>

		<section>
			<div class="flex items-center justify-between">
				<h2 class="font-semibold">Moments</h2>
				<button class="text-sm underline" disabled={!!busy} onclick={newMoment}>New moment</button>
			</div>
			<ul
				class="mt-2 divide-y divide-byl-border rounded-lg border border-byl-border bg-byl-background text-sm"
			>
				{#each data.moments as m (m.id)}
					<li class="flex items-center gap-3 px-4 py-2">
						<a class="font-medium hover:underline" href="/admin/moments/{m.id}">{m.title}</a>
						<span class="text-xs text-byl-muted">{m.date_display}</span>
						{#if m.id === place.primary_moment_id}<span
								class="rounded bg-byl-action px-1.5 text-xs text-byl-action-text">primary</span
							>{/if}
						<span class="ml-auto"><StatusPill status={m.status} /></span>
					</li>
				{:else}
					<li class="px-4 py-3 text-byl-muted">No moments yet. Approve a candidate or add one.</li>
				{/each}
			</ul>
		</section>
	</div>
	<aside>
		<Map
			markers={[{ lat: place.latitude, lng: place.longitude, label: place.name }]}
			height="22rem"
		/>
		<p class="mt-2 text-xs text-byl-muted">Arrival radius {place.arrival_radius_m} m.</p>
	</aside>
</div>
