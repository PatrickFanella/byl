import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

// Shared route links are {origin}/t/{slug}; the tour page shows the route and the app link.
export const load: PageLoad = ({ params }) => {
	redirect(307, `/tours/${params.slug}`);
};
