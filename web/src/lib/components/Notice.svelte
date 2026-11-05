<script lang="ts">
	let { kind = 'info', message }: { kind?: 'info' | 'error' | 'success'; message: string } =
		$props();
	const tone = {
		info: 'bg-sky-50 text-sky-800',
		error: 'bg-red-50 text-red-700',
		success: 'bg-emerald-50 text-emerald-800'
	};
</script>

{#if message}
	<p class="rounded-md p-3 text-sm {tone[kind]}">{message}</p>
{/if}
