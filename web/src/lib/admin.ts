// Small typed helpers over the admin API used by several pages.
import { api, qs, ApiError, type Schemas } from '#lib/api/index.ts';

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
export const getJob = (id: string) => api.get<Job>(`/api/v1/admin/jobs/${encodeURIComponent(id)}`);
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
export const draftCandidate = (id: string, evidence_ids: string[]) =>
	api.post<{ job: Job }>(`/api/v1/admin/candidates/${id}/draft`, { evidence_ids });
export const createImport = (body: Record<string, unknown>) =>
	api.post<{ job: Job }>('/api/v1/admin/imports', body);

export type RouteBrief = Schemas['RouteBrief'];
export type RouteBriefInput = Schemas['RouteBriefInput'];
export type BriefStatus = Schemas['BriefStatus'];
export const listRouteBriefs = (status = '', offset = 0) =>
	api.list<RouteBrief>(`/api/v1/admin/route-briefs${qs({ status, offset, limit: 50 })}`);
export const getRouteBrief = (id: string) =>
	api.get<RouteBrief>(`/api/v1/admin/route-briefs/${encodeURIComponent(id)}`);
export const createRouteBrief = (submitted: RouteBriefInput, key: string) =>
	api.request<RouteBrief>('POST', '/api/v1/admin/route-briefs', submitted, false, {
		'Idempotency-Key': key
	});
export const updateRouteBrief = (id: string, version: number, submitted: RouteBriefInput) =>
	api.put<RouteBrief>(`/api/v1/admin/route-briefs/${encodeURIComponent(id)}`, {
		version,
		submitted
	});

export function sleep(ms: number) {
	return new Promise((r) => setTimeout(r, ms));
}

// Content
export type PlaceSummary = Schemas['PlaceSummary'];
export type PlaceDetail = Schemas['PlaceSummary'] & {
	primary_moment?: MomentSummary | null;
	moments: MomentSummary[];
	media: MediaAsset[];
};
export type MomentSummary = Schemas['MomentSummary'];
export type MomentDetail = MomentSummary & {
	body: string;
	media: MediaAsset[];
	sources: Source[];
	topics: Topic[];
	narration?: MediaAsset | null;
};
export type MediaAsset = Schemas['MediaAsset'];
export type Source = Schemas['Source'];
export type Topic = Schemas['Topic'];
export type TourSummary = Schemas['TourSummary'];
export type TourStop = Schemas['TourStop'];
export type TourDetail = TourSummary & { stops: TourStop[] };
export type TourRoute = Schemas['TourRoute'];

export const getCity = (ref: string) =>
	api.get<City & { topics: Topic[] }>(`/api/v1/cities/${ref}`);
export const listPlaces = (city: string, limit = 200) =>
	api.list<PlaceSummary>(`/api/v1/admin/places${qs({ city, limit })}`);
export const listPublishedPlaces = (city: string) =>
	api.list<PlaceSummary>(`/api/v1/cities/${city}/places${qs({ limit: 200 })}`);
export const getPlace = (id: string) => api.get<PlaceDetail>(`/api/v1/admin/places/${id}`);
export const createPlace = (body: Record<string, unknown>) =>
	api.post<PlaceDetail>('/api/v1/admin/places', body);
export const updatePlace = (id: string, body: Record<string, unknown>) =>
	api.patch<PlaceDetail>(`/api/v1/admin/places/${id}`, body);
export const setPlaceStatus = (id: string, action: 'publish' | 'unpublish') =>
	api.post<PlaceDetail>(`/api/v1/admin/places/${id}/${action}`);
export const getMoment = (id: string) => api.get<MomentDetail>(`/api/v1/admin/moments/${id}`);
export const createMoment = (body: Record<string, unknown>) =>
	api.post<MomentDetail>('/api/v1/admin/moments', body);
export const updateMoment = (id: string, body: Record<string, unknown>) =>
	api.patch<MomentDetail>(`/api/v1/admin/moments/${id}`, body);
export const setMomentStatus = (id: string, action: 'publish' | 'unpublish') =>
	api.post<MomentDetail>(`/api/v1/admin/moments/${id}/${action}`);
export const setMomentMedia = (id: string, media_ids: string[]) =>
	api.put<MomentDetail>(`/api/v1/admin/moments/${id}/media`, { media_ids });
export const setMomentSources = (id: string, sources: Source[]) =>
	api.put<MomentDetail>(`/api/v1/admin/moments/${id}/sources`, { sources });
export const setMomentTopics = (id: string, topic_ids: string[]) =>
	api.put<MomentDetail>(`/api/v1/admin/moments/${id}/topics`, { topic_ids });
export const listMedia = (limit = 100) =>
	api.list<MediaAsset>(`/api/v1/admin/media${qs({ limit })}`);
export const uploadMedia = (form: FormData) =>
	api.postForm<MediaAsset>('/api/v1/admin/media', form);
export const updateMedia = (id: string, body: Record<string, unknown>) =>
	api.patch<MediaAsset>(`/api/v1/admin/media/${id}`, body);
export const listTours = (city: string) =>
	api.list<TourSummary>(`/api/v1/admin/tours${qs({ city, limit: 200 })}`);
export const getTour = (id: string) => api.get<TourDetail>(`/api/v1/admin/tours/${id}`);
export const createTour = (body: Record<string, unknown>) =>
	api.post<TourDetail>('/api/v1/admin/tours', body);
export const updateTour = (id: string, body: Record<string, unknown>) =>
	api.patch<TourDetail>(`/api/v1/admin/tours/${id}`, body);
export const setTourStops = (
	id: string,
	stops: { place_id: string; featured_moment_id?: string | null; note?: string }[]
) => api.put<TourDetail>(`/api/v1/admin/tours/${id}/stops`, { stops });
export const setTourStatus = (id: string, action: 'publish' | 'unpublish') =>
	api.post<TourDetail>(`/api/v1/admin/tours/${id}/${action}`);
export const recomputeRoutes = (id: string) =>
	api.post<{ status: string }>(`/api/v1/admin/tours/${id}/routes/recompute`);
export const deleteTour = (id: string) => api.del(`/api/v1/admin/tours/${id}`);
export const suggestFeatured = (id: string, position: number) =>
	api.post<{ ordered: { moment: MomentSummary; reason: string }[] }>(
		`/api/v1/admin/tours/${id}/stops/${position}/suggest`
	);
export const getTourRoute = (id: string, mode: string) =>
	api.get<TourRoute>(`/api/v1/tours/${id}/route${qs({ mode })}`);
export const listMoments = (placeId: string) =>
	api.list<MomentSummary>(`/api/v1/admin/moments${qs({ place: placeId })}`);

export function errorMessage(e: unknown, fallback: string): string {
	if (e instanceof ApiError) return e.message;
	return fallback;
}

export const createMediaLink = (body: {
	kind: 'video' | 'document';
	public_url: string;
	caption: string;
	credit: string;
	source_url: string;
}) => api.post<MediaAsset>('/api/v1/admin/media/links', body);
export const setPlaceMedia = (id: string, media_ids: string[]) =>
	api.put<PlaceDetail>(`/api/v1/admin/places/${id}/media`, { media_ids });

export type SourceEvidence = Schemas['SourceEvidence'];
export const listBriefEvidence = (id: string) =>
	api.list<SourceEvidence>(
		`/api/v1/admin/route-briefs/${encodeURIComponent(id)}/evidence?limit=50`
	);
export type HistoricalDraftCitation = { evidence_id: string; passage: string; quote: string };
export type HistoricalDraft = {
	title_citations: HistoricalDraftCitation[];
	date_citations: HistoricalDraftCitation[];
	title: string;
	body: string;
	date_start: string;
	date_end: string;
	date_precision: string;
	date_display: string;
	notes: string;
	topic_ids: string[];
	sources: Source[];
	review_gaps: string[];
	claims: {
		text: string;
		kind: string;
		attribution: string;
		citations: { evidence_id: string; passage: string; quote: string }[];
	}[];
	uncertainties: { kind: string; text: string; citations?: HistoricalDraftCitation[] }[];
	evidence_manifest: {
		id: string;
		url: string;
		title: string;
		retrieved_at: string;
		content_sha256: string;
		outcome: string;
	}[];
};

export type PathQualitySummary = Schemas['PathQualitySummary'];
export type PathQualityReport = Schemas['PathQualityReport'];
export const getPathQuality = (tour: string, mode: 'walk' | 'bike', report_id?: string) =>
	api.get<{ report: PathQualityReport | null }>(
		`/api/v1/admin/tours/${encodeURIComponent(tour)}/routes/quality${qs({ mode, report_id })}`
	);
