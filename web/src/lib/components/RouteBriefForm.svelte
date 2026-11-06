<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { ApiError } from '#lib/api/index.ts';
	import {
		createRouteBrief,
		updateRouteBrief,
		type RouteBrief,
		type RouteBriefInput
	} from '#lib/admin.ts';

	let {
		brief,
		onsaved
	}: { brief?: RouteBrief; onsaved: (saved: RouteBrief) => void | Promise<void> } = $props();
	const initial = untrack(() => brief?.submitted);
	const fields = [
		{
			key: 'topic',
			label: 'Topic',
			help: 'The people, movement, theme or question this route should explore.',
			max: 300,
			required: true
		},
		{
			key: 'area',
			label: 'Area',
			help: 'A neighborhood, town or region. An exact boundary is optional.',
			max: 500,
			required: true
		},
		{
			key: 'notes',
			label: 'Submitted notes',
			help: 'Unverified context and questions to investigate. These notes stay private.',
			max: 10000,
			multiline: true
		},
		{
			key: 'place_leads',
			label: 'Place leads',
			help: 'One name or address per line. Unresolved locations are welcome; up to 20.',
			max: 40000,
			multiline: true
		},
		{
			key: 'event_leads',
			label: 'Event leads',
			help: 'One event or question per line; up to 20.',
			max: 40000,
			multiline: true
		},
		{
			key: 'source_links',
			label: 'Source links',
			help: 'One HTTP or HTTPS link per line; up to 20. Links are leads, not verified citations.',
			max: 40000,
			multiline: true
		},
		{
			key: 'start_preference',
			label: 'Start preference',
			help: 'Optional meeting place, station or other starting preference.',
			max: 500
		},
		{
			key: 'end_preference',
			label: 'End preference',
			help: 'Optional destination or return preference.',
			max: 500
		},
		{
			key: 'pilot_id',
			label: 'Pilot identifier',
			help: 'Optional identifier for a planned pilot route.',
			max: 100
		}
	] as const;
	type Key = (typeof fields)[number]['key'];
	let values = $state<Record<Key, string>>({
		topic: initial?.topic ?? '',
		area: initial?.area ?? '',
		notes: initial?.notes ?? '',
		place_leads: initial?.place_leads?.join('\n') ?? '',
		event_leads: initial?.event_leads?.join('\n') ?? '',
		source_links: initial?.source_links?.join('\n') ?? '',
		start_preference: initial?.start_preference ?? '',
		end_preference: initial?.end_preference ?? '',
		pilot_id: initial?.pilot_id ?? ''
	});
	let modes = $state<string[]>(initial?.modes ?? ['walk', 'bike']);
	let shape = $state(initial?.trip_shape ?? 'flexible');
	let duration = $state<number | undefined>(initial?.duration_minutes ?? undefined);
	let busy = $state(false);
	let errors = $state<Record<string, string>>({});
	let message = $state('');
	let form: HTMLFormElement;
	let lastAttempt = '';
	let requestKey = '';
	const locked = $derived(brief?.status === 'processing');
	const lines = (value: string) =>
		value
			.split('\n')
			.map((line) => line.trim())
			.filter(Boolean);

	async function focusError() {
		await tick();
		form.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
	}

	async function save(event: SubmitEvent) {
		event.preventDefault();
		errors = {};
		message = '';
		if (!modes.length) {
			errors = { modes: 'Choose walking, cycling, or both.' };
			await focusError();
			return;
		}
		const submitted: RouteBriefInput = {
			...values,
			place_leads: lines(values.place_leads),
			event_leads: lines(values.event_leads),
			source_links: lines(values.source_links),
			modes: modes as ('walk' | 'bike')[],
			trip_shape: shape,
			duration_minutes: duration ?? null
		};
		const attempt = JSON.stringify(submitted);
		if (attempt !== lastAttempt) {
			requestKey = crypto.randomUUID();
			lastAttempt = attempt;
		}
		busy = true;
		try {
			const saved = brief
				? await updateRouteBrief(brief.id, brief.version, submitted)
				: await createRouteBrief(submitted, requestKey);
			await onsaved(saved);
		} catch (error) {
			if (error instanceof ApiError) {
				errors = error.fields;
				message =
					error.status === 409
						? 'This brief changed since you opened it. Your unsaved edits are still here. Copy them before reloading the saved brief.'
						: Object.keys(errors).length
							? 'Check the marked fields and save again.'
							: error.message;
			} else {
				message = 'Could not reach the API. Your edits are still here; try saving again.';
			}
		} finally {
			busy = false;
			if (Object.keys(errors).length) await focusError();
		}
	}
</script>

<form bind:this={form} onsubmit={save} novalidate class="mt-6 max-w-3xl space-y-5">
	{#if message}<p role="alert" class="text-sm text-byl-danger">{message}</p>{/if}
	{#if locked}<p class="text-sm text-byl-muted">
			Processing is in progress. Editing is available after processing stops.
		</p>{/if}
	<fieldset disabled={busy || locked} class="min-w-0 space-y-5">
		<legend class="sr-only">Private route brief</legend>
		{#each fields as field (field.key)}
			<div>
				<label for={`brief-${field.key}`} class="block text-sm font-medium"
					>{field.label}{'required' in field && field.required ? ' (required)' : ''}</label
				>
				<p id={`brief-${field.key}-hint`} class="mt-1 text-sm text-byl-muted">{field.help}</p>
				{#if 'multiline' in field && field.multiline}
					<textarea
						id={`brief-${field.key}`}
						class="mt-2 w-full rounded-md border-byl-border-control"
						rows={field.key === 'notes' ? 5 : 3}
						maxlength={field.max}
						bind:value={values[field.key]}
						aria-invalid={errors[field.key] ? 'true' : undefined}
						aria-describedby={`brief-${field.key}-hint${errors[field.key] ? ` brief-${field.key}-error` : ''}`}
					></textarea>
				{:else}
					<input
						id={`brief-${field.key}`}
						class="mt-2 w-full rounded-md border-byl-border-control"
						maxlength={field.max}
						required={'required' in field && field.required}
						bind:value={values[field.key]}
						aria-invalid={errors[field.key] ? 'true' : undefined}
						aria-describedby={`brief-${field.key}-hint${errors[field.key] ? ` brief-${field.key}-error` : ''}`}
					/>
				{/if}
				{#if errors[field.key]}<p
						id={`brief-${field.key}-error`}
						class="mt-1 text-sm text-byl-danger"
					>
						{errors[field.key]}
					</p>{/if}
			</div>
		{/each}
		<fieldset aria-describedby={errors.modes ? 'brief-modes-error' : undefined}>
			<legend class="text-sm font-medium">Travel modes</legend>
			<div class="mt-2 flex flex-wrap gap-5">
				{#each [{ value: 'walk', label: 'Walking' }, { value: 'bike', label: 'Cycling' }] as mode (mode.value)}
					<label class="flex items-center gap-2 text-sm"
						><input
							type="checkbox"
							value={mode.value}
							bind:group={modes}
							aria-invalid={errors.modes ? 'true' : undefined}
						/>{mode.label}</label
					>
				{/each}
			</div>
			{#if errors.modes}<p id="brief-modes-error" class="mt-1 text-sm text-byl-danger">
					{errors.modes}
				</p>{/if}
		</fieldset>
		<div class="grid gap-5 sm:grid-cols-2">
			<div>
				<label for="brief-shape" class="block text-sm font-medium">Trip shape</label>
				<select
					id="brief-shape"
					class="mt-2 w-full rounded-md border-byl-border-control"
					bind:value={shape}
					aria-invalid={errors.trip_shape ? 'true' : undefined}
					aria-describedby={errors.trip_shape ? 'brief-shape-error' : undefined}
				>
					<option value="flexible">Flexible</option><option value="loop">Loop</option><option
						value="point_to_point">Point to point</option
					>
				</select>
				{#if errors.trip_shape}<p id="brief-shape-error" class="mt-1 text-sm text-byl-danger">
						{errors.trip_shape}
					</p>{/if}
			</div>
			<div>
				<label for="brief-duration" class="block text-sm font-medium"
					>Duration in minutes (optional)</label
				>
				<input
					id="brief-duration"
					type="number"
					min="15"
					max="1440"
					class="mt-2 w-full rounded-md border-byl-border-control"
					bind:value={duration}
					aria-invalid={errors.duration_minutes ? 'true' : undefined}
					aria-describedby={errors.duration_minutes ? 'brief-duration-error' : undefined}
				/>
				{#if errors.duration_minutes}<p
						id="brief-duration-error"
						class="mt-1 text-sm text-byl-danger"
					>
						{errors.duration_minutes}
					</p>{/if}
			</div>
		</div>
		<button
			type="submit"
			class="rounded-md bg-byl-action px-4 py-2 text-sm text-byl-action-text disabled:opacity-50"
			>{busy ? 'Saving…' : brief ? 'Save changes' : 'Save brief'}</button
		>
	</fieldset>
</form>
