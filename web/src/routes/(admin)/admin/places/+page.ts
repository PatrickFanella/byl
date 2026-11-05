import { listCities, listPlaces } from '#lib/admin.ts';

export async function load({ url }) {
	const cities = await listCities();
	const city = url.searchParams.get('city') ?? cities.items[0]?.slug ?? '';
	const places = city ? await listPlaces(city) : { items: [], nextCursor: '' };
	return { cities: cities.items, city, places: places.items };
}
