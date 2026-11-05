<script lang="ts">
	import { goto } from '$app/navigation';
	import RouteBuilderForm from '#lib/components/visitor/RouteBuilderForm.svelte';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import { session } from '#lib/auth.ts';

	const current = session.store;
</script>

<svelte:head><title>New route · Byl</title></svelte:head>

<a href="/routes" class="text-sm text-byl-muted hover:underline">← Your routes</a>
<h1 class="mt-2 text-3xl font-semibold">New route</h1>
<div class="mt-5">
	{#if !$current}
		<SignInPrompt reason="Sign in to build routes that follow you to every device." />
	{:else}
		<RouteBuilderForm editing={null} ondone={() => goto('/routes')} />
	{/if}
</div>
