<script lang="ts">
	import type { RouteJourney } from '#lib/tour/estimate.ts';
	import { modeLabel, tripEstimateText } from '#lib/tour/estimate.ts';
	let { mode, journey }: { mode: string; journey?: RouteJourney | null } = $props();
</script>

<section
	aria-label={`${modeLabel(mode)} route readiness`}
	class="mt-3 space-y-2 border-t border-byl-border pt-3 text-sm"
>
	<p class="font-semibold">
		{modeLabel(mode)} · {journey?.state.replaceAll('_', ' ') ?? 'Assessment unavailable'}
	</p>
	{#if journey}
		<p>
			{journey.travel_duration_s == null
				? 'Travel time unavailable'
				: `${Math.ceil(journey.travel_duration_s / 60)} min travel`} · {Math.ceil(
				journey.dwell_duration_s / 60
			)} min planned stops · {Math.ceil(journey.narration_duration_s / 60)} min planned narration
		</p>
		{#if tripEstimateText(journey)}<p>{tripEstimateText(journey)}</p>{/if}
		{#if !journey.usable}<p class="text-byl-warning">
				This path is not qualified for guided travel. Check access independently; saved stop visits
				remain available.
			</p>{/if}
		{#if journey.segments.length}<details>
				<summary class="cursor-pointer">Trail and connecting sections</summary>
				<ol class="mt-2 list-decimal space-y-2 pl-5">
					{#each journey.segments as segment, index (`${segment.id}:${index}`)}<li>
							{segment.id} · {segment.kind === 'road_connector' ? 'Road connector' : 'Trail'} · {segment.reverse
								? 'Reverse direction'
								: 'Forward direction'} · Access: {segment.access} · Surface: {segment.surface ??
								'Unknown'}
							{#if /^https?:\/\//i.test(segment.source.url)}<a
									class="ml-2 underline"
									href={segment.source.url}
									target="_blank"
									rel="noreferrer">{segment.source.title || 'Reviewed reference'}</a
								>{/if}
						</li>{/each}
				</ol>
			</details>{/if}
		<details>
			<summary class="cursor-pointer">Timing and return assumptions</summary>
			<ul class="mt-2 list-disc pl-5">
				{#each journey.assumptions as assumption (assumption)}<li>{assumption}</li>{/each}
			</ul>
			<p class="mt-2">Return: {journey.return_plan.status.replaceAll('_', ' ')}.</p>
		</details>
	{:else}<p class="text-byl-warning">
			This revision has no readiness assessment. A computed line alone does not establish legal or
			current access.
		</p>{/if}
</section>
