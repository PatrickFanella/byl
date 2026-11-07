package models

import "invisible-museum/backend/internal/geo"

// DiscoveryOrigin explains why a source was shortlisted without asserting a historical fact.
type DiscoveryOrigin struct {
	Provider string   `json:"provider"`
	Query    string   `json:"query"`
	URL      string   `json:"url"`
	Reasons  []string `json:"reasons"`
}

// DiscoveryItem is a private lead for subsequent evidence review and reconciliation.
type DiscoveryItem struct {
	Kind     CandidateKind     `json:"kind"`
	EntityID string            `json:"entity_id,omitempty"`
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	URL      string            `json:"url"`
	Lat      *float64          `json:"lat,omitempty"`
	Lng      *float64          `json:"lng,omitempty"`
	Area     string            `json:"area,omitempty"`
	Evidence string            `json:"evidence"`
	Origins  []DiscoveryOrigin `json:"origins"`
	Identity *IdentityFacts    `json:"identity,omitempty"`
}

// DiscoverySource retains a bounded retrieval snapshot, including unsuccessful access.
type DiscoverySource struct {
	URL    string         `json:"url"`
	Via    string         `json:"via"`
	Result EvidenceResult `json:"result"`
}

// DiscoveryProvider records partial failure using safe codes rather than raw errors.
type DiscoveryProvider struct {
	Name    string `json:"name"`
	Query   string `json:"query"`
	Outcome string `json:"outcome"`
}

// DiscoveryReport is stored only in the private research checkpoint.
type DiscoveryReport struct {
	Items     []DiscoveryItem     `json:"items"`
	Sources   []DiscoverySource   `json:"sources"`
	Providers []DiscoveryProvider `json:"providers"`
	Gaps      []string            `json:"gaps"`
}

// DiscoveryScope supplies an optional known city boundary; absent coordinates are never invented.
type DiscoveryScope struct {
	CityID string
	BBox   geo.BBox
}
