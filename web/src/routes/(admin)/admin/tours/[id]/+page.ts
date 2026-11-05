import { error } from '@sveltejs/kit';
import { getTour, getCity, listPublishedPlaces, getTourRoute, listMoments } from '#lib/admin.ts';
import { ApiError } from '#lib/api/index.ts';

export async function load({ params }) {
	try {
		const tour = await getTour(params.id);
		const city = await getCity(tour.city_id);
		const places = await listPublishedPlaces(city.slug);
		const momentsByPlace: Record<string, Awaited<ReturnType<typeof listMoments>>['items']> = {};
		await Promise.all(
			tour.stops.map(async (s) => {
				momentsByPlace[s.place.id] = (await listMoments(s.place.id)).items.filter(
					(m) => m.status === 'published'
				);
			})
		);
		let route: Awaited<ReturnType<typeof getTourRoute>> | null = null;
		const firstMode = tour.modes?.[0] ?? 'walk';
		if (tour.routes?.[firstMode]?.status === 'ready') {
			route = await getTourRoute(tour.id, firstMode).catch(() => null);
		}
		return { tour, topics: city.topics, places: places.items, momentsByPlace, route };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) error(404, 'Tour not found');
		throw e;
	}
}
