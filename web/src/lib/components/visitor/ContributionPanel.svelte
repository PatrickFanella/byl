<script lang="ts">
	import type { TripContributions } from '#lib/tour/contributions.svelte.ts';
	let {
		contributions,
		signedIn,
		trip = false
	}: { contributions: TripContributions; signedIn: boolean; trip?: boolean } = $props();
</script>

<details
	class="min-w-0 rounded-lg border border-byl-border bg-byl-background p-3 text-sm [overflow-wrap:anywhere]"
>
	<summary class="cursor-pointer font-semibold">Optional route improvement</summary>
	<p class="mt-2">
		Navigation and location permission do not opt you in. This trial sends only a route revision,
		segment, mode, direction, UTC day and qualified passage/departure counts. It does not store or
		upload your location trace.
	</p>
	<p class="mt-2 text-byl-muted">
		Pending summaries expire within seven days; retained contributions expire within 30 days.
		Closing or hiding the trip pauses observation. The detector has not been field calibrated.
	</p>
	<p class="mt-2" role="status">{contributions.message}</p>
	{#if contributions.pending}<p class="mt-2">
			{contributions.pending} pending summaries on this browser.
		</p>{/if}
	{#if trip}
		{#if !signedIn}<p class="mt-2">
				Sign in before choosing to contribute. Walking still works without an account.
			</p>
		{:else if !contributions.eligible}<p class="mt-2">
				This revision has no supported, reviewed observation reference. Contributions stay off.
			</p>
		{:else if contributions.optedIn}
			<button
				class="mt-3 rounded border border-byl-border-control px-3 py-2"
				onclick={() => contributions.togglePause()}
				>{contributions.paused ? 'Resume route observation' : 'Pause route observation'}</button
			>
		{:else}<button
				class="mt-3 rounded border border-byl-border-control px-3 py-2 disabled:opacity-50"
				disabled={!contributions.admitted || contributions.busy}
				onclick={() => contributions.optIn()}
				>Contribute day-level summaries for this revision and mode</button
			>{/if}
	{/if}
	<div class="mt-3 flex flex-wrap gap-3">
		<button
			class="rounded border border-byl-border-control px-3 py-2"
			onclick={() => contributions.refresh()}>Retry contribution work</button
		>
		<button
			class="rounded border border-byl-border-control px-3 py-2"
			onclick={() => contributions.withdrawAll()}
			>Stop and delete contributions from this browser</button
		>
	</div>
	<p class="mt-2 text-xs text-byl-muted">
		Deletion covers choices and retained route contributions made from this browser and server.
		Queued summaries are cleared first. An interrupted consent request may need the original account
		to recover its deletion handle; retry after signing in. Other browsers and devices have their
		own controls.
	</p>
</details>
