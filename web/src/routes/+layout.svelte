<script lang="ts">
	import './layout.css';
	import { onMount } from 'svelte';
	import { startTheme } from '#lib/theme.ts';
	import type { LayoutProps } from './$types';
	let { children }: LayoutProps = $props();
	onMount(startTheme);
</script>

<svelte:head>
	<link rel="stylesheet" href="/brand/tokens.css" />
	<link rel="icon" href="/brand/favicon.svg" type="image/svg+xml" />
	<link rel="icon" href="/brand/favicon-32.png" sizes="32x32" />
	<link rel="apple-touch-icon" href="/brand/apple-touch-icon.png" />
	<link rel="manifest" href="/manifest.webmanifest" />
	<meta
		name="description"
		content="Byl — local history, on foot or by bike. Explore tours, place stories and their sources."
	/>
	<meta property="og:site_name" content="Byl" />
	<meta property="og:image" content="/brand/link-preview.png" />
</svelte:head>

{@render children()}
