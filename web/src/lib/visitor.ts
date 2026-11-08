// Typed calls for the visitor site: public content plus the signed-in /me routes.
import { api } from '#lib/api/index.ts';
import type { components } from '#lib/api/types.ts';

type Schemas = components['schemas'];
export type City = Schemas['City'] & { topics?: Topic[] };
export type Topic = Schemas['Topic'];
export type PlaceSummary = Schemas['PlaceSummary'];
export type MomentSummary = Schemas['MomentSummary'];
export type MediaAsset = Schemas['MediaAsset'];
export type Source = Schemas['Source'];
export type TourSummary = Schemas['TourSummary'];
export type TourStop = Schemas['TourStop'];
export type TourRoute = Schemas['TourRoute'];
export type TourProgress = Schemas['TourProgress'];
export type RouteSummary = Schemas['RouteSummary'];
export type PlaceDetail = PlaceSummary & {
	primary_moment?: MomentSummary | null;
	moments: MomentSummary[];
	media: MediaAsset[];
};
export type MomentDetail = MomentSummary & {
	body: string;
	media: MediaAsset[];
	sources: Source[];
	topics: Topic[];
	narration?: MediaAsset | null;
};
export type TourDetail = Schemas['TourDetail'];
export type TourRevision = Schemas['TourRevision'];
export type RevisionProgress = Schemas['RevisionProgress'];
export type Account = Schemas['Account'];

function qs(params: Record<string, string | number | undefined | null>): string {
	const u = new URLSearchParams();
	for (const [k, v] of Object.entries(params)) {
		if (v !== undefined && v !== null && v !== '') u.set(k, String(v));
	}
	const s = u.toString();
	return s ? `?${s}` : '';
}

// Public
export const listCities = () => api.list<City>('/api/v1/cities');
export const getCity = (ref: string) => api.get<City>(`/api/v1/cities/${encodeURIComponent(ref)}`);
export const listTours = (city: string, topic?: string, mode?: string) =>
	api.list<TourSummary>(`/api/v1/cities/${encodeURIComponent(city)}/tours${qs({ topic, mode })}`);
export const getTour = (ref: string) =>
	api.get<TourDetail>(`/api/v1/tours/${encodeURIComponent(ref)}`);
export const getRoute = (tour: string, mode: string) =>
	api.get<TourRoute>(`/api/v1/tours/${encodeURIComponent(tour)}/route${qs({ mode })}`);
export const getTourRevision = (tour: string, revision: string) =>
	api.get<TourRevision>(
		`/api/v1/tours/${encodeURIComponent(tour)}/revisions/${encodeURIComponent(revision)}`
	);
export const listTourRevisions = (tour: string) =>
	api.list<TourRevision>(`/api/v1/tours/${encodeURIComponent(tour)}/revisions?limit=20`);
export const getRevisionProgress = (tour: string, revision: string) =>
	api.get<RevisionProgress>(
		`/api/v1/me/progress/${encodeURIComponent(tour)}/revisions/${encodeURIComponent(revision)}`
	);
export const putRevisionProgress = (
	tour: string,
	revision: string,
	body: { mode: 'walk' | 'bike'; visited_stop_ids: string[]; completed: boolean; reset?: boolean }
) =>
	api.put<RevisionProgress>(
		`/api/v1/me/progress/${encodeURIComponent(tour)}/revisions/${encodeURIComponent(revision)}`,
		{ ...body, reset: body.reset ?? false }
	);
export const listPlaces = async (city: string) => {
	const items: PlaceSummary[] = [];
	let cursor = '';
	do {
		const page = await api.list<PlaceSummary>(
			`/api/v1/cities/${encodeURIComponent(city)}/places${qs({ limit: 200, cursor })}`
		);
		items.push(...page.items);
		cursor = page.nextCursor || '';
	} while (cursor);
	return { items, nextCursor: '' };
};
export const listPlacesNear = (city: string, lat: number, lng: number, radiusM = 2000) =>
	api.list<PlaceSummary>(
		`/api/v1/cities/${encodeURIComponent(city)}/places${qs({ near: `${lat},${lng}`, radius_m: radiusM, limit: 50 })}`
	);
export const getPlace = (ref: string) =>
	api.get<PlaceDetail>(`/api/v1/places/${encodeURIComponent(ref)}`);
export const getMoment = (id: string) =>
	api.get<MomentDetail>(`/api/v1/moments/${encodeURIComponent(id)}`);

// Me
export const me = () => api.get<Account>('/api/v1/me');
export const mySaved = () => api.list<TourSummary>('/api/v1/me/saved');
export const saveTour = (id: string) =>
	api.put<unknown>(`/api/v1/me/saved/${encodeURIComponent(id)}`);
export const unsaveTour = (id: string) => api.del(`/api/v1/me/saved/${encodeURIComponent(id)}`);
export const myProgress = () => api.list<TourProgress>('/api/v1/me/progress');
export const putProgress = (
	tour: string,
	body: { mode: string; visited_positions: number[]; completed: boolean; reset?: boolean }
) => api.put<TourProgress>(`/api/v1/me/progress/${encodeURIComponent(tour)}`, body);
export const myTours = () => api.list<TourSummary>('/api/v1/me/tours');
export const createTour = (body: Record<string, unknown>) =>
	api.post<TourDetail>('/api/v1/me/tours', body);
export const updateTour = (id: string, body: Record<string, unknown>) =>
	api.patch<TourDetail>(`/api/v1/me/tours/${encodeURIComponent(id)}`, body);
export const deleteTour = (id: string) => api.del(`/api/v1/me/tours/${encodeURIComponent(id)}`);
