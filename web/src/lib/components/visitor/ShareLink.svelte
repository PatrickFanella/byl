<script lang="ts">
	let { url, title }: { url: string; title: string } = $props();
	let copied = $state(false);
	const canShare = typeof navigator !== 'undefined' && typeof navigator.share === 'function';

	async function copy() {
		try {
			await navigator.clipboard.writeText(url);
			copied = true;
			setTimeout(() => (copied = false), 2000);
		} catch {
			// clipboard blocked: the field below is selectable
		}
	}

	async function share() {
		try {
			await navigator.share({ title, url });
		} catch {
			// cancelled
		}
	}
</script>

<div class="space-y-2">
	<input
		class="w-full rounded-xl border border-white/20 bg-black/40 px-3 py-2 font-mono text-xs text-white"
		readonly
		value={url}
		aria-label="Share link"
		onfocus={(e) => e.currentTarget.select()}
	/>
	<div class="flex gap-2">
		<button class="rounded-xl bg-white px-3 py-2 text-sm font-semibold text-black" onclick={copy}
			>{copied ? 'Copied' : 'Copy link'}</button
		>
		{#if canShare}
			<button
				class="rounded-xl border border-white/20 px-3 py-2 text-sm hover:bg-white/10"
				onclick={share}>Share…</button
			>
		{/if}
	</div>
</div>
