<script lang="ts">
	import { page } from '$app/state';
	import { invalidateAll } from '$app/navigation';
	import BriefItineraryPanel from '#lib/components/BriefItineraryPanel.svelte';
	import CuratorPackagePanel from '#lib/components/CuratorPackagePanel.svelte';
	import RouteBriefForm from '#lib/components/RouteBriefForm.svelte';
	import { when } from '#lib/format.ts';
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let saved = $state(false);
	let refreshError = $state('');
	let requestedKey = $state('');
	const researchRequested = $derived(requestedKey === `${data.brief.id}:${data.brief.version}`);
	const work = $derived(
		researchRequested
			? 'Research was requested from this saved version. Reload progress after processing; unsaved form text is retained.'
			: {
					draft: 'This brief is saved. Research has not started.',
					processing: 'Research is in progress. Editing is paused until processing stops.',
					needs_research: 'More research is needed before this brief can move forward.',
					review: 'This brief is ready for curator review.',
					accepted:
						'A curator accepted this brief. Check the package review below for the publication result.',
					declined: 'A curator declined this brief. Saving edits creates a draft for fresh review.'
				}[data.brief.status]
	);
	async function refresh() {
		refreshError = '';
		try {
			await invalidateAll();
		} catch {
			refreshError = 'Could not reload the saved brief. Try again.';
		}
	}
</script>

<a href="/admin/briefs" class="text-sm underline">Route briefs</a>
<h1 class="mt-4 text-2xl font-semibold break-words">{data.brief.submitted.topic}</h1>
<section aria-label="Brief progress" class="mt-4 max-w-3xl border-y border-byl-border py-4 text-sm">
	<p class="font-medium">
		State: {researchRequested ? 'research requested' : data.brief.status.replaceAll('_', ' ')}
	</p>
	<p class="mt-2">{work}</p>
	{#if data.brief.reason}<p class="mt-2 break-words whitespace-pre-wrap">
			<strong>Reason:</strong>
			{data.brief.reason}
		</p>{/if}
	<p class="mt-2 text-byl-muted">
		Version {data.brief.version} · Updated {when(data.brief.updated_at)}
	</p>
	<button type="button" class="mt-3 underline" onclick={refresh}>Reload saved brief</button>
	{#if refreshError}<p role="alert" class="mt-2 text-byl-danger">{refreshError}</p>{/if}
</section>
{#if saved || page.url.searchParams.has('saved')}<p
		role="status"
		class="mt-4 text-sm text-byl-success"
	>
		Brief saved.
	</p>{/if}
{#key data.brief.id}
	<RouteBriefForm
		brief={data.brief}
		onsaved={async () => {
			saved = true;
			await invalidateAll();
		}}
	/>
{/key}

<BriefItineraryPanel
	brief={data.brief}
	onresearchrequested={(id, version) => (requestedKey = `${id}:${version}`)}
/>

{#key data.brief.id}
	<CuratorPackagePanel brief={data.brief} onchanged={refresh} />
{/key}
