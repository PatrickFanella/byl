<script lang="ts">
	let { status }: { status: string } = $props();
	const tone: Record<string, string> = {
		pending: 'bg-amber-100 text-amber-800',
		approved: 'bg-emerald-100 text-emerald-800',
		rejected: 'bg-stone-200 text-stone-700',
		published: 'bg-emerald-100 text-emerald-800',
		draft: 'bg-stone-100 text-stone-700',
		in_review: 'bg-amber-100 text-amber-800',
		archived: 'bg-stone-200 text-stone-600',
		queued: 'bg-sky-100 text-sky-800',
		running: 'bg-indigo-100 text-indigo-800',
		done: 'bg-emerald-100 text-emerald-800',
		failed: 'bg-red-100 text-red-800',
		ready: 'bg-emerald-100 text-emerald-800',
		import: 'bg-sky-100 text-sky-800',
		user: 'bg-violet-100 text-violet-800'
	};
</script>

<span
	class="inline-block rounded px-2 py-0.5 text-xs font-medium {tone[status] ??
		'bg-stone-100 text-stone-700'}">{status}</span
>
