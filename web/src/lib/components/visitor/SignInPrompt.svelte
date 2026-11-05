<script lang="ts">
	import { page } from '$app/state';

	let { reason }: { reason: string } = $props();
</script>

<div class="rounded-lg border border-byl-border bg-byl-surface p-5">
	<div class="text-xs font-bold tracking-normal text-byl-action">Sign in</div>
	<p class="mt-1 text-sm">{reason}</p>
	<a
		href="/signin?next={encodeURIComponent(page.url.pathname + page.url.search)}"
		class="mt-3 inline-block rounded-lg bg-byl-action px-4 py-2 text-sm font-semibold text-byl-action-text"
		>Sign in</a
	>
</div>
