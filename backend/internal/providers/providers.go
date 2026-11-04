// Package providers selects provider implementations from configuration.
package providers

import (
	"fmt"

	"invisible-museum/backend/internal/blob"
	"invisible-museum/backend/internal/config"
	"invisible-museum/backend/internal/llm"
	"invisible-museum/backend/internal/narration"
	"invisible-museum/backend/internal/routing"
	"invisible-museum/backend/internal/sources"
)

// Providers are the external dependencies the API and worker use.
type Providers struct {
	Router   routing.Router
	Narrator narration.Narrator
	Blob     blob.Store
	// MediaFSDir is set when the fs store is in use so the API can serve files.
	MediaFSDir string
	Sources    *sources.Registry
	Drafter    llm.Drafter
}

// Build constructs providers from config.
func Build(cfg config.Config) (Providers, error) {
	var p Providers
	switch cfg.RoutingProvider {
	case "valhalla":
		p.Router = routing.NewValhalla(cfg.ValhallaURL, nil)
	default:
		p.Router = routing.NewStraightLine()
	}
	switch cfg.NarrationProvider {
	case "openai":
		p.Narrator = narration.NewOpenAI(cfg.NarrationBaseURL, cfg.NarrationAPIKey, cfg.NarrationModel, cfg.NarrationVoice, nil)
	default:
		p.Narrator = narration.None{}
	}
	switch cfg.LLMProvider {
	case "anthropic":
		if cfg.LLMAPIKey == "" {
			return p, fmt.Errorf("LLM_PROVIDER=anthropic requires LLM_API_KEY")
		}
		p.Drafter = llm.NewAnthropic(cfg.LLMAPIKey, cfg.LLMBaseURL, cfg.LLMModel)
	case "openai":
		p.Drafter = llm.NewOpenAI(cfg.LLMBaseURL, cfg.LLMAPIKey, cfg.LLMModel, nil)
	default:
		p.Drafter = llm.None{}
	}
	httpClient := sources.NewHTTPClient(cfg.SourcesUserAgent)
	httpClient.AllowPrivateHosts = cfg.SourcesAllowPrivateHosts
	p.Sources = sources.NewRegistry(httpClient, cfg.LocalArchiveURLs)
	switch cfg.MediaStore {
	case "s3":
		s, err := blob.NewS3(blob.S3Config{Endpoint: cfg.S3Endpoint, Bucket: cfg.S3Bucket, AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, UseSSL: cfg.S3UseSSL, PublicBase: cfg.MediaPublicBaseURL})
		if err != nil {
			return p, err
		}
		p.Blob = s
	default:
		fs, err := blob.NewFS(cfg.MediaFSDir, cfg.MediaPublicBaseURL)
		if err != nil {
			return p, fmt.Errorf("media directory: %w", err)
		}
		p.Blob = fs
		p.MediaFSDir = fs.Dir()
	}
	return p, nil
}
