<script lang="ts">
	import { modal } from '#lib/modal.ts';
	import type { Snippet } from 'svelte';
	let {
		open = $bindable(false),
		title,
		children
	}: { open: boolean; title: string; children: Snippet } = $props();

	function onBackdropKey(e: KeyboardEvent) {
		if (e.key === 'Escape') open = false;
	}
</script>

{#if open}
	<div
		class="byl-scrim fixed inset-0 z-40 flex items-center justify-center p-4"
		role="presentation"
		onclick={() => (open = false)}
		onkeydown={onBackdropKey}
	>
		<div
			class="w-full max-w-lg rounded-lg bg-byl-background p-5 shadow-xl"
			role="dialog"
			use:modal={() => {
				open = false;
			}}
			aria-modal="true"
			aria-label={title}
			tabindex="-1"
			onclick={(e) => e.stopPropagation()}
			onkeydown={(e) => e.stopPropagation()}
		>
			<h2 class="text-lg font-semibold">{title}</h2>
			<div class="mt-4">{@render children()}</div>
		</div>
	</div>
{/if}
