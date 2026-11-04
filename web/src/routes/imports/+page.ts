import { listCities, listSources, listJobs } from '#lib/admin.ts';

export async function load() {
	const [cities, sources, jobs] = await Promise.all([
		listCities(),
		listSources(),
		listJobs('', 100)
	]);
	return {
		cities: cities.items,
		sources: sources.items,
		jobs: jobs.items.filter((j) => j.kind === 'import_source')
	};
}
