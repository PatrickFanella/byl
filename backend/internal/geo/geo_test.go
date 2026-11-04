package geo

import "testing"

func TestDistanceMetersStationToTivoli(t *testing.T) {
	d := DistanceMeters(41.7959, -88.0092, 41.7952, -88.0099)
	if d < 90 || d > 110 {
		t.Fatalf("expected roughly 100 m, got %.1f", d)
	}
}

func TestBoundingBoxContainsRadius(t *testing.T) {
	b := BoundingBox(41.7959, -88.0092, 300)
	if b.MinLat >= 41.7959 || b.MaxLat <= 41.7959 || b.MinLng >= -88.0092 || b.MaxLng <= -88.0092 {
		t.Fatalf("box does not surround center: %+v", b)
	}
	// 300 m is about 0.0027 degrees of latitude.
	if span := b.MaxLat - b.MinLat; span < 0.005 || span > 0.006 {
		t.Fatalf("unexpected latitude span %.5f", span)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Fishel Park!":            "fishel-park",
		"  Main   Street Station": "main-street-station",
		"Tivoli Théâtre (1928)":   "tivoli-theatre-1928",
		"---":                     "",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRandomSlug(t *testing.T) {
	a, b := RandomSlug(10), RandomSlug(10)
	if len(a) != 10 || len(b) != 10 {
		t.Fatalf("lengths %d %d", len(a), len(b))
	}
	if a == b {
		t.Fatal("two random slugs matched")
	}
	for _, r := range a {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') {
			t.Fatalf("unexpected rune %q", r)
		}
	}
}
