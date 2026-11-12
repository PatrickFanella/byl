<script lang="ts">
	import { session } from '#lib/auth.ts';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import RouteSuggestionList from '#lib/components/visitor/RouteSuggestionList.svelte';
	const current = session.store;
</script>

<svelte:head><title>Tour suggestions · Byl</title></svelte:head>
<a href="/routes" class="text-sm text-byl-muted hover:underline">← Your routes</a>
<h1 class="byl-title mt-2">Tour suggestions</h1>
<p class="mt-2 max-w-2xl text-byl-muted">
	Public tour ideas go through research and curator review. To choose existing places yourself, <a
		href="/routes/new"
		class="text-byl-action underline">plan a personal route</a
	>.
</p>
{#if !$current}
	<div class="mt-6">
		<SignInPrompt reason="Sign in to suggest a tour and read your private submission status." />
	</div>
{:else}
	{#key $current.account.id}<RouteSuggestionList ownerID={$current.account.id} />{/key}
{/if}
