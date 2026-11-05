import { error } from '@sveltejs/kit';
import { getPlace, listMoments } from '#lib/admin.ts';
import { ApiError } from '#lib/api/index.ts';

export async function load({ params }) {
	try {
		const place = await getPlace(params.id);
		const moments = await listMoments(place.id);
		return { place, moments: moments.items };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) error(404, 'Place not found');
		throw e;
	}
}
