<script lang="ts">
	import { session } from '#lib/auth.ts';
	const current = session.store;
</script>

<h1 class="text-2xl font-semibold">Dashboard</h1>
{#if $current}
	<p class="mt-2 text-stone-600">
		Signed in as {$current.account.email}. The dashboard fills in with the review queue work.
	</p>
{/if}
