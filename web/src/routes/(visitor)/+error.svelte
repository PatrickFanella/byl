<script lang="ts">
	import { page } from '$app/state';
</script>

<svelte:head><title>Unable to open this page · Byl</title></svelte:head>

<section class="max-w-[66ch] space-y-4 py-8 text-byl-text">
	<h1 class="byl-title">Unable to open this page</h1>
	<p class="byl-lede">{page.error?.message ?? 'Please try again.'}</p>
	<a href="/" class="byl-btn">Back to tours</a>
</section>
