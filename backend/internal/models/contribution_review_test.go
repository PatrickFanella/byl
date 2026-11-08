package models

import "testing"

func TestContributionReviewThresholdBoundaries(t *testing.T) {
	for _, c := range []struct {
		name                                    string
		scopes, departing, passages, departures int
		want                                    bool
	}{
		{"exact share", 5, 3, 10, 3, true}, {"below share", 5, 3, 11, 3, false},
		{"repeat visitor", 1, 1, 5, 5, false}, {"low independent sample", 4, 3, 5, 3, false},
		{"low departure sample", 5, 2, 5, 3, false}, {"missing passages", 5, 3, 0, 0, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := ContributionReview{ContributorScopes: c.scopes, DepartureScopes: c.departing, QualifiedPassages: c.passages, DepartureRejoins: c.departures}
			if r.MeetsThreshold() != c.want {
				t.Fatal("incorrect threshold decision")
			}
		})
	}
}
