import { listRouteBriefs } from '#lib/admin.ts';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ parent, url }) => {
	await parent();
	const status = url.searchParams.get('status') ?? '';
	const offset = Math.max(0, Number(url.searchParams.get('offset')) || 0);
	return { briefs: (await listRouteBriefs(status, offset)).items, status, offset };
};
