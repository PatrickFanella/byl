<script lang="ts">
	let { title, eyebrow = '' }: { title: string; eyebrow?: string } = $props();
</script>

<div>
	{#if eyebrow}
		<div class="text-xs font-bold tracking-normal text-byl-action">{eyebrow}</div>
	{/if}
	<h2 class="text-lg font-semibold">{title}</h2>
</div>
