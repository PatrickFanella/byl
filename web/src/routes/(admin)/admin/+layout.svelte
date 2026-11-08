<script lang="ts">
	import AppearancePicker from '#lib/components/AppearancePicker.svelte';
	import Brand from '#lib/components/Brand.svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { session, canCurate } from '#lib/auth.ts';
	import { api } from '#lib/api/index.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	const current = session.store;

	const nav = [
		{ href: '/admin', label: 'Dashboard' },
		{ href: '/admin/briefs', label: 'Route briefs' },
		{ href: '/admin/queue', label: 'Review queue' },
		{ href: '/admin/queue/reports', label: 'Route reports' },
		{ href: '/admin/imports', label: 'Imports' },
		{ href: '/admin/places', label: 'Places' },
		{ href: '/admin/media', label: 'Media' },
		{ href: '/admin/tours', label: 'Tours' },
		{ href: '/admin/jobs', label: 'Jobs' }
	];

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
		if (href === '/admin/queue' && page.url.pathname.startsWith('/admin/queue/reports'))
			return false;
		return href === '/admin' ? page.url.pathname === '/admin' : page.url.pathname.startsWith(href);
	}
</script>

<svelte:head>
	<title>Byl admin</title>
</svelte:head>

{#if $current && canCurate($current.account)}
	<div class="byl-curator min-h-screen bg-byl-background text-byl-text">
		<header class="border-b border-byl-border bg-byl-background">
			<div class="byl-header">
				<a href="/admin" aria-label="Byl home"><Brand /></a>
				<nav class="byl-nav text-sm" aria-label="Main navigation">
					{#each nav as item (item.href)}
						<a
							href={item.href}
							aria-current={active(item.href) ? 'page' : undefined}
							class="rounded-md px-3 py-1.5 hover:bg-byl-surface {active(item.href)
								? 'bg-byl-action text-byl-action-text hover:bg-byl-action'
								: ''}">{item.label}</a
						>
					{/each}
				</nav>
				<div class="flex items-center gap-3 text-sm text-byl-muted">
					<a href="/" class="text-byl-muted hover:text-byl-text">Visitor site</a>
					<span title={$current.account.email}
						>{$current.account.display_name || $current.account.email}</span
					>
					<span class="rounded bg-byl-surface px-2 py-0.5 text-xs tracking-wide"
						>{$current.account.role}</span
					>
					<button class="text-byl-muted hover:text-byl-text" onclick={signOut}>Sign out</button>
				</div>
			</div>
		</header>
		<main class="byl-main">
			<div class="mb-6"><AppearancePicker /></div>
			{@render children()}
		</main>
	</div>
{:else if $current}
	<div class="flex min-h-screen items-center justify-center bg-byl-background p-6 text-center">
		<div class="max-w-md space-y-4">
			<h1 class="text-2xl font-semibold">This account needs the curator role</h1>
			<p class="text-byl-muted">
				You are signed in as <strong>{$current.account.email}</strong> with the
				<strong>{$current.account.role}</strong> role. Ask an administrator to grant curator access, then
				sign in again.
			</p>
			<p><a class="underline" href="/">Back to the visitor site</a></p>
			<button class="rounded-md bg-byl-action px-4 py-2 text-byl-action-text" onclick={signOut}
				>Sign out</button
			>
		</div>
	</div>
{:else}
	<div class="flex min-h-screen items-center justify-center text-byl-muted">
		Redirecting to sign in…
	</div>
{/if}
