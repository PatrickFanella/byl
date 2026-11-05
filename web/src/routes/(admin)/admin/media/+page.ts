import { listMedia } from '#lib/admin.ts';

export async function load() {
	const media = await listMedia(200);
	return { media: media.items };
}
