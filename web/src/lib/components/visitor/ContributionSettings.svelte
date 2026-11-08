<script lang="ts">
	import { onMount } from 'svelte';
	import { session } from '#lib/auth.ts';
	import { contributionClient } from '#lib/tour/contribution-api.ts';
	import { ContributionStore } from '#lib/tour/contribution-store.ts';
	import { TripContributions } from '#lib/tour/contributions.svelte.ts';
	import ContributionPanel from './ContributionPanel.svelte';
	const current = session.store;
	let contributions = $state<TripContributions | null>(null);
	onMount(() => {
		const scope = contributionClient();
		const controller = new TripContributions({
			...scope,
			store: new ContributionStore(localStorage, scope.base)
		});
		contributions = controller;
		void controller.refresh();
		const retry = () => void controller.refresh();
		window.addEventListener('online', retry);
		window.addEventListener('storage', retry);
		return () => {
			controller.lifecycle('end');
			window.removeEventListener('online', retry);
			window.removeEventListener('storage', retry);
		};
	});
</script>

{#if contributions}<ContributionPanel {contributions} signedIn={$current !== null} />{/if}
