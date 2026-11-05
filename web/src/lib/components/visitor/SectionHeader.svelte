<script lang="ts">
	let { title, eyebrow = '' }: { title: string; eyebrow?: string } = $props();
</script>

<div>
	{#if eyebrow}
		<div class="text-[11px] font-bold tracking-widest text-blue-300 uppercase">{eyebrow}</div>
	{/if}
	<h2 class="text-lg font-semibold">{title}</h2>
</div>
