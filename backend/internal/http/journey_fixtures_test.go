package httpapi

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestJourneySharedClientFixtures(t *testing.T) {
	root := filepath.Join("..", "..", "api", "fixtures", "route-journeys-v1")
	var tour models.TourDetail
	raw, err := os.ReadFile(filepath.Join(root, "tour-partial.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &tour); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"route-return.json", "route-failed.json", "route-pending.json", "route-unreviewed.json", "tour-partial.json"} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, native := range []string{filepath.Join("..", "..", "..", "android", "core", "src", "test", "resources", "fixtures"), filepath.Join("..", "..", "..", "ios-invisible-museum", "InvisibleMuseumKit", "Tests", "InvisibleMuseumKitTests", "Fixtures")} {
			copied, err := os.ReadFile(filepath.Join(native, "journey-"+name))
			if err != nil || string(copied) != string(raw) {
				t.Fatal("native fixture differs", native, name, err)
			}
		}
		if name == "tour-partial.json" {
			continue
		}
		var route models.TourRoute
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&route); err != nil {
			t.Fatal(err)
		}
		expected := route.Journey
		actual := models.DescribeJourney(&tour, &route)
		if !reflect.DeepEqual(actual, expected) {
			a, _ := json.Marshal(actual)
			b, _ := json.Marshal(expected)
			t.Fatalf("%s disagrees with contract fixture\nactual %s\nfixture %s", name, a, b)
		}
	}
	if tour.Journeys[models.TravelModeWalk].State != "usable" || tour.Journeys[models.TravelModeBike].State != "failed" || tour.Journeys[models.TravelModeBike].TravelDurationS != nil {
		t.Fatal("partial mode contract lost")
	}
}
