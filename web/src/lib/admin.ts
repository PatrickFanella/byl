// Small typed helpers over the admin API used by several pages.
import { api, qs, type Schemas } from '#lib/api/index.ts';

export type Candidate = Schemas['Candidate'];
export type Job = Schemas['Job'];
export type City = Schemas['City'];
export type Stats = Schemas['Stats'];
export type SourceInfo = {
	name: string;
	description: string;
	supports_query: boolean;
	supports_near: boolean;
};

export const listCities = () => api.list<City>('/api/v1/cities');
export const listSources = () => api.list<SourceInfo>('/api/v1/admin/sources');
export const getStats = () => api.get<Stats>('/api/v1/admin/stats');
export const listJobs = (status = '', limit = 50, offset = 0) =>
	api.list<Job>(`/api/v1/admin/jobs${qs({ status, limit, offset })}`);
export const retryJob = (id: string) => api.post<Job>(`/api/v1/admin/jobs/${id}/retry`);
export const listCandidates = (f: {
	status?: string;
	kind?: string;
	origin?: string;
	source?: string;
	cursor?: string;
	limit?: number;
}) => api.list<Candidate>(`/api/v1/admin/candidates${qs({ ...f })}`);
export const getCandidate = (id: string) => api.get<Candidate>(`/api/v1/admin/candidates/${id}`);
export const approveCandidate = (id: string, payload: unknown, note = '') =>
	api.post<Candidate>(`/api/v1/admin/candidates/${id}/approve`, { note, payload });
export const rejectCandidate = (id: string, note: string) =>
	api.post<Candidate>(`/api/v1/admin/candidates/${id}/reject`, { note });
export const draftCandidate = (id: string) =>
	api.post<{ job: Job }>(`/api/v1/admin/candidates/${id}/draft`);
export const createImport = (body: Record<string, unknown>) =>
	api.post<{ job: Job }>('/api/v1/admin/imports', body);

export function sleep(ms: number) {
	return new Promise((r) => setTimeout(r, ms));
}
