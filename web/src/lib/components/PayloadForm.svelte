<script lang="ts">
	import FieldInput from './Field.svelte';
	import type { Field } from '#lib/payload.ts';
	let { fields = $bindable(), errors = {} }: { fields: Field[]; errors?: Record<string, string> } =
		$props();
</script>

<div class="grid gap-4 md:grid-cols-2">
	{#each fields as _, i (fields[i].key)}
		<div class={fields[i].kind === 'textarea' ? 'md:col-span-2' : ''}>
			<FieldInput bind:field={fields[i]} error={errors[fields[i].key] ?? ''} />
		</div>
	{/each}
</div>
