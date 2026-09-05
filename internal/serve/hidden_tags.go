package serve

import (
	"context"
	"strconv"
	"strings"

	"gooru.local/internal/query"
	"gooru.local/types"
)

// hiddenTagQueryPolicy applies WebUI-only default exclusions without changing the
// core query language. A hidden tag is lifted only when the user positively
// requests that exact tag somewhere in the parsed expression.
type hiddenTagQueryPolicy struct {
	tags []string
}

func newHiddenTagQueryPolicy(tags []string) hiddenTagQueryPolicy {
	return hiddenTagQueryPolicy{tags: append([]string(nil), tags...)}
}

func (p hiddenTagQueryPolicy) apply(queryText string) string {
	return p.applyWithRequested(queryText, p.requested(queryText))
}

// baselineFor returns the library-wide visibility query for the same request.
// Hidden tags explicitly requested by the user are lifted, while unrelated
// filters are intentionally omitted from the library count.
func (p hiddenTagQueryPolicy) baselineFor(queryText string) string {
	return p.applyWithRequested("", p.requested(queryText))
}

func (p hiddenTagQueryPolicy) applyWithRequested(queryText string, requested map[string]struct{}) string {
	if len(p.tags) == 0 {
		return queryText
	}
	exclusions := make([]string, 0, len(p.tags))
	for _, tag := range p.tags {
		if _, ok := requested[tag]; ok {
			continue
		}
		exclusions = append(exclusions, "-"+strconv.Quote(tag))
	}
	if len(exclusions) == 0 {
		return queryText
	}
	trimmed := strings.TrimSpace(queryText)
	if trimmed == "" {
		return strings.Join(exclusions, " ")
	}
	// Group the user expression so an OR query cannot bypass exclusions on only
	// one branch (for example, "a | b" must become "(a | b) -hidden").
	return "(" + queryText + ") " + strings.Join(exclusions, " ")
}

func (p hiddenTagQueryPolicy) requested(queryText string) map[string]struct{} {
	requested := make(map[string]struct{})
	if len(p.tags) == 0 || strings.TrimSpace(queryText) == "" {
		return requested
	}
	expr, err := query.Parse(queryText)
	if err != nil {
		return requested
	}
	positive := make(map[string]struct{})
	collectPositiveQueryTags(expr, false, positive)
	for _, tag := range p.tags {
		if _, ok := positive[tag]; ok {
			requested[tag] = struct{}{}
		}
	}
	return requested
}

func collectPositiveQueryTags(expr *query.Expression, negated bool, out map[string]struct{}) {
	if expr == nil {
		return
	}
	for _, andTerm := range expr.Or {
		if andTerm == nil {
			continue
		}
		for _, term := range andTerm.And {
			if term == nil || term.Factor == nil {
				continue
			}
			termNegated := negated != term.Not
			if term.Factor.Tag != nil && !termNegated {
				out[*term.Factor.Tag] = struct{}{}
			}
			if term.Factor.SubExpr != nil {
				collectPositiveQueryTags(term.Factor.SubExpr, termNegated, out)
			}
		}
	}
}

// hiddenTagLibrary decorates the concrete WebUI library so the policy stays at
// the query boundary. Tag listing/suggestions deliberately pass through.
type hiddenTagLibrary struct {
	inner  *GooruLibrary
	policy hiddenTagQueryPolicy
}

func newHiddenTagLibrary(inner *GooruLibrary, tags []string) *hiddenTagLibrary {
	return &hiddenTagLibrary{inner: inner, policy: newHiddenTagQueryPolicy(tags)}
}

func (l *hiddenTagLibrary) ListFiles(ctx context.Context, queryText string) ([]types.FileInfo, error) {
	return l.inner.ListFiles(ctx, l.policy.apply(queryText))
}

func (l *hiddenTagLibrary) ListFilesPage(ctx context.Context, queryText string, page Page) (PageResult[types.FileInfo], error) {
	return l.inner.ListFilesPage(ctx, l.policy.apply(queryText), page)
}

func (l *hiddenTagLibrary) ListFilesSearch(ctx context.Context, queryText string, page Page, sort string, order string) (PageResult[types.FileInfo], error) {
	return l.inner.ListFilesSearch(ctx, l.policy.apply(queryText), page, sort, order)
}

func (l *hiddenTagLibrary) CountFiles(ctx context.Context, queryText string) (int, error) {
	return l.inner.CountFiles(ctx, l.policy.apply(queryText))
}

func (l *hiddenTagLibrary) LibraryCount(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return l.inner.client.CountFilesByQuery(l.policy.baselineFor(""), l.inner.verbose)
}

func (l *hiddenTagLibrary) LibraryCountForQuery(ctx context.Context, queryText string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return l.inner.client.CountFilesByQuery(l.policy.baselineFor(queryText), l.inner.verbose)
}

func (l *hiddenTagLibrary) KindFacets(ctx context.Context, queryText string) ([]FacetValueDTO, error) {
	return l.inner.KindFacets(ctx, l.policy.apply(queryText))
}

func (l *hiddenTagLibrary) TagSuggestions(ctx context.Context, prefix string, existing string, limit int) ([]TagDTO, error) {
	return l.inner.TagSuggestions(ctx, prefix, existing, limit)
}

func (l *hiddenTagLibrary) TagNamespaces(ctx context.Context) ([]string, error) {
	return l.inner.TagNamespaces(ctx)
}

func (l *hiddenTagLibrary) FileMetadata(ctx context.Context, locationID int64) (MediaMetadata, error) {
	return l.inner.FileMetadata(ctx, locationID)
}

func (l *hiddenTagLibrary) GetFile(ctx context.Context, locationID int64) (types.FileInfo, error) {
	return l.inner.GetFile(ctx, locationID)
}

func (l *hiddenTagLibrary) ListTags(ctx context.Context, counts bool, limit int) ([]TagDTO, error) {
	return l.inner.ListTags(ctx, counts, limit)
}

func (l *hiddenTagLibrary) PublicFileID(file types.FileInfo) string {
	return l.inner.PublicFileID(file)
}

func (l *hiddenTagLibrary) GetFileByPublicID(ctx context.Context, id string) (types.FileInfo, error) {
	return l.inner.GetFileByPublicID(ctx, id)
}

func (l *hiddenTagLibrary) DeleteFileByPublicID(ctx context.Context, id string) (bool, error) {
	return l.inner.DeleteFileByPublicID(ctx, id)
}
