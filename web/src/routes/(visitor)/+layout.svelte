<script lang="ts">
	import Brand from '#lib/components/Brand.svelte';
	import { page } from '$app/state';
	import { session, canCurate } from '#lib/auth.ts';
	import type { LayoutProps } from './$types';

	let { children }: LayoutProps = $props();
	const current = session.store;
	let navigation: HTMLElement | undefined = $state();
	let navHeight = $state(80);
	$effect(() => {
		if (!navigation) return;
		const element = navigation;
		const observer = new ResizeObserver(() => {
			navHeight = element.getBoundingClientRect().height;
		});
		observer.observe(element);
		return () => observer.disconnect();
	});

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
	<title>Byl · Local history, on foot or by bike</title>
</svelte:head>

{#if isSignIn}
	{@render children()}
{:else}
	<div
		class="byl-visitor gallery min-h-screen text-byl-text"
		style:--byl-nav-clearance={`${navHeight}px`}
	>
		{#if !isWalk}
			<header class="border-b border-byl-border">
				<div class="byl-header">
					<a href="/" aria-label="Byl home"><Brand /></a>
					<nav bind:this={navigation} class="byl-nav text-sm" aria-label="Main navigation">
						{#each nav as item (item.href)}
							<a href={item.href} aria-current={active(item.href) ? 'page' : undefined}
								>{item.label}</a
							>
						{/each}
					</nav>
					{#if $current && canCurate($current.account)}
						<a href="/admin" class="text-xs tracking-wide text-byl-muted hover:text-byl-text"
							>Admin</a
						>
					{/if}
				</div>
			</header>
		{/if}
		<main class={isWalk ? '' : 'byl-main'}>
			{@render children()}
		</main>
	</div>
{/if}
