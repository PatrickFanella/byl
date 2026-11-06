import { error } from '@sveltejs/kit';
import { getRouteBrief } from '#lib/admin.ts';
import { ApiError } from '#lib/api/index.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, params }) => {
	await parent();
	try {
		return { brief: await getRouteBrief(params.id) };
	} catch (err) {
		if (err instanceof ApiError && err.status === 404) error(404, 'Route brief not found');
		throw err;
	}
};
