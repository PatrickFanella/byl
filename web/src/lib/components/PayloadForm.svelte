<script lang="ts">
	import FieldInput from './Field.svelte';
	import type { Field } from '#lib/payload.ts';
	let { fields = $bindable(), errors = {} }: { fields: Field[]; errors?: Record<string, string> } =
		$props();
</script>

<div class="grid gap-4 md:grid-cols-2">
	{#each fields as field, i (field.key)}
		<div class={field.kind === 'textarea' ? 'md:col-span-2' : ''}>
			<FieldInput bind:field={fields[i]} error={errors[field.key] ?? ''} />
		</div>
	{/each}
</div>
