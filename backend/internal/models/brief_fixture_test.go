package models

import (
	"encoding/json"
	"os"
	"testing"
)

func TestSharedRouteSuggestionFixture(t *testing.T) {
	raw, err := os.ReadFile("../../api/fixtures/route-briefs-v1/brief-response.json")
	if err != nil {
		t.Fatal(err)
	}
	var brief RouteBrief
	if err := json.Unmarshal(raw, &brief); err != nil {
		t.Fatal(err)
	}
	if brief.ID == "" || brief.OwnerAccountID == "" || brief.Status != BriefNeedsResearch || brief.Version != 3 || !brief.CreatedAt.Before(brief.UpdatedAt) || len(brief.Submitted.Validate()) != 0 {
		t.Fatal("shared suggestion fixture does not match the private brief contract")
	}
}
