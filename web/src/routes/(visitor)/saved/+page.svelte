<script lang="ts">
	import { session } from '#lib/auth.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import { myProgress, mySaved, type TourProgress, type TourSummary } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import TourCard from '#lib/components/visitor/TourCard.svelte';

	const current = session.store;
	const saved = new Loader<{ tours: TourSummary[]; progress: Record<string, TourProgress> }>(
		async () => {
			const [t, p] = await Promise.all([mySaved(), myProgress()]);
			const best: Record<string, TourProgress> = {};
			for (const rec of p.items) {
				if ((best[rec.tour_id]?.visited_positions.length ?? -1) < rec.visited_positions.length)
					best[rec.tour_id] = rec;
			}
			return { tours: t.items, progress: best };
		},
		(v) => v.tours.length === 0
	);
	$effect(() => {
		if ($current) saved.run();
	});
</script>

<svelte:head><title>Saved · Invisible Museum</title></svelte:head>

<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">Saved</div>
<h1 class="text-3xl font-semibold">Tours to come back to</h1>

<div class="mt-6">
	{#if !$current}
		<SignInPrompt reason="Sign in to save tours and keep your progress on every device." />
	{:else}
		<LoadState
			status={saved.status}
			message={saved.message}
			emptyTitle="Nothing saved yet"
			emptyMessage="Tap Save for later on any tour."
			onretry={() => saved.run()}
		>
			<div class="grid gap-6 sm:grid-cols-2">
				{#each saved.data?.tours ?? [] as tour (tour.id)}
					{@const p = saved.data?.progress[tour.id]}
					<div class="space-y-2">
						<TourCard {tour} />
						{#if p}
							<div class="flex items-center gap-3 px-1 text-xs text-white/60">
								<progress
									class="h-1.5 flex-1 overflow-hidden rounded-full"
									max={Math.max(tour.stop_count, 1)}
									value={p.visited_positions.length}
								></progress>
								<span
									>{p.completed_at
										? 'Completed'
										: `${p.visited_positions.length} of ${tour.stop_count} stops`}</span
								>
							</div>
						{/if}
					</div>
				{/each}
			</div>
		</LoadState>
	{/if}
</div>
