<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session } from '#lib/auth.ts';
	import { ApiError } from '#lib/api/index.ts';
	import { Loader } from '#lib/loader.svelte.ts';
	import { getTour, getTourRevision } from '#lib/visitor.ts';
	import LoadState from '#lib/components/visitor/LoadState.svelte';
	import SignInPrompt from '#lib/components/visitor/SignInPrompt.svelte';
	import RouteReportForm from '#lib/components/visitor/RouteReportForm.svelte';
	const current = session.store;
	const ref = $derived(page.params.ref ?? '');
	const revisionID = $derived(page.url.searchParams.get('revision') ?? '');
	const mode = $derived(page.url.searchParams.get('mode') === 'bike' ? 'bike' : 'walk');
	const revision = new Loader(async () => {
		const tour = await getTour(ref);
		const id = revisionID || tour.revision_id;
		if (!id)
			throw new ApiError(
				409,
				'unavailable',
				'A published route revision is needed before reporting a path problem.'
			);
		const retained = await getTourRevision(tour.id, id);
		if (
			!retained.snapshot ||
			retained.id !== id ||
			retained.tour_id !== tour.id ||
			retained.snapshot.tour.id !== tour.id ||
			retained.snapshot.tour.revision_id !== retained.id ||
			retained.snapshot.tour.revision_number !== retained.number ||
			!retained.snapshot.tour.modes.includes(mode) ||
			retained.snapshot.routes[mode]?.status !== 'ready'
		)
			throw new ApiError(
				409,
				'unavailable',
				'The selected revision has no retained path for this travel mode.'
			);
		return retained;
	});
	$effect(() => {
		void ref;
		void revisionID;
		void mode;
		void $current;
		revision.run();
	});
</script>

<svelte:head><title>Suggest a route change · Byl</title></svelte:head>
<a href={`/tours/${encodeURIComponent(ref)}`} class="text-sm text-byl-muted underline">← Tour</a>
<h1 class="byl-title mt-3">Suggest a route change</h1>
<p class="mt-3 max-w-2xl text-byl-muted">
	Report a closure, entrance or path problem. You do not need to draw a replacement. Curators review
	observations before changing a public route.
</p>
<LoadState status={revision.status} message={revision.message} onretry={() => revision.run()}>
	{#if revision.data}
		{#if !$current}<div class="mt-6">
				<SignInPrompt reason="Sign in to save a private route report and follow its decision." />
			</div>
		{:else}{#key `${$current.account.id}:${revision.data.id}:${mode}`}<RouteReportForm
					revision={revision.data}
					{mode}
					ownerID={$current.account.id}
					initialTarget={page.url.searchParams.get('affected') ?? ''}
					onsaved={async (report) => {
						await goto(`/routes/reports/${report.id}?submitted=1`);
					}}
				/>{/key}{/if}
	{/if}
</LoadState>
