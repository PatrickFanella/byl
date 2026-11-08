//go:build e2e

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
)

// Called only after prepare's explicit isolated museum_test admission. The
// retained announcement is synthetic; no network or real-event claim is made.
func prepareHappening(ctx context.Context, pool *pgxpool.Pool, store *repository.Store, place string) error {
	now := time.Now().UTC()
	zone, err := time.LoadLocation("America/Chicago")
	if err != nil {
		return err
	}
	day := func(offset int) *models.Date {
		d, _ := models.ParseDate(now.In(zone).AddDate(0, 0, offset).Format("2006-01-02"))
		return d
	}
	actor, err := store.UpsertAccount(ctx, models.AuthProviderDev, "e2e-happening-curator", "e2e-happening@example.com", "", true)
	if err != nil {
		return err
	}
	brief, _, err := store.CreateRouteBrief(ctx, actor.ID, models.RouteBriefInput{Topic: "Synthetic current notice", Area: "Downers Grove"}, "")
	if err != nil {
		return err
	}
	sourceKey := sha256.Sum256([]byte("https://organizer.example/browser-announcement"))
	evidence, err := store.QueueSourceEvidence(ctx, brief.ID, models.EvidenceRequest{Version: 1, URL: "https://organizer.example/browser-announcement", SourceKind: "page"}, "page", hex.EncodeToString(sourceKey[:]), "e2e-happening-evidence")
	if err != nil {
		return err
	}
	quote := fmt.Sprintf("Fixture Parks announces the Synthetic browser history talk at Main Street Station on %s. This is a synthetic fixture, not a real event.", day(0).Format("2006-01-02"))
	digest := sha256.Sum256([]byte(quote))
	raw, err := json.Marshal(models.EvidenceResult{Outcome: "inspected_page", FinalURL: evidence.URL, Title: "Synthetic organizer announcement", Publisher: "Fixture Parks", RetrievedAt: now, HTTPStatus: 200, ContentType: "text/html", ContentSHA256: hex.EncodeToString(digest[:]), Passages: []models.EvidencePassage{{Reference: "paragraph:1", Text: quote}}, Untrusted: true, MediaReuse: "unresolved"})
	if err != nil {
		return err
	}
	if _, err = pool.Exec(ctx, `UPDATE source_evidence SET result=$2 WHERE id=$1`, evidence.ID, raw); err != nil {
		return err
	}
	// Retire only this fixture's queued retrieval before starting the worker.
	if _, err = pool.Exec(ctx, `UPDATE jobs SET status='done',finished_at=now() WHERE id=$1 AND status='queued'`, evidence.JobID); err != nil {
		return err
	}
	claims := []models.HappeningCitation{}
	for _, field := range []string{"title", "summary", "organizer", "occurrence", "place"} {
		claims = append(claims, models.HappeningCitation{Field: field, EvidenceID: evidence.ID, Passage: "paragraph:1", Quote: quote})
	}
	draft, err := store.SaveHappeningDraft(ctx, "", actor.ID, "e2e-happening-draft", models.HappeningWriteRequest{Input: models.HappeningInput{PlaceID: place, Title: "Synthetic browser history talk", Summary: "This is a synthetic fixture, not a real event.", Organizer: "Fixture Parks", StartsOn: day(0), EndsOn: day(0), State: "scheduled", ReviewOn: day(1), ExpiresOn: day(2), EvidenceIDs: []string{evidence.ID}, Claims: claims}}, now)
	if err != nil {
		return err
	}
	pkg, err := store.GetHappeningReviewPackage(ctx, draft.ID, now)
	if err != nil {
		return err
	}
	if !pkg.ReadyToPublish {
		return fmt.Errorf("synthetic happening package not ready: %+v", pkg.Tasks)
	}
	_, err = store.ReviewHappening(ctx, draft.ID, actor.ID, models.HappeningReviewRequest{Version: pkg.Version, Action: "approve", Note: "Synthetic browser contract fixture only"}, now)
	return err
}
