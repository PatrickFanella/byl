<script lang="ts">
	import { session } from '#lib/auth.ts';
	import RouteReportQueue from '#lib/components/RouteReportQueue.svelte';
	const account = session.store;
</script>

{#key $account?.account.id}<RouteReportQueue />{/key}
