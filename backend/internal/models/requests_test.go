package models

import "testing"

func stops(n int) []StopInput {
	out := make([]StopInput, n)
	for i := range out {
		out[i] = StopInput{PlaceID: "00000000-0000-4000-8000-00000000b0b1"}
	}
	return out
}

func TestCreateUserTourRequestValidate(t *testing.T) {
	base := CreateUserTourRequest{City: "downers-grove", Title: "My loop", Stops: stops(2)}
	if f := base.Validate(); len(f) != 0 {
		t.Fatalf("expected valid, got %v", f)
	}
	one := base
	one.Stops = stops(1)
	if f := one.Validate(); f["stops"] == "" {
		t.Fatalf("expected stops error for 1 stop, got %v", f)
	}
	many := base
	many.Stops = stops(31)
	if f := many.Validate(); f["stops"] == "" {
		t.Fatalf("expected stops error for 31 stops, got %v", f)
	}
	pub := base
	pub.Visibility = "public"
	if f := pub.Validate(); f["visibility"] == "" {
		t.Fatalf("user routes cannot be public, got %v", f)
	}
	untitled := base
	untitled.Title = "  "
	if f := untitled.Validate(); f["title"] == "" {
		t.Fatalf("expected title error, got %v", f)
	}
}

func TestProgressRequestValidate(t *testing.T) {
	ok := ProgressRequest{Mode: "walk", VisitedPositions: []int{1, 2}}
	if f := ok.Validate(); len(f) != 0 {
		t.Fatalf("expected valid, got %v", f)
	}
	bad := ProgressRequest{Mode: "fly"}
	if f := bad.Validate(); f["mode"] == "" {
		t.Fatalf("expected mode error, got %v", f)
	}
}

func TestEnumsValid(t *testing.T) {
	if !EntityStatusPublished.Valid() || EntityStatus("nope").Valid() {
		t.Fatal("entity status")
	}
	if !TravelModeBike.Valid() || TravelMode("fly").Valid() {
		t.Fatal("travel mode")
	}
	if !AccountRoleAdmin.Valid() || AccountRole("god").Valid() {
		t.Fatal("role")
	}
}

func TestCreatePlaceRequestValidate(t *testing.T) {
	ok := CreatePlaceRequest{CityID: "c", Name: "Fishel Park", Latitude: 41.79, Longitude: -88.01}
	if f := ok.Validate(); len(f) != 0 {
		t.Fatalf("expected valid, got %v", f)
	}
	bad := CreatePlaceRequest{CityID: "c", Name: "", Latitude: 95, Longitude: -88.01}
	f := bad.Validate()
	if f["name"] == "" || f["latitude"] == "" {
		t.Fatalf("expected name and latitude errors, got %v", f)
	}
}
