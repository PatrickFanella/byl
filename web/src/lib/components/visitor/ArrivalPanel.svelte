<script lang="ts">
	import type { TourStop, PlaceDetail } from '#lib/visitor.ts';
	import { getPlace } from '#lib/visitor.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import MomentCard from './MomentCard.svelte';
	import LoadState from './LoadState.svelte';

	let {
		stop,
		isLast = false,
		playing = false,
		onlisten,
		oncontinue
	}: {
		stop: TourStop;
		isLast?: boolean;
		playing?: boolean;
		onlisten?: () => void;
		oncontinue: () => void;
	} = $props();

	const featured = $derived(stop.featured_moment ?? null);
	const place = new Loader<PlaceDetail>(() => getPlace(stop.place.id));
	$effect(() => {
		if (stop.place.moment_count > 1) place.run();
	});
	const others = $derived((place.data?.moments ?? []).filter((m) => m.id !== featured?.id));
</script>

<div
	class="fixed inset-x-0 bottom-0 z-20 max-h-[85vh] overflow-y-auto rounded-t-3xl border-t border-white/10 bg-[#0a0a0b] p-5 text-white shadow-2xl sm:inset-x-auto sm:right-4 sm:bottom-4 sm:w-[28rem] sm:rounded-3xl sm:border"
	role="dialog"
	aria-label="You have arrived"
>
	<div class="text-[11px] font-bold tracking-widest text-emerald-300 uppercase">
		You have arrived · stop {stop.position}
	</div>
	<h2 class="mt-1 text-2xl font-semibold">{stop.place.name}</h2>
	{#if stop.note}<p class="mt-1 text-sm text-white/60 italic">{stop.note}</p>{/if}

	{#if featured}
		<div class="mt-4"><MomentCard moment={featured} prominent testid="arrival-moment" /></div>
		<div class="mt-3 flex gap-2">
			<a
				href="/moments/{featured.id}"
				class="rounded-xl border border-white/20 px-3 py-2 text-sm hover:bg-white/10"
				>Read the full story</a
			>
			{#if onlisten}
				<button
					class="rounded-xl border border-white/20 px-3 py-2 text-sm hover:bg-white/10"
					onclick={onlisten}>{playing ? '■ Stop' : '▶ Listen'}</button
				>
			{/if}
		</div>
	{:else}
		<div class="mt-4">
			<LoadState
				status="empty"
				emptyTitle="No moment featured here"
				emptyMessage="This stop is a waypoint; look around and carry on."
			/>
		</div>
	{/if}

	{#if stop.place.moment_count > 1}
		<div class="mt-5 space-y-2">
			<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">
				Also at this place
			</div>
			<LoadState
				status={place.status === 'ready' && others.length === 0 ? 'empty' : place.status}
				message={place.message}
				emptyTitle="Nothing else here"
			>
				{#each others as m (m.id)}<MomentCard moment={m} />{/each}
			</LoadState>
		</div>
	{/if}

	<button
		class="mt-5 w-full rounded-xl bg-white px-4 py-3 text-sm font-semibold text-black"
		onclick={oncontinue}>{isLast ? 'Finish' : 'Continue to the next stop'}</button
	>
</div>
