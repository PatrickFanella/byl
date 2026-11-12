<script lang="ts">
	import { session } from '#lib/auth.ts';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import RouteReportList from '#lib/components/visitor/RouteReportList.svelte';
	const current = session.store;
</script>

<svelte:head><title>Your route reports · Byl</title></svelte:head>
<a href="/routes" class="text-sm text-byl-muted underline">← Your routes</a>
<h1 class="byl-title mt-3">Your route reports</h1>
<p class="mt-3 max-w-2xl text-byl-muted">
	Private observations and curator decisions about the routes you reported.
</p>
{#if !$current}<div class="mt-6">
		<SignInPrompt reason="Sign in to read your own route reports and curator decisions." />
	</div>{:else}{#key $current.account.id}<RouteReportList />{/key}{/if}
