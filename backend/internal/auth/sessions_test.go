package auth

import (
	"context"
	"testing"
	"time"

	"invisible-museum/backend/internal/models"
	"invisible-museum/backend/internal/repository"
	"invisible-museum/backend/internal/testdb"
)

func TestSessionsIssueResolveRevoke(t *testing.T) {
	store := repository.New(testdb.Open(t))
	ctx := context.Background()
	acct, err := store.UpsertAccount(ctx, models.AuthProviderDev, "s1", "s1@example.com", "", false)
	if err != nil {
		t.Fatal(err)
	}
	sessions := NewSessions(store, 2*time.Hour)
	token, expires, err := sessions.Issue(ctx, acct.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) < 40 || time.Until(expires) < 119*time.Minute {
		t.Fatalf("token %q expires %v", token, expires)
	}
	got, err := sessions.Resolve(ctx, token)
	if err != nil || got.ID != acct.ID {
		t.Fatalf("resolve: %v %+v", err, got)
	}
	if HashToken(token) == token {
		t.Fatal("hash must differ from token")
	}
	if err := sessions.Revoke(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Resolve(ctx, token); err == nil {
		t.Fatal("revoked token resolved")
	}
	if _, err := sessions.Resolve(ctx, "garbage"); err == nil {
		t.Fatal("garbage token resolved")
	}
}
