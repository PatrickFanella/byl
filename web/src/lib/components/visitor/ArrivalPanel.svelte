<script lang="ts">
	import HappeningsPanel from './HappeningsPanel.svelte';
	import { modal } from '#lib/modal.ts';
	import type { TourStop, PlaceDetail, MomentDetail } from '#lib/visitor.ts';
	import { getPlace } from '#lib/visitor.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import MomentCard from './MomentCard.svelte';
	import LoadState from './LoadState.svelte';

	let {
		stop,
		isLast = false,
		playing = false,
		pinned = false,
		pinnedMoment = null,
		onlisten,
		oncontinue
	}: {
		stop: TourStop;
		isLast?: boolean;
		playing?: boolean;
		pinned?: boolean;
		pinnedMoment?: MomentDetail | null;
		onlisten?: () => void;
		oncontinue: () => void;
	} = $props();

	const featured = $derived(stop.featured_moment ?? null);
	const place = new Loader<PlaceDetail>(() => getPlace(stop.place.id));
	$effect(() => {
		if (!pinned && stop.place.moment_count > 1) place.run();
	});
	const others = $derived((place.data?.moments ?? []).filter((m) => m.id !== featured?.id));
</script>

<div
	class="fixed inset-x-0 bottom-0 z-20 max-h-[85vh] overflow-y-auto rounded-t-lg border-t border-byl-border bg-byl-background p-5 text-byl-text shadow-2xl sm:inset-x-auto sm:right-4 sm:bottom-4 sm:w-[28rem] sm:rounded-lg sm:border"
	role="dialog"
	use:modal={() => {
		oncontinue();
	}}
	aria-label="You have arrived"
>
	<div class="text-xs font-bold tracking-normal text-byl-success">
		You have arrived · stop {stop.position}
	</div>
	<h2 class="mt-1 text-2xl font-semibold">{stop.place.name}</h2>
	{#if stop.note}<p class="mt-1 text-sm text-byl-muted italic">{stop.note}</p>{/if}

	{#if featured}
		<div class="mt-4"><MomentCard moment={featured} prominent testid="arrival-moment" /></div>
		<div class="mt-3 flex gap-2">
			{#if pinned}<details>
					<summary class="cursor-pointer rounded-lg border border-byl-border px-3 py-2 text-sm"
						>Read the reviewed story</summary
					>
					<p class="mt-3 whitespace-pre-wrap">{pinnedMoment?.body ?? featured.excerpt}</p>
				</details>{:else}
				<a
					href="/moments/{featured.id}"
					class="rounded-lg border border-byl-border px-3 py-2 text-sm hover:bg-byl-surface"
					>Read the full story</a
				>
			{/if}
			{#if onlisten}
				<button
					class="rounded-lg border border-byl-border px-3 py-2 text-sm hover:bg-byl-surface"
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

	<div class="mt-5"><HappeningsPanel placeID={stop.place.id} tripContext /></div>
	{#if !pinned && stop.place.moment_count > 1}
		<div class="mt-5 space-y-2">
			<div class="byl-eyebrow">Also at this place</div>
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
		class="mt-5 w-full rounded-lg bg-byl-action px-4 py-3 text-sm font-semibold text-byl-action-text"
		onclick={oncontinue}>{isLast ? 'Finish' : 'Continue to the next stop'}</button
	>
</div>
