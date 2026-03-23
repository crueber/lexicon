package metadata

import (
	"context"
	"log/slog"
)

// AudibleProvider is a placeholder provider for Audible.
// Audible does not provide a public API and aggressively blocks scrapers.
// This stub satisfies the Provider interface for future implementation.
type AudibleProvider struct {
	logger *slog.Logger
}

// NewAudibleProvider creates a new AudibleProvider.
func NewAudibleProvider(logger *slog.Logger) *AudibleProvider {
	return &AudibleProvider{logger: logger}
}

// Name returns the provider name.
func (p *AudibleProvider) Name() string {
	return "audible"
}

// Search returns empty results. Audible does not provide a public API.
// Scraping is blocked aggressively.
func (p *AudibleProvider) Search(_ context.Context, _ Query) ([]Result, error) {
	p.logger.Debug("audible provider not implemented: no public API available")
	return nil, nil
}

// FetchByID returns nil. Audible does not provide a public API.
func (p *AudibleProvider) FetchByID(_ context.Context, _ string) (*Result, error) {
	return nil, nil
}
