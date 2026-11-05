<script lang="ts">
	import type { MomentSummary } from '#lib/visitor.ts';

	let {
		moment,
		prominent = false,
		testid = 'moment-card'
	}: { moment: MomentSummary; prominent?: boolean; testid?: string } = $props();
	const hero = $derived(moment.hero_media?.public_url ?? null);
</script>

{#if prominent}
	<a
		href="/moments/{moment.id}"
		data-testid={testid}
		class="block overflow-hidden rounded-lg border border-byl-border bg-byl-surface"
	>
		{#if hero}<img src={hero} alt="" class="h-44 w-full object-cover" />{/if}
		<div class="bg-byl-background p-4">
			{#if moment.date_display}
				<div class="text-xs font-bold tracking-normal text-byl-action">
					{moment.date_display}
				</div>
			{/if}
			<div class="font-[Libre_Baskerville] text-xl">{moment.title}</div>
			<p class="mt-1 line-clamp-2 text-sm text-byl-muted">{moment.excerpt}</p>
			{#if moment.has_narration}
				<div class="mt-1 text-xs font-bold tracking-wide text-byl-muted">Audio guide</div>
			{/if}
		</div>
	</a>
{:else}
	<a
		href="/moments/{moment.id}"
		data-testid={testid}
		class="block rounded-lg border border-byl-border bg-byl-surface p-4 hover:bg-byl-surface"
	>
		{#if moment.date_display}
			<div class="text-xs font-bold tracking-wide text-byl-action">{moment.date_display}</div>
		{/if}
		<div class="font-semibold">{moment.title}</div>
		<p class="line-clamp-2 text-sm text-byl-muted">{moment.excerpt}</p>
	</a>
{/if}
