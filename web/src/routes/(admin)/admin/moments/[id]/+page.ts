import { error } from '@sveltejs/kit';
import { getMoment, getPlace, getCity, listMedia } from '#lib/admin.ts';
import { ApiError } from '#lib/api/index.ts';

export async function load({ params }) {
	try {
		const moment = await getMoment(params.id);
		const place = await getPlace(moment.place_id);
		const [city, media] = await Promise.all([getCity(place.city_id), listMedia(100)]);
		return { moment, place, topics: city.topics, library: media.items };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) error(404, 'Moment not found');
		throw e;
	}
}
