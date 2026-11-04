<script lang="ts">
	import './layout.css';
	import favicon from '$lib/assets/favicon.svg';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session, canCurate } from '$lib/auth';
	import { api } from '$lib/api';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	const current = session.store;

	const nav = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/queue', label: 'Review queue' },
		{ href: '/imports', label: 'Imports' },
		{ href: '/places', label: 'Places' },
		{ href: '/media', label: 'Media' },
		{ href: '/tours', label: 'Tours' },
		{ href: '/jobs', label: 'Jobs' }
	];

	let isSignIn = $derived(page.url.pathname.startsWith('/signin'));

	$effect(() => {
		if (!isSignIn && !$current) {
			goto(`/signin?next=${encodeURIComponent(page.url.pathname)}`, { replaceState: true });
		}
	});

	async function signOut() {
		try {
			await api.del('/api/v1/auth/session');
		} catch {
			// token may already be gone
		}
		session.clear();
		goto('/signin');
	}

	function active(href: string) {
		return href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Invisible Museum admin</title>
</svelte:head>

{#if isSignIn}
	{@render children()}
{:else if $current && canCurate($current.account)}
	<div class="min-h-screen bg-stone-50 text-stone-900">
		<header class="border-b border-stone-200 bg-white">
			<div class="mx-auto flex max-w-6xl items-center gap-6 px-4 py-3">
				<a href="/" class="font-semibold tracking-tight">Invisible Museum</a>
				<nav class="flex flex-1 flex-wrap gap-1 text-sm">
					{#each nav as item (item.href)}
						<a
							href={item.href}
							class="rounded-md px-3 py-1.5 hover:bg-stone-100 {active(item.href) ? 'bg-stone-900 text-white hover:bg-stone-800' : ''}"
							>{item.label}</a
						>
					{/each}
				</nav>
				<div class="flex items-center gap-3 text-sm text-stone-600">
					<span title={$current.account.email}>{$current.account.display_name || $current.account.email}</span>
					<span class="rounded bg-stone-100 px-2 py-0.5 text-xs uppercase tracking-wide">{$current.account.role}</span>
					<button class="text-stone-500 hover:text-stone-900" onclick={signOut}>Sign out</button>
				</div>
			</div>
		</header>
		<main class="mx-auto max-w-6xl px-4 py-6">
			{@render children()}
		</main>
	</div>
{:else if $current}
	<div class="flex min-h-screen items-center justify-center bg-stone-50 p-6 text-center">
		<div class="max-w-md space-y-4">
			<h1 class="text-2xl font-semibold">This account needs the curator role</h1>
			<p class="text-stone-600">
				You are signed in as <strong>{$current.account.email}</strong> with the
				<strong>{$current.account.role}</strong> role. Ask an administrator to grant curator access, then
				sign in again.
			</p>
			<button class="rounded-md bg-stone-900 px-4 py-2 text-white" onclick={signOut}>Sign out</button>
		</div>
	</div>
{:else}
	<div class="flex min-h-screen items-center justify-center text-stone-500">Redirecting to sign in…</div>
{/if}
