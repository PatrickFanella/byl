<script lang="ts">
	import { onMount } from 'svelte';
	import 'maplibre-gl/dist/maplibre-gl.css';

	export interface Marker {
		lat: number;
		lng: number;
		label: string;
	}

	let {
		markers = [],
		line = [],
		height = '20rem'
	}: { markers?: Marker[]; line?: [number, number][]; height?: string } = $props();

	let container: HTMLDivElement;
	let map: import('maplibre-gl').Map | null = null;
	let lib: typeof import('maplibre-gl') | null = null;
	let markerHandles: import('maplibre-gl').Marker[] = [];

	function fit() {
		if (!map || !lib) return;
		const pts: [number, number][] = [
			...markers.map((m) => [m.lng, m.lat] as [number, number]),
			...line
		];
		if (pts.length === 0) return;
		if (pts.length === 1) {
			map.jumpTo({ center: pts[0], zoom: 15 });
			return;
		}
		const b = new lib.LngLatBounds(pts[0], pts[0]);
		for (const p of pts) b.extend(p);
		map.fitBounds(b, { padding: 40, maxZoom: 16, duration: 0 });
	}

	function render() {
		if (!map || !lib) return;
		for (const h of markerHandles) h.remove();
		markerHandles = markers.map((m) =>
			new lib!.Marker({ color: '#1c1917' })
				.setLngLat([m.lng, m.lat])
				.setPopup(new lib!.Popup({ offset: 24 }).setText(m.label))
				.addTo(map!)
		);
		const src = map.getSource('route') as import('maplibre-gl').GeoJSONSource | undefined;
		const data = {
			type: 'Feature' as const,
			properties: {},
			geometry: { type: 'LineString' as const, coordinates: line }
		};
		if (src) src.setData(data);
		else {
			map.addSource('route', { type: 'geojson', data });
			map.addLayer({
				id: 'route',
				type: 'line',
				source: 'route',
				paint: { 'line-color': '#4f46e5', 'line-width': 4, 'line-opacity': 0.8 }
			});
		}
		fit();
	}

	onMount(() => {
		let cancelled = false;
		(async () => {
			lib = await import('maplibre-gl');
			if (cancelled) return;
			map = new lib.Map({
				container,
				style: 'https://tiles.openfreemap.org/styles/liberty',
				center: [-88.0112, 41.8089],
				zoom: 13,
				attributionControl: { compact: true }
			});
			map.addControl(new lib.NavigationControl({ showCompass: false }));
			map.on('load', render);
		})();
		return () => {
			cancelled = true;
			map?.remove();
			map = null;
		};
	});

	$effect(() => {
		void markers;
		void line;
		if (map && map.loaded()) render();
	});
</script>

<div
	bind:this={container}
	class="w-full overflow-hidden rounded-xl border border-stone-200"
	style="height: {height}"
></div>
