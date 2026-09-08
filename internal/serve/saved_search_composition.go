package serve

import (
	"context"
	"fmt"
	"strings"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
	"gooru.local/types"
)

func expandSavedSearchQuery(raw string, saved []types.SavedSearch) (string, error) {
	if !strings.Contains(strings.ToLower(raw), "@saved:") {
		return raw, nil
	}
	ast, err := query.Parse(raw)
	if err != nil {
		return "", core.ErrInvalidQuery
	}
	byName := make(map[string]string, len(saved))
	for _, item := range saved {
		byName[strings.ToLower(strings.TrimSpace(item.Name))] = item.Query
	}
	if err := query.ExpandSavedSearches(ast, func(name string) (string, error) {
		stored, ok := byName[strings.ToLower(strings.TrimSpace(name))]
		if !ok {
			return "", fmt.Errorf("%w: saved search %q not found", core.ErrInvalidQuery, name)
		}
		return stored, nil
	}); err != nil {
		return "", fmt.Errorf("%w: %v", core.ErrInvalidQuery, err)
	}
	if err := query.ValidateAST(ast); err != nil {
		return "", core.ErrInvalidQuery
	}
	return query.Format(ast), nil
}

func (s *Server) expandSavedSearchQueryForRequest(ctx context.Context, raw string) (string, error) {
	if !strings.Contains(strings.ToLower(raw), "@saved:") {
		return raw, nil
	}
	library, ok := s.library.(SavedSearchLibrary)
	if !ok {
		return "", core.ErrInvalidQuery
	}
	auth, ok := currentAuth(ctx)
	if !ok {
		return "", core.ErrInvalidQuery
	}
	saved, err := library.ListSavedSearches(ctx, auth.User.ID)
	if err != nil {
		return "", err
	}
	return expandSavedSearchQuery(raw, saved)
}

func savedSearchNameSuggestions(prefix string, saved []types.SavedSearch, limit int) []TagDTO {
	candidate := strings.TrimSpace(prefix)
	negated := strings.HasPrefix(candidate, "-")
	candidate = strings.TrimPrefix(candidate, "-")
	if !strings.HasPrefix(strings.ToLower(candidate), "@saved:") {
		return nil
	}
	namePrefix := strings.TrimSpace(candidate[len("@saved:"):])
	if limit <= 0 {
		limit = 20
	}
	out := make([]TagDTO, 0, min(limit, len(saved)))
	for _, item := range saved {
		name := strings.TrimSpace(item.Name)
		if name == "" || !strings.HasPrefix(strings.ToLower(name), strings.ToLower(namePrefix)) {
			continue
		}
		syntax := "@saved:" + name
		if negated {
			syntax = "-" + syntax
		}
		out = append(out, TagDTO{Name: syntax, Value: "saved search"})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (s *Server) savedSearchNameSuggestionsForRequest(ctx context.Context, prefix string, limit int) ([]TagDTO, error) {
	candidate := strings.TrimPrefix(strings.TrimSpace(prefix), "-")
	if !strings.HasPrefix(strings.ToLower(candidate), "@saved:") {
		return nil, nil
	}
	library, ok := s.library.(SavedSearchLibrary)
	if !ok {
		return nil, nil
	}
	auth, ok := currentAuth(ctx)
	if !ok {
		return nil, nil
	}
	saved, err := library.ListSavedSearches(ctx, auth.User.ID)
	if err != nil {
		return nil, err
	}
	return savedSearchNameSuggestions(prefix, saved, limit), nil
}
