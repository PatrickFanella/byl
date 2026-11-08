import type { RouteSuggestion } from '#lib/visitor.ts';

export const suggestionStatuses: Record<
	RouteSuggestion['status'],
	{ label: string; detail: string }
> = {
	draft: {
		label: 'Received',
		detail: 'Your suggestion is saved privately for curator triage. Research has not started yet.'
	},
	processing: {
		label: 'Research in progress',
		detail: 'Research is being prepared. Check back for an update.'
	},
	needs_research: {
		label: 'More research needed',
		detail: 'Some questions or sources still need investigation.'
	},
	review: { label: 'Curator review', detail: 'The proposal is waiting for a curator decision.' },
	accepted: {
		label: 'Accepted',
		detail:
			'Your suggestion was accepted. Only content approved through curator publication appears as a public tour.'
	},
	declined: {
		label: 'Declined',
		detail: 'This suggestion is not being taken forward. Read the decision below.'
	}
};

export const suggestionFields = [
	{
		key: 'topic',
		label: 'Topic',
		max: 300,
		required: true,
		help: 'A theme, people, event or question you would like to explore.'
	},
	{
		key: 'area',
		label: 'Area',
		max: 500,
		required: true,
		help: 'A neighborhood, town or region. You do not need an exact location.'
	},
	{
		key: 'notes',
		label: 'Research notes',
		max: 10000,
		help: 'Unverified context or questions. These notes stay private.'
	},
	{
		key: 'place_leads',
		label: 'Place leads',
		max: 40000,
		help: 'One name or address per line, up to 20. Unknown locations are welcome.'
	},
	{
		key: 'event_leads',
		label: 'Event leads',
		max: 40000,
		help: 'One event or question per line, up to 20.'
	},
	{
		key: 'source_links',
		label: 'Source links',
		max: 40000,
		help: 'One HTTP or HTTPS link per line, up to 20. Links are research leads, not verified citations.'
	}
] as const;

export type SuggestionValues = Record<(typeof suggestionFields)[number]['key'], string>;

export const emptySuggestion = (): SuggestionValues => ({
	topic: '',
	area: '',
	notes: '',
	place_leads: '',
	event_leads: '',
	source_links: ''
});

export const suggestionLines = (value: string) =>
	value
		.split('\n')
		.map((line) => line.trim())
		.filter(Boolean);
