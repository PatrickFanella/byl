<script lang="ts">
	import { page } from '$app/state';
	import { session } from '#lib/auth.ts';
	import RouteReportReview from '#lib/components/RouteReportReview.svelte';
	const account = session.store;
</script>

{#key `${$account?.account.id}:${page.params.id}`}
	<RouteReportReview id={page.params.id ?? ''} />
{/key}
