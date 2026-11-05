<script lang="ts">
	import type { Field } from '#lib/payload.ts';
	let { field = $bindable(), error = '' }: { field: Field; error?: string } = $props();
	const id = `f-${field.key}`;
</script>

<label class="block text-sm" for={id}>
	<span class="text-byl-text">{field.label}</span>
	{#if field.kind === 'textarea'}
		<textarea
			{id}
			class="mt-1 w-full rounded-md border-byl-border-control font-mono text-sm"
			rows={field.key === 'body' ? 10 : 4}
			bind:value={field.value}></textarea>
	{:else if field.kind === 'select'}
		<select {id} class="mt-1 w-full rounded-md border-byl-border-control" bind:value={field.value}>
			<option value="">—</option>
			{#each field.options ?? [] as opt (opt)}
				<option value={opt}>{opt}</option>
			{/each}
		</select>
	{:else if field.kind === 'number'}
		<input
			{id}
			class="mt-1 w-full rounded-md border-byl-border-control"
			type="number"
			step="any"
			bind:value={field.value}
		/>
	{:else if field.kind === 'date'}
		<input
			{id}
			class="mt-1 w-full rounded-md border-byl-border-control"
			type="date"
			bind:value={field.value}
		/>
	{:else}
		<input
			{id}
			class="mt-1 w-full rounded-md border-byl-border-control"
			type="text"
			bind:value={field.value}
		/>
	{/if}
	{#if error}
		<span class="mt-1 block text-xs text-byl-danger">{error}</span>
	{:else if field.help}
		<span class="mt-1 block text-xs text-byl-muted">{field.help}</span>
	{/if}
</label>
