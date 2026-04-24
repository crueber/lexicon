package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/crueber/lexicon/internal/book"
	"github.com/crueber/lexicon/internal/library"
)

// ErrProposalNotFound is returned when a proposal does not exist.
var ErrProposalNotFound = errors.New("proposal not found")

// BroadcastBookUpdatedFunc broadcasts a BOOK_UPDATED WebSocket event.
type BroadcastBookUpdatedFunc func(bookID int64)

// BroadcastProposalReadyFunc broadcasts a METADATA_PROPOSAL_READY WebSocket event.
type BroadcastProposalReadyFunc func(proposalID int64)

// Service orchestrates metadata providers and manages proposals.
type Service struct {
	db                     *sql.DB
	providers              map[string]Provider
	logger                 *slog.Logger
	broadcastBookUpdated   BroadcastBookUpdatedFunc
	broadcastProposalReady BroadcastProposalReadyFunc
}

// NewService creates a new metadata Service.
func NewService(db *sql.DB, logger *slog.Logger) *Service {
	return &Service{
		db:        db,
		providers: make(map[string]Provider),
		logger:    logger,
	}
}

// WithBroadcastBookUpdatedFunc sets the broadcast function for book updates.
func (s *Service) WithBroadcastBookUpdatedFunc(fn BroadcastBookUpdatedFunc) {
	s.broadcastBookUpdated = fn
}

// WithBroadcastProposalReadyFunc sets the broadcast function for metadata proposal readiness.
func (s *Service) WithBroadcastProposalReadyFunc(fn BroadcastProposalReadyFunc) {
	s.broadcastProposalReady = fn
}

// RegisterProvider adds a provider to the registry.
func (s *Service) RegisterProvider(p Provider) {
	s.providers[p.Name()] = p
}

// Search searches registered providers for a book.
// If query.LibraryID is set, only providers configured for that library are searched.
func (s *Service) Search(ctx context.Context, query Query) (map[string][]Result, error) {
	results := make(map[string][]Result)

	providersToSearch := s.providers
	if query.LibraryID > 0 {
		lq := library.New(s.db)
		sources, err := lq.GetLibraryMetadataSources(ctx, query.LibraryID)
		if err != nil {
			s.logger.Warn("get library metadata sources failed", "library_id", query.LibraryID, "error", err)
			// Non-fatal: fall back to searching all providers.
		} else if len(sources) > 0 {
			filtered := make(map[string]Provider, len(sources))
			for _, src := range sources {
				if p, ok := s.providers[src.Provider]; ok {
					filtered[src.Provider] = p
				}
			}
			providersToSearch = filtered
		}
	}

	for name, p := range providersToSearch {
		providerResults, err := p.Search(ctx, query)
		if err != nil {
			s.logger.Warn("provider search failed",
				"provider", name,
				"error", err,
			)
			// Non-fatal: continue with other providers.
			continue
		}
		results[name] = providerResults
	}
	return results, nil
}

// CreateProposal saves a search result as a proposal for a book.
func (s *Service) CreateProposal(ctx context.Context, bookID int64, result Result) (int64, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return 0, fmt.Errorf("marshal proposal data: %w", err)
	}

	q := New(s.db)
	params := CreateMetadataProposalParams{
		BookID:   bookID,
		Provider: result.Provider,
		Data:     string(data),
	}
	if result.ProviderID != "" {
		params.ProviderID = sql.NullString{String: result.ProviderID, Valid: true}
	}

	proposal, err := q.CreateMetadataProposal(ctx, params)
	if err != nil {
		return 0, fmt.Errorf("create metadata proposal: %w", err)
	}

	if s.broadcastProposalReady != nil {
		s.broadcastProposalReady(proposal.ID)
	}

	return proposal.ID, nil
}

// AcceptProposal applies a proposal's metadata to the book.
// Respects field lock flags — locked fields are not overwritten.
func (s *Service) AcceptProposal(ctx context.Context, proposalID int64) error {
	q := New(s.db)

	// Load the proposal.
	proposal, err := q.GetMetadataProposal(ctx, proposalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProposalNotFound
		}
		return fmt.Errorf("get proposal: %w", err)
	}

	if proposal.Status != "PENDING" {
		return fmt.Errorf("proposal %d is not pending (status: %s)", proposalID, proposal.Status)
	}

	// Parse the proposal data.
	var result Result
	if err := json.Unmarshal([]byte(proposal.Data), &result); err != nil {
		return fmt.Errorf("parse proposal data: %w", err)
	}

	// Load current book metadata to check lock flags.
	bq := book.New(s.db)
	meta, err := bq.GetBookMetadata(ctx, proposal.BookID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get book metadata: %w", err)
	}

	// Build upsert params, respecting lock flags.
	upsertParams := book.UpsertBookMetadataParams{
		BookID: proposal.BookID,
	}

	// Title: only set if not locked.
	if meta.TitleLocked == 0 && result.Title != "" {
		upsertParams.Title = sql.NullString{String: result.Title, Valid: true}
	} else if meta.Title.Valid {
		upsertParams.Title = meta.Title
	}

	// Subtitle: only set if not locked.
	if meta.SubtitleLocked == 0 && result.Subtitle != "" {
		upsertParams.Subtitle = sql.NullString{String: result.Subtitle, Valid: true}
	} else if meta.Subtitle.Valid {
		upsertParams.Subtitle = meta.Subtitle
	}

	// Description: only set if not locked.
	if meta.DescriptionLocked == 0 && result.Description != "" {
		upsertParams.Description = sql.NullString{String: result.Description, Valid: true}
	} else if meta.Description.Valid {
		upsertParams.Description = meta.Description
	}

	// Publisher: only set if not locked.
	if meta.PublisherLocked == 0 && result.Publisher != "" {
		upsertParams.Publisher = sql.NullString{String: result.Publisher, Valid: true}
	} else if meta.Publisher.Valid {
		upsertParams.Publisher = meta.Publisher
	}

	// PublishDate: only set if not locked.
	if meta.PublishDateLocked == 0 && result.PublishDate != "" {
		upsertParams.PublishDate = sql.NullString{String: result.PublishDate, Valid: true}
	} else if meta.PublishDate.Valid {
		upsertParams.PublishDate = meta.PublishDate
	}

	// PageCount: only set if not locked.
	if meta.PageCountLocked == 0 && result.PageCount > 0 {
		upsertParams.PageCount = sql.NullInt64{Int64: int64(result.PageCount), Valid: true}
	} else if meta.PageCount.Valid {
		upsertParams.PageCount = meta.PageCount
	}

	// Language: only set if not locked.
	if meta.LanguageLocked == 0 && result.Language != "" {
		upsertParams.Language = sql.NullString{String: result.Language, Valid: true}
	} else if meta.Language.Valid {
		upsertParams.Language = meta.Language
	}

	// ISBN10: only set if not locked.
	if meta.Isbn10Locked == 0 && result.ISBN10 != "" {
		upsertParams.Isbn10 = sql.NullString{String: result.ISBN10, Valid: true}
	} else if meta.Isbn10.Valid {
		upsertParams.Isbn10 = meta.Isbn10
	}

	// ISBN13: only set if not locked.
	if meta.Isbn13Locked == 0 && result.ISBN13 != "" {
		upsertParams.Isbn13 = sql.NullString{String: result.ISBN13, Valid: true}
	} else if meta.Isbn13.Valid {
		upsertParams.Isbn13 = meta.Isbn13
	}

	// Apply the metadata update.
	if err := bq.UpsertBookMetadata(ctx, upsertParams); err != nil {
		return fmt.Errorf("upsert book metadata: %w", err)
	}

	// Update authors if not locked (no per-field lock for authors; use title lock as proxy).
	if meta.TitleLocked == 0 && len(result.Authors) > 0 {
		// Clear existing authors and re-link.
		if _, err := s.db.ExecContext(ctx, "DELETE FROM book_author WHERE book_id = ?", proposal.BookID); err != nil {
			return fmt.Errorf("clear book authors: %w", err)
		}
		for i, authorName := range result.Authors {
			author, err := bq.GetOrCreateAuthor(ctx, authorName)
			if err != nil {
				return fmt.Errorf("get or create author %q: %w", authorName, err)
			}
			if err := bq.LinkBookAuthor(ctx, book.LinkBookAuthorParams{
				BookID:    proposal.BookID,
				AuthorID:  author.ID,
				SortOrder: int64(i),
			}); err != nil {
				return fmt.Errorf("link book author %q: %w", authorName, err)
			}
		}
	}

	// Mark proposal as ACCEPTED.
	if err := q.UpdateProposalStatus(ctx, UpdateProposalStatusParams{
		Status: "ACCEPTED",
		ID:     proposalID,
	}); err != nil {
		return fmt.Errorf("update proposal status: %w", err)
	}

	s.logger.Info("metadata proposal accepted",
		"proposal_id", proposalID,
		"book_id", proposal.BookID,
		"provider", proposal.Provider,
	)

	if s.broadcastBookUpdated != nil {
		s.broadcastBookUpdated(proposal.BookID)
	}

	return nil
}

// RejectProposal marks a proposal as rejected.
func (s *Service) RejectProposal(ctx context.Context, proposalID int64) error {
	q := New(s.db)

	proposal, err := q.GetMetadataProposal(ctx, proposalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProposalNotFound
		}
		return fmt.Errorf("get proposal: %w", err)
	}

	if proposal.Status != "PENDING" {
		return fmt.Errorf("proposal %d is not pending (status: %s)", proposalID, proposal.Status)
	}

	if err := q.UpdateProposalStatus(ctx, UpdateProposalStatusParams{
		Status: "REJECTED",
		ID:     proposalID,
	}); err != nil {
		return fmt.Errorf("update proposal status: %w", err)
	}

	return nil
}

// ListProposals returns proposals for a book.
func (s *Service) ListProposals(ctx context.Context, bookID int64) ([]Proposal, error) {
	q := New(s.db)

	rows, err := q.ListMetadataProposals(ctx, bookID)
	if err != nil {
		return nil, fmt.Errorf("list metadata proposals: %w", err)
	}

	proposals := make([]Proposal, 0, len(rows))
	for _, row := range rows {
		var result Result
		if err := json.Unmarshal([]byte(row.Data), &result); err != nil {
			s.logger.Warn("parse proposal data",
				"proposal_id", row.ID,
				"error", err,
			)
			continue
		}

		p := Proposal{
			ID:        row.ID,
			BookID:    row.BookID,
			Provider:  row.Provider,
			Status:    row.Status,
			Data:      result,
			CreatedAt: row.CreatedAt,
		}
		if row.ProviderID.Valid {
			p.ProviderID = row.ProviderID.String
		}

		proposals = append(proposals, p)
	}

	return proposals, nil
}

// MergeProposals merges multiple proposals for the same book using provider priorities.
func (s *Service) MergeProposals(ctx context.Context, bookID int64, proposalIDs []int64) error {
	q := New(s.db)

	// Fetch all proposals.
	type proposalWithResult struct {
		proposal MetadataProposal
		result   Result
	}

	proposals := make([]proposalWithResult, 0, len(proposalIDs))
	for _, id := range proposalIDs {
		proposal, err := q.GetMetadataProposal(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return ErrProposalNotFound
			}
			return fmt.Errorf("get proposal %d: %w", id, err)
		}

		if proposal.Status != "PENDING" {
			return fmt.Errorf("proposal %d is not pending (status: %s)", id, proposal.Status)
		}

		if proposal.BookID != bookID {
			return fmt.Errorf("proposal %d belongs to book %d, expected %d", id, proposal.BookID, bookID)
		}

		var result Result
		if err := json.Unmarshal([]byte(proposal.Data), &result); err != nil {
			return fmt.Errorf("parse proposal %d data: %w", id, err)
		}

		proposals = append(proposals, proposalWithResult{proposal: proposal, result: result})
	}

	// Build ranked proposals with provider priorities (default 5).
	type rankedProposal struct {
		proposalWithResult
		priority int
	}

	ranked := make([]rankedProposal, 0, len(proposals))
	for _, p := range proposals {
		priority, err := q.GetProviderPriority(ctx, p.proposal.Provider)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				priority = 5
			} else {
				return fmt.Errorf("get provider priority for %q: %w", p.proposal.Provider, err)
			}
		}
		ranked = append(ranked, rankedProposal{proposalWithResult: p, priority: int(priority)})
	}

	// Sort by priority descending (highest first).
	sort.Slice(ranked, func(i, j int) bool {
		return ranked[i].priority > ranked[j].priority
	})

	// Load current book metadata to check lock flags.
	bq := book.New(s.db)
	meta, err := bq.GetBookMetadata(ctx, bookID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("get book metadata: %w", err)
	}

	// Build upsert params, respecting lock flags and picking highest-priority value.
	upsertParams := book.UpsertBookMetadataParams{
		BookID: bookID,
	}

	pickString := func(locked int64, current sql.NullString, getter func(r Result) string) sql.NullString {
		if locked != 0 {
			return current
		}
		for _, rp := range ranked {
			if v := getter(rp.result); v != "" {
				return sql.NullString{String: v, Valid: true}
			}
		}
		return current
	}

	pickInt64 := func(locked int64, current sql.NullInt64, getter func(r Result) int64) sql.NullInt64 {
		if locked != 0 {
			return current
		}
		for _, rp := range ranked {
			if v := getter(rp.result); v > 0 {
				return sql.NullInt64{Int64: v, Valid: true}
			}
		}
		return current
	}

	upsertParams.Title = pickString(meta.TitleLocked, meta.Title, func(r Result) string { return r.Title })
	upsertParams.Subtitle = pickString(meta.SubtitleLocked, meta.Subtitle, func(r Result) string { return r.Subtitle })
	upsertParams.Description = pickString(meta.DescriptionLocked, meta.Description, func(r Result) string { return r.Description })
	upsertParams.Publisher = pickString(meta.PublisherLocked, meta.Publisher, func(r Result) string { return r.Publisher })
	upsertParams.PublishDate = pickString(meta.PublishDateLocked, meta.PublishDate, func(r Result) string { return r.PublishDate })
	upsertParams.Language = pickString(meta.LanguageLocked, meta.Language, func(r Result) string { return r.Language })
	upsertParams.Isbn10 = pickString(meta.Isbn10Locked, meta.Isbn10, func(r Result) string { return r.ISBN10 })
	upsertParams.Isbn13 = pickString(meta.Isbn13Locked, meta.Isbn13, func(r Result) string { return r.ISBN13 })
	upsertParams.PageCount = pickInt64(meta.PageCountLocked, meta.PageCount, func(r Result) int64 { return int64(r.PageCount) })

	if err := bq.UpsertBookMetadata(ctx, upsertParams); err != nil {
		return fmt.Errorf("upsert book metadata: %w", err)
	}

	// Update authors if not locked (no per-field lock for authors; use title lock as proxy).
	var mergedAuthors []string
	if meta.TitleLocked == 0 {
		for _, rp := range ranked {
			if len(rp.result.Authors) > 0 {
				mergedAuthors = rp.result.Authors
				break
			}
		}
	}
	if len(mergedAuthors) > 0 {
		if _, err := s.db.ExecContext(ctx, "DELETE FROM book_author WHERE book_id = ?", bookID); err != nil {
			return fmt.Errorf("clear book authors: %w", err)
		}
		for i, authorName := range mergedAuthors {
			author, err := bq.GetOrCreateAuthor(ctx, authorName)
			if err != nil {
				return fmt.Errorf("get or create author %q: %w", authorName, err)
			}
			if err := bq.LinkBookAuthor(ctx, book.LinkBookAuthorParams{
				BookID:    bookID,
				AuthorID:  author.ID,
				SortOrder: int64(i),
			}); err != nil {
				return fmt.Errorf("link book author %q: %w", authorName, err)
			}
		}
	}

	// Mark all proposals as ACCEPTED.
	for _, rp := range ranked {
		if err := q.UpdateProposalStatus(ctx, UpdateProposalStatusParams{
			Status: "ACCEPTED",
			ID:     rp.proposal.ID,
		}); err != nil {
			return fmt.Errorf("update proposal %d status: %w", rp.proposal.ID, err)
		}
	}

	s.logger.Info("metadata proposals merged",
		"book_id", bookID,
		"proposal_count", len(proposalIDs),
	)

	if s.broadcastBookUpdated != nil {
		s.broadcastBookUpdated(bookID)
	}

	return nil
}

// GetProviderPriorities returns all configured provider priorities.
func (s *Service) GetProviderPriorities(ctx context.Context) ([]ProviderPriority, error) {
	q := New(s.db)
	rows, err := q.ListProviderPriorities(ctx)
	if err != nil {
		return nil, fmt.Errorf("list provider priorities: %w", err)
	}

	priorities := make([]ProviderPriority, 0, len(rows))
	for _, row := range rows {
		priorities = append(priorities, ProviderPriority{
			ProviderName: row.ProviderName,
			Priority:     row.Priority,
		})
	}
	return priorities, nil
}

// SetProviderPriority sets the priority for a provider.
func (s *Service) SetProviderPriority(ctx context.Context, provider string, priority int) error {
	q := New(s.db)
	if err := q.UpsertProviderPriority(ctx, UpsertProviderPriorityParams{
		ProviderName: provider,
		Priority:     int64(priority),
	}); err != nil {
		return fmt.Errorf("upsert provider priority: %w", err)
	}
	return nil
}

// GetAppSetting retrieves an app setting value by key.
// Returns empty string if the key does not exist.
func (s *Service) GetAppSetting(ctx context.Context, key string) (string, error) {
	var value sql.NullString
	err := s.db.QueryRowContext(ctx, "SELECT value FROM app_settings WHERE key = ? LIMIT 1", key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", fmt.Errorf("get app setting %q: %w", key, err)
	}
	if !value.Valid {
		return "", nil
	}
	return value.String, nil
}

// SetAppSetting saves an app setting value.
func (s *Service) SetAppSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO app_settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("set app setting %q: %w", key, err)
	}
	return nil
}

// Provider returns a registered provider by name.
func (s *Service) Provider(name string) (Provider, bool) {
	p, ok := s.providers[name]
	return p, ok
}
