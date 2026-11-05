import { listCandidates, listSources } from '#lib/admin.ts';

export async function load({ url }) {
	const f = {
		status: url.searchParams.get('status') ?? 'pending',
		kind: url.searchParams.get('kind') ?? '',
		origin: url.searchParams.get('origin') ?? '',
		source: url.searchParams.get('source') ?? '',
		cursor: url.searchParams.get('cursor') ?? '',
		limit: 50
	};
	const [page, sources] = await Promise.all([listCandidates(f), listSources()]);
	return {
		filters: f,
		candidates: page.items,
		nextCursor: page.nextCursor,
		sources: sources.items
	};
}
