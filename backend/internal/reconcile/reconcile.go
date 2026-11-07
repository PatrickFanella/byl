// Package reconcile proposes conservative identities without changing collection records.
package reconcile

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"invisible-museum/backend/internal/geo"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/sources"
)

// Key produces a stable identity key from normalized, delimited components.
func Key(parts ...string) string {
	for i := range parts {
		parts[i] = sources.DiscoveryTerms(parts[i])
	}
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(h[:])
}

func equal(a, b string) bool {
	normalized := sources.DiscoveryTerms(a)
	return normalized != "" && normalized == sources.DiscoveryTerms(b)
}

// Propose keeps name collisions, address changes and date disagreements reviewable.
// Coordinates corroborate names; they never establish identity by themselves.
func Propose(item models.DiscoveryItem, cityID string, places []models.IdentityPlace, claims []models.IdentityClaim, complete bool) models.ReconciliationProposal {
	f := models.IdentityFacts{}
	if item.Identity != nil {
		f = *item.Identity
	}
	name := f.PlaceName
	if name == "" && item.Kind == models.CandidateKindPlace {
		name = item.Title
	}
	title := f.MomentTitle
	if title == "" && item.Kind == models.CandidateKindMoment {
		title = item.Title
	}
	p := models.ReconciliationProposal{Item: item, CityID: cityID, Outcome: "needs_review", Issues: []string{}}
	p.PlaceKey = Key("place", cityID, name, f.Address)
	p.Key = Key("lead", cityID, item.Area, name, f.Address, title, item.URL)
	issue := func(code string) {
		for _, s := range p.Issues {
			if s == code {
				return
			}
		}
		p.Issues = append(p.Issues, code)
	}
	if cityID == "" {
		issue("unresolved_city")
	}
	if !complete {
		issue("catalogue_limit")
	}
	if f.Moved {
		issue("moved_building")
	}
	if len(f.HistoricalAddresses) > 0 {
		issue("historical_address")
	}
	if f.DateStart != "" {
		if d, err := models.ParseDate(f.DateStart); err != nil || d == nil || !f.DatePrecision.Valid() {
			issue("unresolved_date")
		}
	}
	var matches []models.IdentityPlace
	nearbyDistinct := false
	directFound := item.EntityID == ""
	for _, place := range places {
		if cityID != "" && place.CityID != cityID {
			continue
		}
		direct := item.Kind == models.CandidateKindPlace && item.EntityID == place.ID
		for _, m := range place.Moments {
			if item.Kind == models.CandidateKindMoment && item.EntityID == m.ID {
				direct = true
			}
		}
		if direct {
			directFound = true
		}
		alias := false
		for _, c := range claims {
			if c.PlaceID == place.ID {
				for _, a := range c.Aliases {
					if equal(name, a) {
						alias = true
					}
				}
				for _, a := range c.HistoricalAddresses {
					if equal(f.Address, a) {
						issue("historical_address")
					}
				}
			}
		}
		named := equal(name, place.Name) || alias
		if direct && name != "" && !named {
			issue("identity_name_conflict")
		}
		// Proposed aliases are untrusted and require review until a curator confirms them.
		for _, a := range f.Aliases {
			if equal(a, place.Name) && !named {
				issue("unconfirmed_alias")
			}
		}
		address := equal(f.Address, place.Address)
		near := item.Lat != nil && item.Lng != nil && geo.DistanceMeters(*item.Lat, *item.Lng, place.Lat, place.Lng) <= 75
		sourcedMoment := false
		for _, m := range place.Moments {
			if equal(title, m.Title) && sameSource(item.URL, m.SourceURLs) {
				sourcedMoment = true
			}
		}
		if direct || named {
			if f.Address != "" && place.Address != "" && !address {
				issue("address_conflict")
			}
			if item.Lat != nil && item.Lng != nil && !near {
				issue("coordinate_conflict")
			}
			if direct || (named && (alias || address || near || sourcedMoment)) {
				matches = append(matches, place)
			} else {
				issue("name_without_corroboration")
			}
		} else if address || near {
			nearbyDistinct = true
		}
	}
	if !directFound {
		issue("unknown_entity")
	}
	if len(matches) == 0 && nearbyDistinct {
		issue("nearby_distinct_identity")
	}
	if len(matches) > 1 {
		issue("ambiguous_place")
	}
	if len(matches) == 1 {
		place := matches[0]
		p.PlaceID = place.ID
		p.CityID = place.CityID
		p.PlaceKey = Key("place_id", place.ID)
		p.Key = Key(p.PlaceKey, title)
		p.Outcome = "existing_place"
		if title != "" || item.Kind == models.CandidateKindMoment {
			var moments []models.IdentityMoment
			for _, m := range place.Moments {
				direct := item.Kind == models.CandidateKindMoment && item.EntityID == m.ID
				if direct || equal(title, m.Title) {
					if dateConflict(f, m) {
						issue("date_conflict")
					}
					moments = append(moments, m)
				}
			}
			switch len(moments) {
			case 0:
				p.Outcome = "existing_place_new_moment"
			case 1:
				p.Outcome = "existing_place_existing_moment"
				p.MomentID = moments[0].ID
			default:
				issue("ambiguous_moment")
			}
		}
	} else if len(matches) == 0 && len(p.Issues) == 0 {
		if sources.DiscoveryTerms(name) == "" || sources.DiscoveryTerms(f.Address) == "" || item.Lat == nil || item.Lng == nil || item.URL == "" {
			issue("incomplete_place_identity")
		} else {
			p.Outcome = "new_place"
			p.Key = Key(p.PlaceKey, title)
		}
	}
	if len(p.Issues) > 0 {
		p.Outcome = "needs_review"
	}
	return p
}

func dateConflict(f models.IdentityFacts, m models.IdentityMoment) bool {
	if f.DateStart == "" || m.DateStart == "" {
		return false
	}
	// Compare only the precision both sources actually claim. Circa dates remain reviewable.
	if f.DatePrecision == models.DatePrecisionCirca || m.DatePrecision == models.DatePrecisionCirca {
		return f.DateStart != m.DateStart
	}
	n := 10
	if f.DatePrecision == models.DatePrecisionYear || m.DatePrecision == models.DatePrecisionYear {
		n = 4
	} else if f.DatePrecision == models.DatePrecisionMonth || m.DatePrecision == models.DatePrecisionMonth {
		n = 7
	}
	return len(f.DateStart) < n || len(m.DateStart) < n || f.DateStart[:n] != m.DateStart[:n]
}

// ConflictingDates compares two source claims at their shared precision.
func ConflictingDates(first, second models.IdentityFacts) bool {
	return dateConflict(first, models.IdentityMoment{DateStart: second.DateStart, DatePrecision: second.DatePrecision})
}

func sameSource(u string, urls []string) bool {
	_, _, key, err := sources.NormalizeEvidenceURL(u)
	if err != nil {
		return false
	}
	for _, other := range urls {
		if _, _, k, e := sources.NormalizeEvidenceURL(other); e == nil && k == key {
			return true
		}
	}
	return false
}
