<script lang="ts">
	import { onDestroy, tick, untrack } from 'svelte';
	import { ApiError } from '#lib/api/index.ts';
	import { session } from '#lib/auth.ts';
	import {
		submitRouteSuggestion,
		type RouteSuggestion,
		type RouteSuggestionInput
	} from '#lib/visitor.ts';
	import {
		emptySuggestion,
		suggestionFields,
		suggestionLines,
		type SuggestionValues
	} from '#lib/routes/suggestions.ts';

	let {
		ownerID,
		onsaved
	}: { ownerID: string; onsaved: (suggestion: RouteSuggestion) => Promise<void> } = $props();
	const storageKey = untrack(() => `byl.route-suggestion.v1:${ownerID}`);
	type Attempt = { key: string; submitted: RouteSuggestionInput };
	function restore(): { values: SuggestionValues; pending: Attempt | null } {
		const fresh = { values: emptySuggestion(), pending: null };
		try {
			const raw = sessionStorage.getItem(storageKey);
			if (!raw) return fresh;
			const saved = JSON.parse(raw);
			if (
				!suggestionFields.every(
					(field) =>
						typeof saved.values?.[field.key] === 'string' &&
						saved.values[field.key].length <= field.max
				)
			)
				return fresh;
			const pending = saved.pending;
			if (
				pending &&
				(typeof pending.key !== 'string' ||
					!/^[0-9a-f-]{36}$/.test(pending.key) ||
					!pending.submitted ||
					typeof pending.submitted.topic !== 'string' ||
					typeof pending.submitted.area !== 'string')
			)
				return fresh;
			return { values: saved.values, pending: pending ?? null };
		} catch {
			return fresh;
		}
	}
	const initial = restore();
	let values = $state<SuggestionValues>(initial.values);
	let pending = $state<Attempt | null>(initial.pending);
	let busy = $state(false);
	let closed = $state(false);
	let needsSignIn = $state(false);
	let errors = $state<Record<string, string>>({});
	let message = $state('');
	let form: HTMLFormElement;
	let active = true;
	let completed = false;
	onDestroy(() => {
		active = false;
	});
	const optionalOpen = $derived(
		Boolean(pending) ||
			suggestionFields
				.slice(2)
				.some((field) => Boolean(values[field.key]) || Boolean(errors[field.key]))
	);
	function persist() {
		try {
			sessionStorage.setItem(storageKey, JSON.stringify({ values, pending }));
		} catch {
			/* The current page still retains input. */
		}
	}
	$effect(() => {
		if (!completed) persist();
	});
	async function focusError() {
		await tick();
		form.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus();
	}
	function discard() {
		pending = null;
		values = emptySuggestion();
		errors = {};
		message = '';
	}
	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (busy || closed || needsSignIn || session.account()?.id !== ownerID) return;
		errors = {};
		message = '';
		if (!pending) {
			pending = {
				key: crypto.randomUUID(),
				submitted: {
					topic: values.topic.trim(),
					area: values.area.trim(),
					notes: values.notes,
					place_leads: suggestionLines(values.place_leads),
					event_leads: suggestionLines(values.event_leads),
					source_links: suggestionLines(values.source_links),
					trip_shape: 'flexible',
					start_preference: '',
					end_preference: '',
					pilot_id: ''
				}
			};
		}
		// Persist the exact body/key pair before sending, including across reloads.
		persist();
		const attempt = pending;
		if (!attempt) return;
		busy = true;
		try {
			const saved = await submitRouteSuggestion(attempt.submitted, attempt.key);
			if (!active || session.account()?.id !== ownerID) return;
			completed = true;
			try {
				sessionStorage.removeItem(storageKey);
			} catch {
				/* Storage may be unavailable. */
			}
			await onsaved(saved);
		} catch (error) {
			if (!active || session.account()?.id !== ownerID) return;
			if (error instanceof ApiError && error.code === 'intake_disabled') {
				closed = true;
				message =
					'New suggestions are closed. Your original input and retry key are kept in this tab. You can read your existing submissions below.';
			} else if (error instanceof ApiError && error.status === 401) {
				needsSignIn = true;
				message =
					'Sign in again to continue. Your original input and retry key are kept in this tab.';
			} else if (
				error instanceof ApiError &&
				error.status >= 400 &&
				error.status < 500 &&
				error.status !== 409
			) {
				pending = null;
				errors = error.fields;
				message = Object.keys(errors).length
					? 'Check the marked fields and submit again.'
					: error.message;
			} else {
				message =
					'We could not confirm the submission. Your input is still here. Retry sends the same suggestion; check your submissions below before starting another.';
			}
		} finally {
			if (active) {
				busy = false;
				if (Object.keys(errors).length) await focusError();
			}
		}
	}
</script>

{#snippet input(field: (typeof suggestionFields)[number])}
	<div>
		<label for={`suggestion-${field.key}`} class="block text-sm font-medium"
			>{field.label}{'required' in field ? ' (required)' : ''}</label
		>
		<p id={`suggestion-${field.key}-hint`} class="mt-1 text-sm text-byl-muted">{field.help}</p>
		{#if 'required' in field}
			<input
				id={`suggestion-${field.key}`}
				class="mt-2 w-full rounded-md border-byl-border-control"
				required
				maxlength={field.max}
				bind:value={values[field.key]}
				aria-invalid={errors[field.key] ? 'true' : undefined}
				aria-describedby={`suggestion-${field.key}-hint${errors[field.key] ? ` suggestion-${field.key}-error` : ''}`}
			/>
		{:else}
			<textarea
				id={`suggestion-${field.key}`}
				class="mt-2 w-full rounded-md border-byl-border-control"
				rows={3}
				maxlength={field.max}
				bind:value={values[field.key]}
				aria-invalid={errors[field.key] ? 'true' : undefined}
				aria-describedby={`suggestion-${field.key}-hint${errors[field.key] ? ` suggestion-${field.key}-error` : ''}`}
			></textarea>
		{/if}
		{#if errors[field.key]}<p
				id={`suggestion-${field.key}-error`}
				class="mt-1 text-sm text-byl-danger"
			>
				{errors[field.key]}
			</p>{/if}
	</div>
{/snippet}

<section class="mt-8 max-w-3xl" aria-labelledby="suggestion-title">
	<h2 id="suggestion-title" class="text-xl font-semibold">Suggest a public tour</h2>
	<p class="mt-2 text-byl-muted">
		Only topic and area are required. Your notes and links stay private as research leads.
		Submitting does not publish a tour.
	</p>
	<form bind:this={form} onsubmit={submit} novalidate class="mt-5 space-y-5">
		{#if message}<p role="alert" class="text-sm text-byl-danger">{message}</p>{/if}
		{#if needsSignIn}<a
				href="/signin?next=%2Froutes%2Fsuggestions"
				data-sveltekit-reload
				class="inline-block text-byl-action underline">Sign in again</a
			>{/if}
		{#if pending && !busy}<p class="text-sm text-byl-muted">
				A submission may already be saved. Retry uses its original input and does not create a
				second copy.
			</p>{/if}
		<fieldset disabled={busy || Boolean(pending)} class="min-w-0 space-y-5">
			<legend class="sr-only">Private research suggestion</legend>
			{#each suggestionFields.slice(0, 2) as field (field.key)}{@render input(field)}{/each}
			<details open={optionalOpen} class="rounded-lg border border-byl-border p-4">
				<summary class="cursor-pointer font-medium">Optional research leads</summary>
				<div class="mt-4 space-y-5">
					{#each suggestionFields.slice(2) as field (field.key)}{@render input(field)}{/each}
				</div>
			</details>
		</fieldset>
		<div class="flex flex-wrap items-center gap-4">
			<button
				type="submit"
				disabled={busy || closed || needsSignIn}
				class="rounded-lg bg-byl-action px-4 py-2 text-sm font-semibold text-byl-action-text disabled:opacity-50"
				>{closed
					? 'Suggestions closed'
					: busy
						? 'Submitting…'
						: pending
							? 'Retry same suggestion'
							: 'Submit suggestion'}</button
			>
			{#if pending && !busy}<button
					type="button"
					class="rounded-lg border border-byl-border px-4 py-2 text-sm"
					onclick={discard}>Discard local draft</button
				>{/if}
		</div>
		{#if pending && !busy}<p class="text-sm text-byl-muted">
				Discarding this local draft does not remove a suggestion already saved to your account.
			</p>{/if}
	</form>
</section>
