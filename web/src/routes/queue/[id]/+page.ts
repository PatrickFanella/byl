import { error } from '@sveltejs/kit';
import { getCandidate } from '#lib/admin.ts';
import { ApiError } from '#lib/api/index.ts';

export async function load({ params }) {
	try {
		return { candidate: await getCandidate(params.id) };
	} catch (e) {
		if (e instanceof ApiError && e.status === 404) error(404, 'Candidate not found');
		throw e;
	}
}
