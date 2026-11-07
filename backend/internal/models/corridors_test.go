package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func corridorFixture(t *testing.T) CorridorIntent {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "fixtures", "corridors-v1", "intent.json"))
	if err != nil {
		t.Fatal(err)
	}
	var in CorridorIntent
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestCorridorImportKeepsExplicitBranchesAndJunctions(t *testing.T) {
	if f := corridorFixture(t).Validate(); len(f) > 0 {
		t.Fatal(f)
	}
	for name, change := range map[string]func(*CorridorIntent){
		"wrong branch": func(c *CorridorIntent) { c.Routes[0].Branch = "aurora" },
		"adjacent road": func(c *CorridorIntent) {
			c.Routes[0].Path[1].SegmentID = "adjacent-road"
			c.Routes[0].AllowedConnectors = []string{"adjacent-road"}
		},
		"separated crossing": func(c *CorridorIntent) { c.Routes[0].Path[1].SegmentID = "upper-crossing" },
		"connector not approved": func(c *CorridorIntent) {
			c.Routes[0].Path = append(c.Routes[0].Path, CorridorTraversal{SegmentID: "connector"})
		},
		"connector missing provenance": func(c *CorridorIntent) { c.Segments[3].Source = nil },
		"mode not allowed":             func(c *CorridorIntent) { c.Segments[1].Modes = []TravelMode{TravelModeBike} },
		"malformed dimensions":         func(c *CorridorIntent) { c.Segments[0].Geometry.Coordinates[0] = []float64{1, 2, 3} },
		"wrong point occurrence":       func(c *CorridorIntent) { c.Routes[0].ShapingPoints[0].TraversalIndex = 0 },
		"off-reference point": func(c *CorridorIntent) {
			lat := 0.0
			c.Routes[0].ShapingPoints[0].Latitude = &lat
		},
		"source version missing": func(c *CorridorIntent) { c.Source.Version = "" },
		"too many points": func(c *CorridorIntent) {
			for range 5000 {
				c.Segments[0].Geometry.Coordinates = append(c.Segments[0].Geometry.Coordinates, []float64{-88.0093, 41.7956})
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			in := corridorFixture(t)
			change(&in)
			if f := in.Validate(); len(f) == 0 {
				t.Fatal("invalid selection accepted")
			}
		})
	}
	in := corridorFixture(t)
	in.Routes[0].AllowedConnectors = []string{"connector"}
	in.Routes[0].Path = append(in.Routes[0].Path, CorridorTraversal{SegmentID: "connector"})
	if f := in.Validate(); len(f) > 0 {
		t.Fatalf("explicit connector rejected: %v", f)
	}
	in = corridorFixture(t)
	in.Routes[0].Direction = "reverse"
	in.Routes[0].Path = []CorridorTraversal{{SegmentID: "elgin", Reverse: true}, {SegmentID: "main", Reverse: true}}
	in.Routes[0].ShapingPoints = nil
	if f := in.Validate(); len(f) > 0 {
		t.Fatalf("explicit reverse path rejected: %v", f)
	}
}
