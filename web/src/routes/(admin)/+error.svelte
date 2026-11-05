<script lang="ts">
	import '../layout.css';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session } from '#lib/auth.ts';
	import { api } from '#lib/api/index.ts';

	const current = session.store;

	async function signOut() {
		try {
			await api.del('/api/v1/auth/session');
		} catch {
			// token may already be gone
		}
		session.clear();
		goto('/signin');
	}
</script>

<div
	class="flex min-h-screen items-center justify-center bg-byl-background p-6 text-center text-byl-text"
>
	<div class="max-w-md space-y-4">
		{#if page.status === 403}
			<h1 class="text-2xl font-semibold">This account needs the curator role</h1>
			<p class="text-byl-muted">
				{#if $current}
					You are signed in as <strong>{$current.account.email}</strong> with the
					<strong>{$current.account.role}</strong> role.
				{/if}
				Ask an administrator to grant curator access, then sign in again.
			</p>
		{:else}
			<h1 class="text-2xl font-semibold">{page.status}</h1>
			<p class="text-byl-muted">{page.error?.message ?? 'Something went wrong.'}</p>
		{/if}
		<p><a class="underline" href="/">Back to the visitor site</a></p>
		<button class="rounded-md bg-byl-action px-4 py-2 text-byl-action-text" onclick={signOut}
			>Sign out</button
		>
	</div>
</div>
