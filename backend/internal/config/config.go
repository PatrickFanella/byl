// Package config reads runtime settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds every runtime setting for the API and worker.
type Config struct {
	APIAddr       string
	DatabaseURL   string
	PublicBaseURL string

	DevAuthEnabled       bool
	AppleClientIDs       []string
	GoogleClientIDs      []string
	AdminBootstrapEmails []string
	SessionTTLHours      int

	RoutingProvider string
	ValhallaURL     string

	NarrationProvider string
	NarrationBaseURL  string
	NarrationAPIKey   string
	NarrationModel    string
	NarrationVoice    string

	MediaStore         string
	MediaFSDir         string
	MediaPublicBaseURL string
	S3Endpoint         string
	S3Bucket           string
	S3AccessKey        string
	S3SecretKey        string
	S3UseSSL           bool

	LLMProvider string
	LLMBaseURL  string
	LLMAPIKey   string
	LLMModel    string

	LocalArchiveURLs []string
	SourcesUserAgent string
	// SourcesAllowPrivateHosts lets media downloads reach private networks (development only).
	SourcesAllowPrivateHosts bool

	JobWorkers int
}

var (
	routingProviders   = []string{"straightline", "valhalla"}
	narrationProviders = []string{"none", "openai"}
	mediaStores        = []string{"fs", "s3"}
	llmProviders       = []string{"", "anthropic", "openai"}
)

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		APIAddr:            envOr("API_ADDR", ":8080"),
		DatabaseURL:        strings.TrimSpace(os.Getenv("DATABASE_URL")),
		PublicBaseURL:      strings.TrimRight(envOr("PUBLIC_BASE_URL", "http://localhost:8080"), "/"),
		DevAuthEnabled:     envBool("DEV_AUTH_ENABLED"),
		AppleClientIDs:     envList("APPLE_CLIENT_IDS"),
		GoogleClientIDs:    envList("GOOGLE_CLIENT_IDS"),
		RoutingProvider:    envOr("ROUTING_PROVIDER", "straightline"),
		ValhallaURL:        envOr("VALHALLA_URL", "http://localhost:8002"),
		NarrationProvider:  envOr("NARRATION_PROVIDER", "none"),
		NarrationBaseURL:   envOr("NARRATION_BASE_URL", "https://api.openai.com"),
		NarrationAPIKey:    strings.TrimSpace(os.Getenv("NARRATION_API_KEY")),
		NarrationModel:     envOr("NARRATION_MODEL", "gpt-4o-mini-tts"),
		NarrationVoice:     envOr("NARRATION_VOICE", "alloy"),
		MediaStore:         envOr("MEDIA_STORE", "fs"),
		MediaFSDir:         envOr("MEDIA_FS_DIR", "./data/media"),
		MediaPublicBaseURL: strings.TrimRight(strings.TrimSpace(os.Getenv("MEDIA_PUBLIC_BASE_URL")), "/"),
		S3Endpoint:         strings.TrimSpace(os.Getenv("S3_ENDPOINT")),
		S3Bucket:           strings.TrimSpace(os.Getenv("S3_BUCKET")),
		S3AccessKey:        strings.TrimSpace(os.Getenv("S3_ACCESS_KEY")),
		S3SecretKey:        strings.TrimSpace(os.Getenv("S3_SECRET_KEY")),
		S3UseSSL:           envBool("S3_USE_SSL"),
		LLMProvider:        strings.TrimSpace(os.Getenv("LLM_PROVIDER")),
		LLMBaseURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("LLM_BASE_URL")), "/"),
		LLMAPIKey:          strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		LLMModel:           strings.TrimSpace(os.Getenv("LLM_MODEL")),
		LocalArchiveURLs:   envList("LOCAL_ARCHIVE_URLS"),
		SourcesUserAgent:   envOr("SOURCES_USER_AGENT", "InvisibleMuseum/0.2 (+https://github.com/PatrickFanella/invisible-museum)"),

		SourcesAllowPrivateHosts: envBool("SOURCES_ALLOW_PRIVATE_HOSTS"),
	}
	for _, e := range envList("ADMIN_BOOTSTRAP_EMAILS") {
		cfg.AdminBootstrapEmails = append(cfg.AdminBootstrapEmails, strings.ToLower(e))
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	var err error
	if cfg.SessionTTLHours, err = envInt("SESSION_TTL_HOURS", 2160); err != nil {
		return Config{}, err
	}
	if cfg.JobWorkers, err = envInt("JOB_WORKERS", 2); err != nil {
		return Config{}, err
	}
	if err := oneOf("ROUTING_PROVIDER", cfg.RoutingProvider, routingProviders); err != nil {
		return Config{}, err
	}
	if err := oneOf("NARRATION_PROVIDER", cfg.NarrationProvider, narrationProviders); err != nil {
		return Config{}, err
	}
	if err := oneOf("MEDIA_STORE", cfg.MediaStore, mediaStores); err != nil {
		return Config{}, err
	}
	if err := oneOf("LLM_PROVIDER", cfg.LLMProvider, llmProviders); err != nil {
		return Config{}, err
	}
	if cfg.MediaStore == "s3" && (cfg.S3Endpoint == "" || cfg.S3Bucket == "") {
		return Config{}, fmt.Errorf("MEDIA_STORE=s3 requires S3_ENDPOINT and S3_BUCKET")
	}
	if cfg.MediaPublicBaseURL == "" {
		cfg.MediaPublicBaseURL = cfg.PublicBaseURL + "/media/files"
	}
	return cfg, nil
}

// IsBootstrapAdmin reports whether the email should receive the admin role on first sign-in.
func (c Config) IsBootstrapAdmin(email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	for _, e := range c.AdminBootstrapEmails {
		if e == email {
			return true
		}
	}
	return false
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func envInt(key string, def int) (int, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer", key)
	}
	return n, nil
}

func envList(key string) []string {
	var out []string
	for _, part := range strings.Split(os.Getenv(key), ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func oneOf(key, value string, allowed []string) error {
	for _, a := range allowed {
		if value == a {
			return nil
		}
	}
	return fmt.Errorf("%s must be one of %s", key, strings.Join(allowed, ", "))
}
