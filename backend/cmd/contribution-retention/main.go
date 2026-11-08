// Command contribution-retention purges expired private records once, independent
// of collection flags, provider credentials or the active application revision.
package main

import (
	"context"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"invisible-museum/backend/internal/db"
	"invisible-museum/backend/internal/repository"
)

func main() {
	admit := flag.Bool("admit-custody", false, "record explicit operator custody approval; collection remains held for two UTC date boundaries")
	flag.Parse()
	var scopeKey []byte
	if *admit {
		decoded, err := hex.DecodeString(strings.TrimSpace(os.Getenv("ROUTE_CONTRIBUTION_SCOPE_KEY")))
		if err != nil || len(decoded) != 32 || !strings.EqualFold(strings.TrimSpace(os.Getenv("ROUTE_CONTRIBUTION_CUSTODY_APPROVED")), "true") {
			fmt.Fprintln(os.Stderr, "custody admission requires a 32-byte hex scope key and explicit ROUTE_CONTRIBUTION_CUSTODY_APPROVED=true")
			os.Exit(1)
		}
		scopeKey = decoded
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	pool, err := db.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "could not connect to the retention database")
		os.Exit(1)
	}
	defer pool.Close()
	store := repository.New(pool)
	if *admit {
		if err := store.AdmitContributionCustody(ctx, scopeKey, time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "custody admission failed; retain collection shutdown and drain prior consent records before rotating keys")
			os.Exit(1)
		}
		fmt.Println("Custody admission recorded; the existing admission date is retained on retries and collection requires a two-UTC-day hold")
		return
	}
	count, err := store.PurgeRouteContributions(ctx, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, "contribution retention failed")
		os.Exit(1)
	}
	fmt.Printf("Retention cleanup completed; direct rows removed: %d (dependent summaries cascade)\n", count)
}
