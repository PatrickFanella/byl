<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import { uploadMedia, updateMedia, errorMessage } from '#lib/admin.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let notice = $state({ kind: 'info' as 'info' | 'error' | 'success', message: '' });
	let busy = $state('');

	async function upload(e: SubmitEvent) {
		e.preventDefault();
		const form = new FormData(e.currentTarget as HTMLFormElement);
		busy = 'upload';
		try {
			await uploadMedia(form);
			notice = { kind: 'success', message: 'Uploaded.' };
			(e.target as HTMLFormElement).reset();
			await invalidateAll();
		} catch (err) {
			notice = { kind: 'error', message: errorMessage(err, 'Upload failed.') };
		} finally {
			busy = '';
		}
	}

	async function saveLicence(id: string, license: string, credit: string) {
		busy = id;
		try {
			await updateMedia(id, { license, credit });
			notice = { kind: 'success', message: 'Licence saved.' };
			await invalidateAll();
		} catch (err) {
			notice = { kind: 'error', message: errorMessage(err, 'Could not save.') };
		} finally {
			busy = '';
		}
	}
</script>

<h1 class="text-2xl font-semibold">Media</h1>
<form
	class="mt-4 flex flex-wrap items-end gap-3 rounded-lg border border-byl-border bg-byl-background p-4 text-sm"
	onsubmit={upload}
	enctype="multipart/form-data"
>
	<label
		>File<input
			class="mt-1 block"
			type="file"
			name="file"
			accept="image/*,video/*,audio/*"
			required
		/></label
	>
	<label
		>Caption<input class="mt-1 block rounded-md border-byl-border-control" name="caption" /></label
	>
	<label
		>Credit<input class="mt-1 block rounded-md border-byl-border-control" name="credit" /></label
	>
	<label
		>Licence<input
			class="mt-1 block w-36 rounded-md border-byl-border-control"
			name="license"
			placeholder="CC-BY-4.0"
		/></label
	>
	<label
		>Source URL<input
			class="mt-1 block rounded-md border-byl-border-control"
			name="source_url"
		/></label
	>
	<button
		class="rounded-md bg-byl-action px-3 py-2 text-byl-action-text disabled:opacity-50"
		disabled={busy === 'upload'}
		type="submit">Upload</button
	>
	<div class="w-full"><Notice kind={notice.kind} message={notice.message} /></div>
</form>

<ul class="mt-6 grid grid-cols-2 gap-3 md:grid-cols-4">
	{#each data.media as m (m.id)}
		<li class="rounded-lg border border-byl-border bg-byl-background p-2 text-xs">
			{#if m.kind === 'image' && m.public_url}
				<img src={m.public_url} alt={m.caption} class="h-32 w-full rounded object-cover" />
			{:else if m.kind === 'audio' && m.public_url}
				<audio controls src={m.public_url} class="w-full"></audio>
			{:else}
				<div class="flex h-32 items-center justify-center rounded bg-byl-surface text-byl-muted">
					{m.kind}{m.public_url ? '' : ' (not fetched yet)'}
				</div>
			{/if}
			<div class="mt-1 truncate font-medium" title={m.caption}>{m.caption || '(no caption)'}</div>
			<div class="flex items-center justify-between">
				<StatusPill status={m.status} /><span class="text-byl-muted">{m.mime_type}</span>
			</div>
			<form
				class="mt-1 flex gap-1"
				onsubmit={(e) => {
					e.preventDefault();
					const f = new FormData(e.currentTarget as HTMLFormElement);
					saveLicence(m.id, String(f.get('license')), String(f.get('credit')));
				}}
			>
				<input
					class="w-full rounded-md border-byl-border-control px-1 py-0.5 text-xs"
					name="license"
					value={m.license}
					placeholder="licence"
				/>
				<input
					class="w-full rounded-md border-byl-border-control px-1 py-0.5 text-xs"
					name="credit"
					value={m.credit}
					placeholder="credit"
				/>
				<button
					class="rounded-md border border-byl-border-control px-1"
					disabled={busy === m.id}
					type="submit">Save</button
				>
			</form>
		</li>
	{:else}
		<li class="col-span-full text-sm text-byl-muted">No media yet.</li>
	{/each}
</ul>
