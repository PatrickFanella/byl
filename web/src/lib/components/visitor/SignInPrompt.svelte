<script lang="ts">
	import { page } from '$app/state';

	let { reason }: { reason: string } = $props();
</script>

<div class="rounded-2xl border border-white/10 bg-white/5 p-5">
	<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">Sign in</div>
	<p class="mt-1 text-sm">{reason}</p>
	<a
		href="/signin?next={encodeURIComponent(page.url.pathname + page.url.search)}"
		class="mt-3 inline-block rounded-xl bg-white px-4 py-2 text-sm font-semibold text-black"
		>Sign in</a
	>
</div>
