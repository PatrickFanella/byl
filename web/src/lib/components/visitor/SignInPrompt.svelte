<script lang="ts">
	import { page } from '$app/state';

	let { reason }: { reason: string } = $props();
</script>

<div class="byl-panel">
	<div class="byl-eyebrow">Sign in</div>
	<p class="mt-1 text-sm">{reason}</p>
	<a
		href="/signin?next={encodeURIComponent(page.url.pathname + page.url.search)}"
		class="byl-btn byl-btn-primary mt-3">Sign in</a
	>
</div>
