package reconcile

import (
	"slices"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestConservativeHistoricalIdentity(t *testing.T) {
	lat, lng := 41.79, -88.01
	places := []models.IdentityPlace{
		{ID: "cemetery", CityID: "city", Name: "Main Street Cemetery", Address: "5190 Main Street", Lat: lat, Lng: lng, Moments: []models.IdentityMoment{{ID: "burial", Title: "Early burials", DateStart: "1850-01-01", DatePrecision: models.DatePrecisionYear, SourceURLs: []string{"https://archive.example/cemetery"}}}},
		{ID: "greatwestern", CityID: "city", Name: "Great Western depot", Address: "100 Depot Street", Lat: 41.8, Lng: -88.02},
	}
	base := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: "Main Street Cemetery", URL: "https://archive.example/cemetery", Identity: &models.IdentityFacts{Address: "5190 Main Street"}}
	for _, tc := range []struct {
		name           string
		edit           func(*models.DiscoveryItem)
		claims         []models.IdentityClaim
		outcome, issue string
	}{
		{name: "existing place", outcome: "existing_place"},
		{name: "existing moment", edit: func(i *models.DiscoveryItem) {
			i.Identity.MomentTitle = "Early burials"
			i.Identity.DateStart = "1850-06-02"
			i.Identity.DatePrecision = models.DatePrecisionDay
		}, outcome: "existing_place_existing_moment"},
		{name: "new moment", edit: func(i *models.DiscoveryItem) { i.Identity.MomentTitle = "Preservation campaign" }, outcome: "existing_place_new_moment"},
		{name: "Downer burial ground unconfirmed", edit: func(i *models.DiscoveryItem) {
			i.Title = "Downer burial ground"
			i.Identity.Aliases = []string{"Main Street Cemetery"}
		}, outcome: "needs_review", issue: "unconfirmed_alias"},
		{name: "Downer burial ground confirmed", edit: func(i *models.DiscoveryItem) { i.Title = "Downer burial ground" }, claims: []models.IdentityClaim{{PlaceID: "cemetery", Aliases: []string{"Downer burial ground"}}}, outcome: "existing_place"},
		{name: "CA&E is not Great Western", edit: func(i *models.DiscoveryItem) { i.Title = "CA&E depot"; i.Identity.Address = "100 Depot Street" }, outcome: "needs_review", issue: "nearby_distinct_identity"},
		{name: "historical street number", edit: func(i *models.DiscoveryItem) { i.Identity.Address = "90 Main Street" }, outcome: "needs_review", issue: "address_conflict"},
		{name: "moved building", edit: func(i *models.DiscoveryItem) { i.Identity.Moved = true }, outcome: "needs_review", issue: "moved_building"},
		{name: "conflicting date", edit: func(i *models.DiscoveryItem) {
			i.Identity.MomentTitle = "Early burials"
			i.Identity.DateStart = "1860-01-01"
			i.Identity.DatePrecision = models.DatePrecisionYear
		}, outcome: "needs_review", issue: "date_conflict"},
		{name: "name without context", edit: func(i *models.DiscoveryItem) { i.Identity.Address = "" }, outcome: "needs_review", issue: "name_without_corroboration"},
		{name: "new place", edit: func(i *models.DiscoveryItem) {
			i.Title = "Independent factory"
			i.Identity.Address = "900 Factory Street"
			a, b := 41.81, -88.04
			i.Lat = &a
			i.Lng = &b
		}, outcome: "new_place"},
		{name: "coordinates alone", edit: func(i *models.DiscoveryItem) {
			i.Title = "Different site"
			i.Identity.Address = ""
			i.Lat = &lat
			i.Lng = &lng
		}, outcome: "needs_review", issue: "nearby_distinct_identity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			i := base
			f := *base.Identity
			i.Identity = &f
			if tc.edit != nil {
				tc.edit(&i)
			}
			p := Propose(i, "city", places, tc.claims, true)
			if p.Outcome != tc.outcome || (tc.issue != "" && !slices.Contains(p.Issues, tc.issue)) {
				t.Fatalf("got %+v", p)
			}
		})
	}
	for _, tc := range []struct {
		city     string
		complete bool
		issue    string
	}{{"", true, "unresolved_city"}, {"city", false, "catalogue_limit"}} {
		p := Propose(base, tc.city, places, nil, tc.complete)
		if p.Outcome != "needs_review" || !slices.Contains(p.Issues, tc.issue) {
			t.Fatalf("unsafe catalogue %+v", p)
		}
	}
}

func TestAmbiguousNamesAndUnknownEntity(t *testing.T) {
	i := models.DiscoveryItem{Kind: models.CandidateKindPlace, Title: "Depot", Identity: &models.IdentityFacts{Address: "1 Main"}}
	p := []models.IdentityPlace{{ID: "a", CityID: "city", Name: "Depot", Address: "1 Main"}, {ID: "b", CityID: "city", Name: "Depot", Address: "1 Main"}}
	if got := Propose(i, "city", p, nil, true); !slices.Contains(got.Issues, "ambiguous_place") {
		t.Fatalf("%+v", got)
	}
	i.EntityID = "missing"
	if got := Propose(i, "city", nil, nil, true); !slices.Contains(got.Issues, "unknown_entity") {
		t.Fatalf("%+v", got)
	}
}
