<script lang="ts">
	import type { MediaAsset } from '#lib/admin.ts';
	let {
		library,
		ids = $bindable([]),
		heroId = $bindable(''),
		chooseHero = false
	}: {
		library: MediaAsset[];
		ids: string[];
		heroId?: string;
		chooseHero?: boolean;
	} = $props();
	let pick = $state('');
	const assets = $derived([...new Map(library.map((asset) => [asset.id, asset])).values()]);
	const selected = $derived(
		ids
			.map((id) => assets.find((asset) => asset.id === id))
			.filter((asset): asset is MediaAsset => !!asset)
	);
	function remove(id: string) {
		ids = ids.filter((value) => value !== id);
		if (heroId === id) heroId = '';
	}
	function move(index: number, direction: number) {
		const next = [...ids];
		const to = index + direction;
		if (to < 0 || to >= next.length) return;
		[next[index], next[to]] = [next[to], next[index]];
		ids = next;
	}
</script>

<section aria-label="Media attachments" class="space-y-3">
	<h2 class="font-semibold">Photos, documents and video links</h2>
	<ol class="space-y-2">
		{#each selected as asset, index (asset.id)}
			<li class="flex flex-wrap items-start gap-3 border-b border-byl-border py-3 text-sm">
				{#if asset.kind === 'image'}<img
						src={asset.public_url}
						alt={asset.caption}
						class="h-20 w-24 rounded bg-byl-surface object-contain"
					/>{/if}
				<div class="min-w-0 flex-1 space-y-1">
					<p class="font-semibold break-words">{asset.caption || 'Untitled asset'}</p>
					<p class="text-xs text-byl-muted">
						{asset.kind} · {asset.external
							? 'External reference'
							: asset.license || 'Needs a reusable license'}
					</p>
					{#if chooseHero && asset.kind === 'image'}<label class="flex items-center gap-2 text-xs"
							><input
								type="radio"
								name="hero"
								checked={heroId === asset.id}
								onchange={() => (heroId = asset.id)}
							/>Featured photo</label
						>{/if}
					<div class="flex flex-wrap gap-3 text-xs">
						<button
							type="button"
							class="underline disabled:opacity-40"
							aria-label={`Move ${asset.caption} up`}
							disabled={index === 0}
							onclick={() => move(index, -1)}>Move up</button
						>
						<button
							type="button"
							class="underline disabled:opacity-40"
							aria-label={`Move ${asset.caption} down`}
							disabled={index === ids.length - 1}
							onclick={() => move(index, 1)}>Move down</button
						>
						<button
							type="button"
							class="underline"
							aria-label={`Remove ${asset.caption}`}
							onclick={() => remove(asset.id)}>Remove</button
						>
					</div>
				</div>
			</li>
		{:else}<li class="text-sm text-byl-muted">No attachments selected.</li>{/each}
	</ol>
	<div class="flex flex-wrap gap-2 text-sm">
		<select
			aria-label="Attach media from library"
			class="max-w-full min-w-0 rounded-md border-byl-border-control"
			bind:value={pick}
		>
			<option value="">Choose from the library…</option>
			{#each assets.filter((asset) => !ids.includes(asset.id)) as asset (asset.id)}<option
					value={asset.id}>{asset.kind} · {asset.caption || asset.id.slice(0, 8)}</option
				>{/each}
		</select>
		<button
			type="button"
			class="rounded-md border border-byl-border-control px-3 py-1 disabled:opacity-40"
			disabled={!pick}
			onclick={() => {
				ids = [...ids, pick];
				pick = '';
			}}>Attach</button
		>
		<a href="/admin/media" class="self-center underline">Upload a file or add a link</a>
	</div>
	<p class="text-xs text-byl-muted">Save this page to keep the attachments and their order.</p>
</section>
