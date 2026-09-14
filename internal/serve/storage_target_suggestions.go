package serve

import "strings"

type storageTargetSuggestionLibrary interface {
	StorageTargetSuggestions(prefix string, limit int) []TagDTO
}

func storageTargetSuggestions(prefix string, targets []UploadTarget, limit int) []TagDTO {
	candidate := strings.TrimSpace(prefix)
	negated := strings.HasPrefix(candidate, "-")
	candidate = strings.TrimPrefix(candidate, "-")
	const syntaxPrefix = "@in_target:"
	if !strings.HasPrefix(strings.ToLower(candidate), syntaxPrefix) {
		return nil
	}
	valuePrefix := strings.TrimSpace(candidate[len(syntaxPrefix):])
	if limit <= 0 {
		limit = 20
	}

	ids := make([]string, 0, len(targets)+1)
	ids = append(ids, "any")
	seen := map[string]struct{}{"any": {}}
	for _, target := range targets {
		id := strings.TrimSpace(target.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		ids = append(ids, id)
	}

	out := make([]TagDTO, 0, min(limit, len(ids)))
	for _, id := range ids {
		if !strings.HasPrefix(strings.ToLower(id), strings.ToLower(valuePrefix)) {
			continue
		}
		name := syntaxPrefix + id
		if negated {
			name = "-" + name
		}
		out = append(out, TagDTO{Name: name, Value: "managed upload target"})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (l *GooruLibrary) StorageTargetSuggestions(prefix string, limit int) []TagDTO {
	return storageTargetSuggestions(prefix, l.managedTargets, limit)
}

func (s *Server) storageTargetSuggestionsForRequest(prefix string, limit int) []TagDTO {
	library, ok := s.library.(storageTargetSuggestionLibrary)
	if !ok {
		return nil
	}
	return library.StorageTargetSuggestions(prefix, limit)
}
