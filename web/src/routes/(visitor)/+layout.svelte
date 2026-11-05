<script lang="ts">
	import '../layout.css';
	import favicon from '#lib/assets/favicon.svg';
	import { page } from '$app/state';
	import { session, canCurate } from '#lib/auth.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	const current = session.store;

	const nav = [
		{ href: '/', label: 'Home' },
		{ href: '/tours', label: 'Tours' },
		{ href: '/routes', label: 'Routes' },
		{ href: '/saved', label: 'Saved' },
		{ href: '/profile', label: 'Profile' }
	];

	let isSignIn = $derived(page.url.pathname.startsWith('/signin'));
	let isWalk = $derived(/^\/tours\/[^/]+\/walk/.test(page.url.pathname));

	function active(href: string) {
		return href === '/' ? page.url.pathname === '/' : page.url.pathname.startsWith(href);
	}
</script>

<svelte:head>
	<link rel="icon" href={favicon} />
	<title>Invisible Museum</title>
</svelte:head>

{#if isSignIn}
	{@render children()}
{:else}
	<div class="gallery min-h-screen text-white">
		{#if !isWalk}
			<header class="border-b border-white/10">
				<div class="mx-auto flex max-w-5xl items-center gap-4 px-4 py-3">
					<a href="/" class="font-semibold tracking-tight">Invisible Museum</a>
					<nav class="flex flex-1 flex-wrap gap-1 text-sm">
						{#each nav as item (item.href)}
							<a
								href={item.href}
								class="rounded-full px-3 py-1.5 hover:bg-white/10 {active(item.href)
									? 'bg-white text-black hover:bg-white'
									: 'text-white/70'}">{item.label}</a
							>
						{/each}
					</nav>
					{#if $current && canCurate($current.account)}
						<a href="/admin" class="text-xs tracking-wide text-white/60 uppercase hover:text-white"
							>Admin</a
						>
					{/if}
				</div>
			</header>
		{/if}
		<main class={isWalk ? '' : 'mx-auto max-w-5xl px-4 py-6'}>
			{@render children()}
		</main>
	</div>
{/if}
