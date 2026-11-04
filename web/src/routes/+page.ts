import { getStats, listJobs, listCandidates } from '#lib/admin.ts';

export async function load() {
	const [stats, failed, recent] = await Promise.all([
		getStats(),
		listJobs('failed', 5),
		listCandidates({ status: 'pending', limit: 8 })
	]);
	return { stats, failedJobs: failed.items, pending: recent.items };
}
