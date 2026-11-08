<script lang="ts">
	import { page } from '$app/state';
	import { session } from '#lib/auth.ts';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import RouteReportStatus from '#lib/components/visitor/RouteReportStatus.svelte';
	const current = session.store;
	const id = $derived(page.params.id ?? '');
</script>

<svelte:head><title>Route report status · Byl</title></svelte:head>
<a href="/routes/reports" class="text-sm text-byl-muted underline">← Your route reports</a>
<h1 class="mt-3 text-3xl font-semibold">Route report status</h1>
{#if !$current}<div class="mt-6">
		<SignInPrompt reason="Sign in to read this account's private route report." />
	</div>{:else}{#key `${$current.account.id}:${id}`}<RouteReportStatus {id} />{/key}{/if}
