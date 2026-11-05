<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { LoadStatus } from '#lib/loader.svelte.ts';

	let {
		status,
		message = '',
		emptyTitle = 'Nothing here yet',
		emptyMessage = '',
		loadingText = 'Loading…',
		onretry,
		children
	}: {
		status: LoadStatus;
		message?: string;
		emptyTitle?: string;
		emptyMessage?: string;
		loadingText?: string;
		onretry?: () => void;
		children?: Snippet;
	} = $props();
</script>

{#if status === 'loading'}
	<div class="flex items-center gap-3 py-5 text-sm text-white/60" role="status">
		<span class="h-4 w-4 animate-spin rounded-full border-2 border-white/30 border-t-white"></span>
		{loadingText}
	</div>
{:else if status === 'error'}
	<div class="rounded-2xl border border-red-400/40 bg-white/5 p-4">
		<div class="text-xs tracking-widest text-red-300 uppercase">Something went wrong</div>
		<p class="mt-1 text-sm">{message}</p>
		{#if onretry}
			<button
				class="mt-3 rounded-xl border border-white/20 px-3 py-1.5 text-sm hover:bg-white/10"
				onclick={onretry}>Try again</button
			>
		{/if}
	</div>
{:else if status === 'empty'}
	<div class="rounded-2xl border border-white/10 bg-white/5 p-4">
		<div class="font-semibold">{emptyTitle}</div>
		{#if emptyMessage}<p class="mt-1 text-sm text-white/60">{emptyMessage}</p>{/if}
	</div>
{:else if children}
	{@render children()}
{/if}
