<script lang="ts">
	import { invalidateAll } from '$app/navigation';
	import StatusPill from '#lib/components/StatusPill.svelte';
	import Notice from '#lib/components/Notice.svelte';
	import { uploadMedia, createMediaLink, updateMedia, errorMessage } from '#lib/admin.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let notice = $state({ kind: 'info' as 'info' | 'error' | 'success', message: '' });
	let busy = $state('');
	const fields = [
		{ key: 'caption', label: 'Title or caption' },
		{ key: 'credit', label: 'Creator or publisher' },
		{ key: 'source_url', label: 'Original source URL' },
		{ key: 'license', label: 'License' },
		{ key: 'license_url', label: 'License URL' }
	] as const;
	async function submit(event: SubmitEvent, action: 'upload' | 'link') {
		event.preventDefault();
		const form = event.currentTarget as HTMLFormElement;
		const values = new FormData(form);
		busy = action;
		try {
			if (action === 'upload') await uploadMedia(values);
			else
				await createMediaLink({
					kind: values.get('kind') as 'video' | 'document',
					public_url: String(values.get('public_url')),
					caption: String(values.get('caption')),
					credit: String(values.get('credit')),
					source_url: String(values.get('source_url') || values.get('public_url'))
				});
			notice = {
				kind: 'success',
				message: 'Added to the library. Attach it to a place or event from its editor.'
			};
			form.reset();
			await invalidateAll();
		} catch (error) {
			notice = { kind: 'error', message: errorMessage(error, 'Could not add this material.') };
		} finally {
			busy = '';
		}
	}
	async function save(event: SubmitEvent, id: string) {
		event.preventDefault();
		const values = new FormData(event.currentTarget as HTMLFormElement);
		busy = id;
		try {
			await updateMedia(
				id,
				Object.fromEntries(fields.map(({ key }) => [key, String(values.get(key))]))
			);
			notice = { kind: 'success', message: 'Metadata saved.' };
			await invalidateAll();
		} catch (error) {
			notice = { kind: 'error', message: errorMessage(error, 'Could not save metadata.') };
		} finally {
			busy = '';
		}
	}
</script>

<h1 class="text-2xl font-semibold">Media library</h1>
<p class="mt-2 max-w-2xl text-sm text-byl-muted">
	Collect photos and records for a place or event. Videos open on their publisher’s site.
</p>
<div class="mt-3"><Notice kind={notice.kind} message={notice.message} /></div>
<div class="mt-6 grid gap-6 lg:grid-cols-2">
	<section class="rounded-lg border border-byl-border p-4">
		<h2 class="font-semibold">Upload a file</h2>
		<p class="mt-1 text-sm text-byl-muted">
			Photos, PDF or plain-text documents, and audio. Maximum 64 MiB. Scanned records can be
			uploaded as photos.
		</p>
		<form
			class="mt-4 space-y-3 text-sm"
			enctype="multipart/form-data"
			onsubmit={(event) => submit(event, 'upload')}
		>
			<label class="block"
				>File<input
					class="mt-1 block max-w-full"
					type="file"
					name="file"
					accept="image/*,application/pdf,text/plain,audio/*"
					required
				/></label
			>
			{#each fields as field (field.key)}<label class="block"
					>{field.label}<input
						class="mt-1 block w-full rounded-md border-byl-border-control"
						name={field.key}
						type={field.key.endsWith('_url') ? 'url' : 'text'}
						placeholder={field.key === 'license' ? 'CC-BY-4.0, CC0-1.0 or PD' : ''}
					/></label
				>{/each}
			<button
				class="rounded-md bg-byl-action px-3 py-2 text-byl-action-text disabled:opacity-50"
				disabled={!!busy}
				type="submit">Upload file</button
			>
		</form>
	</section>
	<section class="rounded-lg border border-byl-border p-4">
		<h2 class="font-semibold">Add a video or document link</h2>
		<p class="mt-1 text-sm text-byl-muted">
			Use the original watch page or document URL. Link to other document formats here.
		</p>
		<form class="mt-4 space-y-3 text-sm" onsubmit={(event) => submit(event, 'link')}>
			<label class="block"
				>Type<select name="kind" class="mt-1 block w-full rounded-md border-byl-border-control"
					><option value="video">Video link</option><option value="document">Document link</option
					></select
				></label
			>
			<label class="block"
				>Destination URL<input
					type="url"
					name="public_url"
					class="mt-1 block w-full rounded-md border-byl-border-control"
					required
				/></label
			>
			<label class="block"
				>Title and relevance<input
					name="caption"
					class="mt-1 block w-full rounded-md border-byl-border-control"
					required
				/></label
			>
			<label class="block"
				>Creator or publisher<input
					name="credit"
					class="mt-1 block w-full rounded-md border-byl-border-control"
					required
				/></label
			>
			<label class="block"
				>Original source URL (if different)<input
					type="url"
					name="source_url"
					class="mt-1 block w-full rounded-md border-byl-border-control"
				/></label
			>
			<button
				class="rounded-md bg-byl-action px-3 py-2 text-byl-action-text disabled:opacity-50"
				disabled={!!busy}
				type="submit">Add link</button
			>
		</form>
	</section>
</div>

<h2 class="mt-8 text-lg font-semibold">In the library</h2>
<ul class="mt-4 grid gap-5 sm:grid-cols-2 lg:grid-cols-3">
	{#each data.media as asset (asset.id)}
		<li class="min-w-0 space-y-2 border-b border-byl-border pb-5 text-sm">
			{#if asset.kind === 'image' && asset.public_url}<img
					src={asset.public_url}
					alt={asset.caption}
					class="h-44 w-full rounded bg-byl-surface object-contain"
				/>{/if}
			<p class="font-semibold break-words">{asset.caption || 'Untitled asset'}</p>
			<div class="flex flex-wrap items-center gap-2 text-xs text-byl-muted">
				<StatusPill status={asset.status} />{asset.kind} · {asset.external
					? 'External reference'
					: asset.mime_type}
			</div>
			{#if asset.public_url}<a
					href={asset.public_url}
					target="_blank"
					rel="noreferrer"
					class="text-byl-action underline">{asset.external ? 'Open original' : 'Open file'} ↗</a
				>{/if}
			<details>
				<summary class="cursor-pointer text-sm underline">Edit metadata</summary>
				<form class="mt-3 space-y-2 text-xs" onsubmit={(event) => save(event, asset.id)}>
					{#each fields as field (field.key)}<label class="block"
							>{field.label}<input
								class="mt-1 block w-full rounded-md border-byl-border-control"
								name={field.key}
								value={asset[field.key]}
								type={field.key.endsWith('_url') ? 'url' : 'text'}
							/></label
						>{/each}
					<button
						class="rounded-md border border-byl-border-control px-3 py-1"
						disabled={!!busy}
						type="submit">Save metadata</button
					>
				</form>
			</details>
		</li>
	{:else}<li class="text-sm text-byl-muted">No media yet.</li>{/each}
</ul>
