import { listCities, listTours, getCity } from '#lib/admin.ts';

export async function load({ url }) {
	const cities = await listCities();
	const city = url.searchParams.get('city') ?? cities.items[0]?.slug ?? '';
	const [tours, detail] = city
		? await Promise.all([listTours(city), getCity(city)])
		: [{ items: [], nextCursor: '' }, null];
	return {
		cities: cities.items,
		city,
		tours: tours.items,
		topics: detail?.topics ?? [],
		cityId: detail?.id ?? ''
	};
}
