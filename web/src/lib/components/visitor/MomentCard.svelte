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
		class="relative block h-52 overflow-hidden rounded-3xl border border-white/10 bg-gradient-to-br from-blue-500 to-purple-500"
	>
		{#if hero}<img src={hero} alt="" class="absolute inset-0 h-full w-full object-cover" />{/if}
		<div class="absolute inset-x-0 bottom-0 bg-gradient-to-t from-black/90 to-transparent p-4">
			{#if moment.date_display}
				<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">
					{moment.date_display}
				</div>
			{/if}
			<div class="text-xl font-semibold">{moment.title}</div>
			<p class="mt-1 line-clamp-2 text-sm text-white/85">{moment.excerpt}</p>
			{#if moment.has_narration}
				<div class="mt-1 text-[10px] font-bold tracking-wide text-white/70 uppercase">
					Audio guide
				</div>
			{/if}
		</div>
	</a>
{:else}
	<a
		href="/moments/{moment.id}"
		data-testid={testid}
		class="block rounded-2xl border border-white/10 bg-white/5 p-4 hover:bg-white/10"
	>
		{#if moment.date_display}
			<div class="text-[11px] font-bold tracking-wide text-blue-300">{moment.date_display}</div>
		{/if}
		<div class="font-semibold">{moment.title}</div>
		<p class="line-clamp-2 text-sm text-white/60">{moment.excerpt}</p>
	</a>
{/if}
