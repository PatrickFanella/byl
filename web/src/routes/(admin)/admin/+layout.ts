import { error, redirect } from '@sveltejs/kit';
import { get } from 'svelte/store';
import { session, canCurate } from '#lib/auth.ts';
import type { LayoutLoad } from './$types';

// Gate the whole admin group before any page loader runs admin-only calls: signed-out visitors
// go to sign-in, other roles get the curator notice (rendered by +error.svelte).
export const load: LayoutLoad = async ({ parent, url }) => {
	await parent();
	const current = get(session.store);
	if (!current) {
		redirect(307, `/signin?next=${encodeURIComponent(url.pathname)}`);
	}
	if (!canCurate(current.account)) {
		error(403, 'This account needs the curator role');
	}
	return {};
};
