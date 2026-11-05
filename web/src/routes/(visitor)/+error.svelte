<script lang="ts">
	import { page } from '$app/state';
</script>

<svelte:head><title>Unable to open this page · Invisible Museum</title></svelte:head>

<section class="mx-auto max-w-xl space-y-4 px-5 py-12 text-white">
	<h1 class="text-2xl font-semibold">Unable to open this page</h1>
	<p class="text-white/60">{page.error?.message ?? 'Please try again.'}</p>
	<a href="/" class="inline-block rounded-xl border border-white/20 px-4 py-2">Back to tours</a>
</section>
