import { error } from '@sveltejs/kit';
import { getPlace, listMoments, listMedia } from '#lib/admin.ts';
import { ApiError } from '#lib/api/index.ts';

export async function load({ params }) {
	try {
		const place = await getPlace(params.id);
		const moments = await listMoments(place.id);
		const library = await listMedia(200);
		return { place, moments: moments.items, library: library.items };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) error(404, 'Place not found');
		throw e;
	}
}
