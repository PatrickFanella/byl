<script lang="ts">
	let { status }: { status: string } = $props();
	const tone: Record<string, string> = {
		pending: 'bg-byl-warning-surface text-byl-warning',
		approved: 'bg-byl-success-surface text-byl-success',
		rejected: 'bg-byl-surface text-byl-text',
		published: 'bg-byl-success-surface text-byl-success',
		draft: 'bg-byl-surface text-byl-text',
		in_review: 'bg-byl-warning-surface text-byl-warning',
		archived: 'bg-byl-surface text-byl-muted',
		queued: 'bg-byl-info-surface text-byl-info',
		running: 'bg-byl-info-surface text-byl-info',
		done: 'bg-byl-success-surface text-byl-success',
		failed: 'bg-byl-danger-surface text-byl-danger',
		ready: 'bg-byl-success-surface text-byl-success',
		import: 'bg-byl-info-surface text-byl-info',
		user: 'bg-byl-info-surface text-byl-info'
	};
</script>

<span
	class="inline-block rounded px-2 py-0.5 text-xs font-medium {tone[status] ??
		'bg-byl-surface text-byl-text'}">{status}</span
>
