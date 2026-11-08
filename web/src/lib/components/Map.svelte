<script lang="ts">
	import { onMount } from 'svelte';
	import { darkTheme } from '#lib/theme.ts';
	import 'maplibre-gl/dist/maplibre-gl.css';

	export interface Marker {
		lat: number;
		lng: number;
		label: string;
		/** Stop number shown in the pin; plain pins when omitted. */
		position?: number;
		visited?: boolean;
		next?: boolean;
	}

	export interface Leg {
		coords: [number, number][];
		state: 'past' | 'active' | 'future';
	}

	let {
		markers = [],
		line = [],
		legs = [],
		user = null,
		fit = 'all',
		height = '20rem',
		onpoint = null
	}: {
		markers?: Marker[];
		line?: [number, number][];
		legs?: Leg[];
		user?: { lat: number; lng: number } | null;
		fit?: 'all' | 'ahead';
		height?: string;
		onpoint?: ((point: { lat: number; lng: number }) => void) | null;
	} = $props();

	let container: HTMLDivElement;
	let map: import('maplibre-gl').Map | null = null;
	let lib: typeof import('maplibre-gl') | null = null;
	let markerHandles: import('maplibre-gl').Marker[] = [];
	let userHandle: import('maplibre-gl').Marker | null = null;
	let fitted = '';
	let ready = $state(false);
	let styleDark = false;

	function token(role: string) {
		return getComputedStyle(document.documentElement)
			.getPropertyValue('--byl-' + role)
			.trim();
	}

	function fitTo(points: [number, number][]) {
		if (!map || !lib || points.length === 0) return;
		if (points.length === 1) {
			map.jumpTo({ center: points[0], zoom: 15 });
			return;
		}
		const b = new lib.LngLatBounds(points[0], points[0]);
		for (const p of points) b.extend(p);
		map.fitBounds(b, {
			padding: 48,
			maxZoom: 16,
			duration: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 180
		});
	}

	function pinElement(m: Marker): HTMLElement {
		const el = document.createElement('div');
		const bg = token(m.visited ? 'success-surface' : m.next ? 'action' : 'background');
		const fg = token(m.visited ? 'success' : m.next ? 'action-text' : 'text');
		const size = m.next ? 36 : 32;
		el.style.cssText = `width:${size}px;height:${size}px;border-radius:50%;background:${bg};color:${fg};border:2px solid ${token('border-control')};display:flex;align-items:center;justify-content:center;font:700 13px system-ui`;
		el.textContent = m.visited ? '✓' : m.position != null ? String(m.position) : '';
		el.setAttribute('aria-label', m.label);
		return el;
	}

	function render() {
		if (!map || !lib) return;
		map.getCanvas().style.cursor = onpoint ? 'crosshair' : '';
		for (const h of markerHandles) h.remove();
		markerHandles = markers.map((m) =>
			(m.position != null
				? new lib!.Marker({ element: pinElement(m) })
				: new lib!.Marker({ color: token('action') })
			)
				.setLngLat([m.lng, m.lat])
				.setPopup(new lib!.Popup({ offset: 24 }).setText(m.label))
				.addTo(map!)
		);
		userHandle?.remove();
		userHandle = null;
		if (user) {
			const el = document.createElement('div');
			el.style.cssText = `width:16px;height:16px;border-radius:50%;background:${token('info')};border:3px solid ${token('info-surface')};`;
			el.setAttribute('aria-label', 'You are here');
			userHandle = new lib.Marker({ element: el }).setLngLat([user.lng, user.lat]).addTo(map);
		}
		const allLegs: Leg[] = legs.length
			? legs
			: line.length
				? [{ coords: line, state: 'active' }]
				: [];
		for (const state of ['past', 'future', 'active'] as const) {
			const id = `leg-${state}`;
			const data = {
				type: 'FeatureCollection' as const,
				features: allLegs
					.filter((l) => l.state === state)
					.map((l) => ({
						type: 'Feature' as const,
						properties: {},
						geometry: { type: 'LineString' as const, coordinates: l.coords }
					}))
			};
			const src = map.getSource(id) as import('maplibre-gl').GeoJSONSource | undefined;
			if (src) {
				src.setData(data);
				map.setPaintProperty(
					id,
					'line-color',
					token(state === 'active' ? 'route' : 'text-secondary')
				);
				map.setPaintProperty(id + '-halo', 'line-color', token('route-halo'));
			} else {
				map.addSource(id, { type: 'geojson', data });
				map.addLayer({
					id: id + '-halo',
					type: 'line',
					source: id,
					layout: { 'line-cap': 'round', 'line-join': 'round' },
					paint: { 'line-color': token('route-halo'), 'line-width': 8 }
				});
				map.addLayer({
					id,
					type: 'line',
					source: id,
					layout: { 'line-cap': 'round', 'line-join': 'round' },
					paint: {
						'line-color': token(state === 'active' ? 'route' : 'text-secondary'),
						'line-width': 4,
						'line-opacity': state === 'past' ? 0.5 : 0.9,
						...(state === 'future' ? { 'line-dasharray': [1.5, 2] } : {})
					}
				});
			}
		}
		const ahead = markers.filter((m) => !m.visited);
		const pts: [number, number][] = [
			...(fit === 'ahead' && ahead.length ? ahead : markers).map(
				(m) => [m.lng, m.lat] as [number, number]
			),
			...(fit === 'all' ? allLegs.flatMap((l) => l.coords) : []),
			...(user && fit === 'ahead' ? [[user.lng, user.lat] as [number, number]] : [])
		];
		const key = JSON.stringify(pts);
		if (key !== fitted) {
			fitted = key;
			fitTo(pts);
		}
	}

	onMount(() => {
		let cancelled = false;
		(async () => {
			lib = await import('maplibre-gl');
			if (cancelled) return;
			map = new lib.Map({
				container,
				style: $darkTheme
					? 'https://tiles.openfreemap.org/styles/dark'
					: 'https://tiles.openfreemap.org/styles/liberty',
				center: [-88.0112, 41.8089],
				zoom: 13,
				attributionControl: { compact: true }
			});
			map.on('click', (event) => onpoint?.({ lat: event.lngLat.lat, lng: event.lngLat.lng }));
			map.addControl(new lib.NavigationControl({ showCompass: false }));
			styleDark = $darkTheme;
			map.on('style.load', () => {
				ready = true;
				render();
			});
		})();
		return () => {
			cancelled = true;
			map?.remove();
			map = null;
		};
	});

	$effect(() => {
		const dark = $darkTheme;
		if (map && ready && styleDark !== dark) {
			styleDark = dark;
			ready = false;
			map.setStyle(
				dark
					? 'https://tiles.openfreemap.org/styles/dark'
					: 'https://tiles.openfreemap.org/styles/liberty'
			);
		}
		void markers;
		void line;
		void legs;
		void user;
		void onpoint;
		if (map && ready) render();
	});
</script>

<div
	bind:this={container}
	class="w-full overflow-hidden border border-byl-border"
	style="height: {height}"
></div>
