<script lang="ts">
	let { kind = 'info', message }: { kind?: 'info' | 'error' | 'success'; message: string } =
		$props();
	const tone = {
		info: 'bg-byl-info-surface text-byl-info',
		error: 'bg-byl-danger-surface text-byl-danger',
		success: 'bg-byl-success-surface text-byl-success'
	};
</script>

{#if message}
	<p class="rounded-md p-3 text-sm {tone[kind]}">{message}</p>
{/if}
