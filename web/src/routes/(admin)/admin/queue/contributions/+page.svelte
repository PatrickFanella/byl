<script lang="ts">
	import { page } from '$app/state';
	import { session } from '#lib/auth.ts';
	import ContributionReviews from '#lib/components/ContributionReviews.svelte';
	const account = session.store;
</script>

{#key `${$account?.account.id}:${page.url.search}`}
	<ContributionReviews search={page.url.search} />
{/key}
