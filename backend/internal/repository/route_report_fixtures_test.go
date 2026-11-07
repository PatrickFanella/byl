package repository

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"invisible-museum/backend/internal/models"
)

func TestRouteReportClientFixtures(t *testing.T) {
	decode := func(path string, target any) {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join("..", "..", "api", "fixtures", path))
		if err != nil {
			t.Fatal(err)
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err := d.Decode(target); err != nil {
			t.Fatal(err)
		}
	}
	var revision models.TourRevision
	decode("tour-revisions-v1/revision-1.json", &revision)
	var minimal models.RouteReportInput
	for _, name := range []string{"minimal-request.json", "alternative-request.json"} {
		var in models.RouteReportInput
		decode("route-reports-v1/"+name, &in)
		in.Normalize()
		if len(in.Validate()) != 0 || validateReportLocation(&revision, in) != nil {
			t.Fatalf("fixture %s does not match its revision", name)
		}
		if name == "minimal-request.json" {
			minimal = in
		}
	}
	var report models.RouteReport
	decode("route-reports-v1/report-response.json", &report)
	if !reflect.DeepEqual(minimal, report.Submitted) || report.Status != models.BriefReview || report.Version != 1 {
		t.Fatal("normalized private response does not match the minimal request")
	}
}
