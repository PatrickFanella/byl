<script lang="ts">
	import { page } from '$app/state';
	import { session } from '#lib/auth.ts';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import RouteSuggestionStatus from '#lib/components/visitor/RouteSuggestionStatus.svelte';
	const current = session.store;
</script>

<svelte:head><title>Suggestion status · Byl</title></svelte:head>
<a href="/routes/suggestions" class="text-sm text-byl-muted hover:underline">← Tour suggestions</a>
<h1 class="mt-2 text-3xl font-semibold">Suggestion status</h1>
{#if !$current}
	<div class="mt-6"><SignInPrompt reason="Sign in to read your private suggestion status." /></div>
{:else}
	{#key `${$current.account.id}/${page.params.id}`}<RouteSuggestionStatus
			id={page.params.id!}
		/>{/key}
{/if}
