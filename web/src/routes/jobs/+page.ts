import { listJobs } from '#lib/admin.ts';

export async function load({ url }) {
	const status = url.searchParams.get('status') ?? '';
	const page = await listJobs(status, 100);
	return { status, jobs: page.items };
}
