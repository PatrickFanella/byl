<script lang="ts">
	import { goto } from '$app/navigation';
	import RouteBriefForm from '#lib/components/RouteBriefForm.svelte';
</script>

<a href="/admin/briefs" class="text-sm underline">Route briefs</a>
<h1 class="mt-4 text-2xl font-semibold">New route brief</h1>
<p class="mt-2 max-w-2xl text-sm text-byl-muted">
	A topic and area are enough to save. Add what you know; research and review can resolve the gaps
	later.
</p>
<RouteBriefForm onsaved={(brief) => goto(`/admin/briefs/${brief.id}?saved=1`)} />
