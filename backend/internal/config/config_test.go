package config

import (
	"strings"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	t.Setenv("TRANSIT_PROVIDER", "")
	t.Setenv("SCHEDULED_TRANSIT_ENABLED", "")
	t.Setenv("HAPPENINGS_IMPORT_ENABLED", "")
	for _, k := range []string{"API_ADDR", "DATABASE_URL", "ROUTING_PROVIDER", "NARRATION_PROVIDER", "MEDIA_STORE", "JOB_WORKERS", "SESSION_TTL_HOURS", "APPLE_CLIENT_IDS", "ADMIN_BOOTSTRAP_EMAILS", "DEV_AUTH_ENABLED", "PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED", "LLM_PROVIDER", "BRIEF_DISCOVERY_PROVIDERS", "BRIEF_DISCOVERY_SEED_URLS", "ROUTE_CONTRIBUTIONS_ENABLED", "ROUTE_CONTRIBUTION_SCOPE_KEY", "ROUTE_CONTRIBUTION_CUSTODY_APPROVED"} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
	}
}

func TestHappeningsImportDefaultsOff(t *testing.T) {
	setEnv(t, map[string]string{"DATABASE_URL": "postgres://fixture"})
	cfg, err := Load()
	if err != nil || cfg.HappeningsImport {
		t.Fatal("happenings import activated by default", err)
	}
	t.Setenv("HAPPENINGS_IMPORT_ENABLED", "true")
	cfg, err = Load()
	if err != nil || !cfg.HappeningsImport {
		t.Fatal("explicit happenings import gate not read", err)
	}
}

func TestScheduledTransitDefaultsOffAndRequiresConfiguredProvider(t *testing.T) {
	for _, tc := range []struct {
		provider, enabled string
		valid, want       bool
	}{
		{"", "", true, false}, {"cta", "", true, false}, {"", "true", false, false}, {"cta", "true", true, true}, {"unknown", "", false, false},
	} {
		setEnv(t, map[string]string{"DATABASE_URL": "postgres://fixture", "TRANSIT_PROVIDER": tc.provider, "SCHEDULED_TRANSIT_ENABLED": tc.enabled})
		cfg, err := Load()
		if (err == nil) != tc.valid || tc.valid && cfg.ScheduledTransit != tc.want {
			t.Fatalf("transit configuration %q/%q: %v", tc.provider, tc.enabled, err)
		}
	}
}

func TestContributionCollectionDefaultsOffAndRequiresCustodyAndScopeKey(t *testing.T) {
	setEnv(t, map[string]string{"DATABASE_URL": "postgres://fixture"})
	cfg, err := Load()
	if err != nil || cfg.RouteContributions || len(cfg.ContributionScopeKey) != 0 {
		t.Fatalf("default collection gate: %v", err)
	}
	for _, tc := range []struct {
		name, key, custody string
		valid              bool
	}{
		{"missing", "", "", false},
		{"no custody", strings.Repeat("ab", 32), "", false},
		{"no key", "", "true", false},
		{"malformed key", "fixture-invalid", "true", false},
		{"reviewed isolated test", strings.Repeat("ab", 32), "true", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, map[string]string{"DATABASE_URL": "postgres://fixture", "ROUTE_CONTRIBUTIONS_ENABLED": "true", "ROUTE_CONTRIBUTION_SCOPE_KEY": tc.key, "ROUTE_CONTRIBUTION_CUSTODY_APPROVED": tc.custody})
			cfg, err := Load()
			if (err == nil) != tc.valid {
				t.Fatalf("gate validity expected %v", tc.valid)
			}
			if tc.valid && (!cfg.RouteContributions || len(cfg.ContributionScopeKey) != 32) {
				t.Fatal("reviewed test gate did not load")
			}
		})
	}
}

func TestDiscoveryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, providers, seeds string
		valid                  bool
	}{
		{"defaults", "", "", true},
		{"configured", "loc, wikipedia_search", "https://history.example/", true},
		{"unknown", "url", "", false},
		{"repeated", "loc,loc", "", false},
		{"excessive", "loc,wikipedia_search,internet_archive,loc", "", false},
		{"private", "loc", "http://127.0.0.1/history", false},
		{"credentials", "loc", "https://user:secret@history.example/", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setEnv(t, map[string]string{"DATABASE_URL": "postgres://x", "BRIEF_DISCOVERY_PROVIDERS": tc.providers, "BRIEF_DISCOVERY_SEED_URLS": tc.seeds})
			cfg, err := Load()
			if (err == nil) != tc.valid {
				t.Fatalf("policy: %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret") {
				t.Fatal("configuration secret leaked")
			}
			if tc.name == "defaults" && (len(cfg.DiscoveryProviders) != 3 || len(cfg.DiscoverySeedURLs) != 3) {
				t.Fatal("discovery defaults missing")
			}
		})
	}
}

func TestLoadDefaults(t *testing.T) {
	setEnv(t, map[string]string{"DATABASE_URL": "postgres://x"})
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIAddr != ":8080" || cfg.RoutingProvider != "straightline" || cfg.NarrationProvider != "none" || cfg.MediaStore != "fs" || cfg.JobWorkers != 2 || cfg.SessionTTLHours != 2160 || cfg.DevAuthEnabled {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.MediaFSDir != "./data/media" {
		t.Fatalf("media dir default %q", cfg.MediaFSDir)
	}
}

func TestLoadRequiresDatabaseURL(t *testing.T) {
	setEnv(t, nil)
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got %v", err)
	}
}

func TestLoadRejectsUnknownProvider(t *testing.T) {
	setEnv(t, map[string]string{"DATABASE_URL": "postgres://x", "ROUTING_PROVIDER": "bogus"})
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "ROUTING_PROVIDER") {
		t.Fatalf("expected ROUTING_PROVIDER error, got %v", err)
	}
}

func TestLoadParsesLists(t *testing.T) {
	setEnv(t, map[string]string{
		"DATABASE_URL":           "postgres://x",
		"APPLE_CLIENT_IDS":       "a.b.c, d.e.f ,",
		"ADMIN_BOOTSTRAP_EMAILS": "Admin@Example.com",
		"DEV_AUTH_ENABLED":       "true",
		"LLM_PROVIDER":           "openai",
	})
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AppleClientIDs) != 2 || cfg.AppleClientIDs[1] != "d.e.f" {
		t.Fatalf("apple ids %v", cfg.AppleClientIDs)
	}
	if !cfg.IsBootstrapAdmin("admin@example.com") {
		t.Fatal("bootstrap email comparison should be case-insensitive")
	}
	if !cfg.DevAuthEnabled || cfg.LLMProvider != "openai" {
		t.Fatalf("flags %+v", cfg)
	}
}

func TestLoadSourcesSettings(t *testing.T) {
	setEnv(t, map[string]string{"DATABASE_URL": "postgres://x"})
	t.Setenv("LOCAL_ARCHIVE_URLS", "https://a.example/one, https://b.example/two")
	t.Setenv("SOURCES_USER_AGENT", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.LocalArchiveURLs) != 2 || cfg.LocalArchiveURLs[1] != "https://b.example/two" {
		t.Fatalf("urls %v", cfg.LocalArchiveURLs)
	}
	if !strings.HasPrefix(cfg.SourcesUserAgent, "InvisibleMuseum/") {
		t.Fatalf("ua %q", cfg.SourcesUserAgent)
	}
}

func TestPublicBriefIntakeIsExplicitAndDefaultOff(t *testing.T) {
	for _, tc := range []struct {
		value   string
		enabled bool
	}{{"", false}, {"false", false}, {"true", true}, {"not-enabled", false}} {
		t.Run(tc.value, func(t *testing.T) {
			setEnv(t, map[string]string{"DATABASE_URL": "postgres://x", "PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED": tc.value})
			cfg, err := Load()
			if err != nil || cfg.PublicBriefIntake != tc.enabled {
				t.Fatalf("intake gate %v %v", cfg.PublicBriefIntake, err)
			}
		})
	}
}

func TestLoadRejectsUnusableSessionTTL(t *testing.T) {
	for _, value := range []string{"0", "3000000"} {
		setEnv(t, map[string]string{"DATABASE_URL": "postgres://x", "SESSION_TTL_HOURS": value})
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_TTL_HOURS") {
			t.Fatalf("SESSION_TTL_HOURS=%s: expected an error, got %v", value, err)
		}
	}
}
