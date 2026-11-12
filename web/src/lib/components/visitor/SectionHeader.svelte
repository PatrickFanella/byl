<script lang="ts">
	let { title, eyebrow = '' }: { title: string; eyebrow?: string } = $props();
</script>

<div>
	{#if eyebrow}
		<div class="byl-eyebrow">{eyebrow}</div>
	{/if}
	<h2 class="byl-h2">{title}</h2>
</div>
