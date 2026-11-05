<script lang="ts">
	import { modal } from '#lib/modal.ts';
	import { session } from '#lib/auth.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import { deleteTour, myTours, type TourSummary } from '#lib/visitor.ts';
	import { shareURL } from '#lib/routes/builder.svelte.ts';
	import { ApiError } from '#lib/api/index.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import TourCard from '#lib/components/visitor/TourCard.svelte';
	import ShareLink from '#lib/components/visitor/ShareLink.svelte';

	const current = session.store;
	const routes = new Loader<TourSummary[]>(
		async () => (await myTours()).items,
		(v) => v.length === 0
	);
	let sharing = $state<string | null>(null);
	let pendingDelete = $state<TourSummary | null>(null);
	let deleteError = $state('');

	$effect(() => {
		if ($current) routes.run();
	});

	async function remove(tour: TourSummary) {
		deleteError = '';
		try {
			await deleteTour(tour.id);
			pendingDelete = null;
			routes.run();
		} catch (e) {
			deleteError = e instanceof ApiError ? e.message : 'Could not delete the route.';
		}
	}
	const origin = typeof location === 'undefined' ? '' : location.origin;
</script>

<svelte:head><title>Your routes · Invisible Museum</title></svelte:head>

<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">Your routes</div>
<h1 class="text-3xl font-semibold">Curate your own gallery</h1>
<p class="mt-1 max-w-2xl text-white/60">
	Order the places you love into a walk or ride and share it by link.
</p>

<div class="mt-6 space-y-4">
	{#if !$current}
		<SignInPrompt reason="Sign in to build routes that follow you to every device." />
	{:else}
		<a
			href="/routes/new"
			class="inline-block rounded-xl bg-white px-4 py-2 text-sm font-semibold text-black"
			>+ New route</a
		>
		<LoadState
			status={routes.status}
			message={routes.message}
			emptyTitle="No routes yet"
			emptyMessage="Tap New route to pick your first two places."
			onretry={() => routes.run()}
		>
			<div class="grid gap-6 sm:grid-cols-2">
				{#each routes.data ?? [] as tour (tour.id)}
					<div class="space-y-2">
						<TourCard {tour} />
						<div class="flex flex-wrap items-center gap-2 text-sm">
							<button
								class="rounded-xl border border-white/20 px-3 py-1.5 hover:bg-white/10"
								onclick={() => (sharing = sharing === tour.id ? null : tour.id)}>Share</button
							>
							<a
								href="/routes/{tour.id}/edit"
								class="rounded-xl border border-white/20 px-3 py-1.5 hover:bg-white/10">Edit</a
							>
							<button
								class="rounded-xl border border-red-400/40 px-3 py-1.5 text-red-200 hover:bg-white/10"
								onclick={() => (pendingDelete = tour)}>Delete</button
							>
							<span class="text-xs text-white/40">{tour.visibility}</span>
						</div>
						{#if sharing === tour.id}
							<ShareLink url={shareURL(origin, tour.slug)} title={tour.title} />
						{/if}
					</div>
				{/each}
			</div>
		</LoadState>
	{/if}
</div>

{#if pendingDelete}
	<div class="fixed inset-0 z-30 flex items-center justify-center bg-black/60 p-4">
		<div
			class="w-full max-w-sm rounded-3xl border border-white/10 bg-[#0a0a0b] p-5"
			role="dialog"
			use:modal={() => {
				pendingDelete = null;
			}}
			aria-label="Delete this route?"
		>
			<h2 class="text-lg font-semibold">Delete this route?</h2>
			<p class="mt-1 text-sm text-white/60">{pendingDelete.title}</p>
			{#if deleteError}<p class="mt-2 text-sm text-red-300">{deleteError}</p>{/if}
			<div class="mt-4 flex gap-2">
				<button
					class="rounded-xl bg-red-500 px-3 py-2 text-sm font-semibold text-white"
					onclick={() => remove(pendingDelete!)}>Delete</button
				>
				<button
					class="rounded-xl border border-white/20 px-3 py-2 text-sm"
					onclick={() => (pendingDelete = null)}>Cancel</button
				>
			</div>
		</div>
	</div>
{/if}
