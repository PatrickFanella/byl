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
	<div class="flex items-center gap-3 py-5 text-sm text-byl-muted" role="status">
		<span class="h-4 w-4 animate-spin rounded border-2 border-byl-border border-t-white"></span>
		{loadingText}
	</div>
{:else if status === 'error'}
	<div class="byl-panel byl-notice-danger" role="alert">
		<div class="font-bold">Something went wrong</div>
		<p class="mt-1 text-sm">{message}</p>
		{#if onretry}
			<button class="byl-btn mt-3" onclick={onretry}>Try again</button>
		{/if}
	</div>
{:else if status === 'empty'}
	<div class="byl-panel">
		<div class="font-bold">{emptyTitle}</div>
		{#if emptyMessage}<p class="mt-1 text-sm text-byl-muted">{emptyMessage}</p>{/if}
	</div>
{:else if children}
	{@render children()}
{/if}
