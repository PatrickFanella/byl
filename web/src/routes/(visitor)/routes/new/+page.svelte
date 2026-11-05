<script lang="ts">
	import { goto } from '$app/navigation';
	import RouteBuilderForm from '#lib/components/visitor/RouteBuilderForm.svelte';
</script>

<svelte:head><title>New route · Invisible Museum</title></svelte:head>

<a href="/routes" class="text-sm text-white/50 hover:underline">← Your routes</a>
<h1 class="mt-2 text-3xl font-semibold">New route</h1>
<div class="mt-5"><RouteBuilderForm editing={null} ondone={() => goto('/routes')} /></div>
