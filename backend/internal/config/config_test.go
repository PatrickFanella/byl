package config

import (
	"strings"
	"testing"
)

func setEnv(t *testing.T, kv map[string]string) {
	t.Helper()
	for _, k := range []string{"API_ADDR", "DATABASE_URL", "ROUTING_PROVIDER", "NARRATION_PROVIDER", "MEDIA_STORE", "JOB_WORKERS", "SESSION_TTL_HOURS", "APPLE_CLIENT_IDS", "ADMIN_BOOTSTRAP_EMAILS", "DEV_AUTH_ENABLED", "PUBLIC_ROUTE_BRIEF_INTAKE_ENABLED", "LLM_PROVIDER", "BRIEF_DISCOVERY_PROVIDERS", "BRIEF_DISCOVERY_SEED_URLS"} {
		t.Setenv(k, "")
	}
	for k, v := range kv {
		t.Setenv(k, v)
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
