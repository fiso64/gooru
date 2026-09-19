package serve

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	core "gooru.local/gooru"
	"gooru.local/internal/query"
	"gooru.local/types"
)

var ErrNotFound = errors.New("not found")

type Library interface {
	ListFiles(ctx context.Context, query string) ([]types.FileInfo, error)
	GetFile(ctx context.Context, locationID int64) (types.FileInfo, error)
	ListTags(ctx context.Context, counts bool, limit int) ([]TagDTO, error)
}

type PagedLibrary interface {
	ListFilesPage(ctx context.Context, query string, page Page) (PageResult[types.FileInfo], error)
}

type SearchLibrary interface {
	ListFilesSearch(ctx context.Context, query string, page Page, sort string, order string) (PageResult[types.FileInfo], error)
	LibraryCount(ctx context.Context) (int, error)
	KindFacets(ctx context.Context, query string) ([]FacetValueDTO, error)
	TagSuggestions(ctx context.Context, prefix string, existing string, limit int) ([]TagDTO, error)
	TagNamespaces(ctx context.Context) ([]string, error)
	FileMetadata(ctx context.Context, locationID int64) (MediaMetadata, error)
}

type QueryLibraryCount interface {
	LibraryCountForQuery(ctx context.Context, query string) (int, error)
}

type PublicFileLibrary interface {
	PublicFileID(file types.FileInfo) string
	GetFileByPublicID(ctx context.Context, id string) (types.FileInfo, error)
	DeleteFileByPublicID(ctx context.Context, id string) (bool, error)
}

// BackgroundFileRemovalLibrary is the durable mutation boundary used by bulk
// delete/untrack. Keeping operation creation on the library adapter lets the
// HTTP package compose work without depending on persistence details.
type BackgroundFileRemovalLibrary interface {
	PublicFileLibrary
	CreateBackgroundOperationWithTasks(core.BackgroundOperationRequest, []core.BackgroundTaskRequest) (core.BackgroundOperation, []core.BackgroundTask, error)
}

type FileSelectionLibrary interface {
	ListPublicFileIDs(ctx context.Context, query string) ([]string, error)
}

type FileCountLibrary interface {
	CountFiles(ctx context.Context, query string) (int, error)
}

type GooruLibrary struct {
	client          *core.Client
	verbose         bool
	metadata        MediaMetadataProvider
	encryption      EncryptionConfig
	managedRoots    []string
	managedTargets  []UploadTarget
	backgroundTasks func(types.LocationInfo) []core.BackgroundTaskRequest
}

func NewGooruLibrary(client *core.Client, verbose bool) *GooruLibrary {
	return &GooruLibrary{client: client, verbose: verbose, metadata: BasicMediaMetadataProvider{}}
}

func (l *GooruLibrary) GetFileByContentHash(ctx context.Context, hash string) (types.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return types.FileInfo{}, err
	}
	file, err := l.client.GetFileInfoByContentHash(hash)
	if err != nil {
		return types.FileInfo{}, err
	}
	return l.repairManagedPath(file)
}

func (l *GooruLibrary) ListFiles(ctx context.Context, query string) ([]types.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(query) == "" {
		return l.client.GetAllFilesInfo()
	}
	return l.client.GetFilesInfoByQuery(query, l.verbose)
}

func (l *GooruLibrary) ListPublicFileIDs(ctx context.Context, query string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.client.ListPublicFileIDsByQuery(query, l.verbose)
}

func (l *GooruLibrary) ListFilesPage(ctx context.Context, query string, page Page) (PageResult[types.FileInfo], error) {
	if err := ctx.Err(); err != nil {
		return PageResult[types.FileInfo]{}, err
	}
	limit := page.Limit + 1
	var files []types.FileInfo
	var err error
	if strings.TrimSpace(query) == "" {
		files, err = l.client.GetAllFilesInfoPage(limit, page.Offset)
	} else {
		files, err = l.client.GetFilesInfoByQueryPage(query, limit, page.Offset, l.verbose)
	}
	if err != nil {
		return PageResult[types.FileInfo]{}, err
	}
	result := PageResult[types.FileInfo]{Items: files}
	if len(result.Items) > page.Limit {
		result.Items = result.Items[:page.Limit]
		result.NextPageToken = NextPageToken(page.Offset, page.Limit, page.Limit)
	}
	return result, nil
}

func (l *GooruLibrary) ListFilesSearch(ctx context.Context, query string, page Page, sort string, order string) (PageResult[types.FileInfo], error) {
	if err := ctx.Err(); err != nil {
		return PageResult[types.FileInfo]{}, err
	}
	cursor, err := ValidateCursorForSort(page.Cursor, sort, order)
	if err != nil {
		return PageResult[types.FileInfo]{}, err
	}
	limit := page.Limit + 1
	var files []types.FileInfo
	if cursor != nil {
		files, err = l.client.GetFilesInfoByQueryPageSorted(query, limit, cursor, sort, order, l.verbose)
	} else {
		files, err = l.client.GetFilesInfoByQueryPageSortedOffset(query, limit, page.Offset, sort, order, l.verbose)
	}
	if err != nil {
		return PageResult[types.FileInfo]{}, err
	}
	result := PageResult[types.FileInfo]{Items: files}
	if len(result.Items) > page.Limit {
		result.Items = result.Items[:page.Limit]
		last := result.Items[len(result.Items)-1]
		result.NextPageToken = CursorPageTokenAtOffset(sort, order, last.ID, page.Offset+page.Limit)
	}
	return result, nil
}

func (l *GooruLibrary) GetFile(ctx context.Context, locationID int64) (types.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return types.FileInfo{}, err
	}
	file, err := l.client.GetFileInfoByLocationID(locationID)
	if errors.Is(err, sql.ErrNoRows) {
		return types.FileInfo{}, ErrNotFound
	}
	if err != nil {
		return types.FileInfo{}, err
	}
	return l.repairManagedPath(file)
}

func (l *GooruLibrary) ListTags(ctx context.Context, counts bool, limit int) ([]TagDTO, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if counts {
		tags, err := l.client.GetTagsWithCounts(limit)
		if err != nil {
			return nil, err
		}
		out := make([]TagDTO, 0, len(tags))
		for _, tag := range tags {
			count := tag.Count
			out = append(out, tagDTO(tag.Tag, &count))
		}
		return out, nil
	}
	tags, err := l.client.GetTags(limit)
	if err != nil {
		return nil, err
	}
	out := make([]TagDTO, 0, len(tags))
	for _, tag := range tags {
		out = append(out, tagDTO(tag, nil))
	}
	return out, nil
}

func (l *GooruLibrary) LibraryCount(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return l.client.CountFilesByQuery("", l.verbose)
}

func (l *GooruLibrary) CountFiles(ctx context.Context, query string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return l.client.CountFileLocationsByQuery(query, l.verbose)
}

func (l *GooruLibrary) KindFacets(ctx context.Context, query string) ([]FacetValueDTO, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facets, err := l.client.KindFacetsByQuery(query, l.verbose)
	if err != nil {
		return nil, err
	}
	out := make([]FacetValueDTO, 0, len(facets))
	for _, item := range facets {
		out = append(out, FacetValueDTO{Value: item.Tag, Count: item.Count})
	}
	return out, nil
}

func (l *GooruLibrary) TagSuggestions(ctx context.Context, prefix string, existing string, limit int) ([]TagDTO, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	excluded := excludedSuggestionTags(existing)
	var tags []types.TagWithCount
	var err error
	if namespace, valuePrefix, ok := strings.Cut(strings.TrimSpace(prefix), ":"); ok && namespace != "" && !strings.ContainsAny(namespace, " \t\r\n") {
		tags, err = l.client.TagValueSuggestions(namespace, valuePrefix, limit)
	} else {
		namespaceTags, err := l.client.NamespaceSuggestions(prefix, limit)
		if err != nil {
			return nil, err
		}
		tagTags, err := l.client.TagSuggestions(prefix, limit)
		if err != nil {
			return nil, err
		}
		tags = mergeSuggestions(limit, namespaceTags, tagTags)
	}
	if err != nil {
		return nil, err
	}
	out := make([]TagDTO, 0, len(tags))
	for _, item := range tags {
		if _, skip := excluded[item.Tag]; skip {
			continue
		}
		count := item.Count
		out = append(out, tagDTO(item.Tag, &count))
	}
	return out, nil
}

func (l *GooruLibrary) TagNamespaces(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return l.client.TagNamespaces()
}

func (l *GooruLibrary) DeleteFileByPublicID(ctx context.Context, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	locationID, err := l.client.ResolvePublicFileID(id)
	if err != nil {
		return false, ErrNotFound
	}
	return l.client.DeleteLocationByID(locationID)
}

func (l *GooruLibrary) CreateBackgroundOperationWithTasks(operation core.BackgroundOperationRequest, tasks []core.BackgroundTaskRequest) (core.BackgroundOperation, []core.BackgroundTask, error) {
	return l.client.CreateBackgroundOperationWithTasks(operation, tasks)
}

func (l *GooruLibrary) FileMetadata(ctx context.Context, locationID int64) (MediaMetadata, error) {
	if err := ctx.Err(); err != nil {
		return MediaMetadata{}, err
	}
	meta, err := l.client.GetMediaMetadata(locationID)
	haveStored := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MediaMetadata{}, err
	}
	if haveStored && meta.PageCount != nil && meta.ImageWidth != nil && meta.ImageHeight != nil {
		return mediaMetadataDTO(meta), nil
	}

	file, fileErr := l.client.GetFileInfoByLocationID(locationID)
	if fileErr == nil {
		file, fileErr = l.repairManagedPath(file)
	}
	if fileErr != nil || !strings.EqualFold(filepath.Ext(file.Path), ".cbz") {
		if haveStored {
			return mediaMetadataDTO(meta), nil
		}
		return MediaMetadata{}, fileErr
	}
	mediaType := mediaTypeForPath(file.Path)
	mediaKind := mediaKindForType(mediaType)
	provider := l.metadata
	if provider == nil {
		provider = BasicMediaMetadataProvider{}
	}
	derived, deriveErr := l.importedMediaMetadata(ctx, provider, file, fileStoragePath(file), mediaType, mediaKind)
	if deriveErr != nil || derived.PageCount == nil {
		if haveStored {
			return mediaMetadataDTO(meta), nil
		}
		return MediaMetadata{}, nil
	}
	if !haveStored {
		meta = types.MediaMetadata{LocationID: locationID, MediaKind: mediaKind, MimeType: mediaType}
	} else {
		if meta.MediaKind == "" {
			meta.MediaKind = mediaKind
		}
		if meta.MimeType == "" {
			meta.MimeType = mediaType
		}
	}
	meta.PageCount = derived.PageCount
	meta.ImageWidth = derived.ImageWidth
	meta.ImageHeight = derived.ImageHeight
	if err := l.client.UpsertMediaMetadata(meta); err != nil {
		return MediaMetadata{}, err
	}
	return mediaMetadataDTO(meta), nil
}

func (l *GooruLibrary) PublicFileID(file types.FileInfo) string {
	if file.PublicID != "" {
		return file.PublicID
	}
	return l.client.PublicFileID(file.ID)
}

func (l *GooruLibrary) GetFileByPublicID(ctx context.Context, id string) (types.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return types.FileInfo{}, err
	}
	file, err := l.client.GetFileInfoByPublicID(id)
	if errors.Is(err, sql.ErrNoRows) {
		return types.FileInfo{}, ErrNotFound
	}
	if err != nil {
		return types.FileInfo{}, err
	}
	return l.repairManagedPath(file)
}

type FileListResponse struct {
	Files             []FileDTO `json:"files"`
	NextPageToken     string    `json:"next_page_token,omitempty"`
	PreviousPageToken string    `json:"previous_page_token,omitempty"`
	TotalCount        int       `json:"total_count"`
	LibraryCount      int       `json:"library_count"`
	Facets            FacetsDTO `json:"facets,omitempty"`
}

type TagListResponse struct {
	Tags         []TagDTO  `json:"tags"`
	LibraryCount int       `json:"library_count"`
	Facets       FacetsDTO `json:"facets,omitempty"`
}

type FacetsDTO struct {
	Kind []FacetValueDTO `json:"kind,omitempty"`
}

type FacetValueDTO struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type FileDTO struct {
	ID              string        `json:"id"`
	ContentID       string        `json:"content_id"`
	Name            string        `json:"name"`
	Path            string        `json:"path,omitempty"`
	SafeDisplayPath string        `json:"safe_display_path"`
	Size            int64         `json:"size"`
	AddedAt         time.Time     `json:"added_at"`
	ModifiedTime    time.Time     `json:"modified_time"`
	MediaType       string        `json:"media_type"`
	MediaKind       string        `json:"media_kind"`
	ViewerSupport   string        `json:"viewer_support"`
	Metadata        MediaMetadata `json:"metadata,omitempty"`
	Tags            []string      `json:"tags"`
	MediaURLs       MediaURLs     `json:"media_urls"`
	CanDelete       bool          `json:"can_delete"`
}

type MediaURLs struct {
	Thumbnail string `json:"thumbnail"`
	Preview   string `json:"preview"`
	Lossless  string `json:"lossless,omitempty"`
	Content   string `json:"content"`
	Download  string `json:"download"`
}

type TagDTO struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Value     string `json:"value,omitempty"`
	Count     *int   `json:"count,omitempty"`
}

func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListFiles(w, r)
	case http.MethodDelete:
		if !s.requireAdmin(w, r) {
			return
		}
		s.handleRemoveFiles(w, r)
	default:
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
	}
}

type fileSearchRequest struct {
	Query         string `json:"query"`
	Limit         int    `json:"limit"`
	PageToken     string `json:"page_token"`
	Sort          string `json:"sort"`
	Order         string `json:"order"`
	IncludeFacets bool   `json:"include_facets"`
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	req := fileSearchRequest{
		Query:         r.URL.Query().Get("query"),
		PageToken:     r.URL.Query().Get("page_token"),
		Sort:          r.URL.Query().Get("sort"),
		Order:         r.URL.Query().Get("order"),
		IncludeFacets: r.URL.Query().Get("include_facets") == "true",
	}
	if value := r.URL.Query().Get("limit"); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			req.Limit = parsed
		} else {
			req.Limit = -1
		}
	}
	s.handleListFilesRequest(w, r, req)
}

func (s *Server) handleFileSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	var req fileSearchRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON request body", nil)
		return
	}
	s.handleListFilesRequest(w, r, req)
}

func (s *Server) handleListFilesRequest(w http.ResponseWriter, r *http.Request, req fileSearchRequest) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	limitText := ""
	if req.Limit != 0 {
		limitText = strconv.Itoa(req.Limit)
	}
	page, err := ParsePage(limitText, req.PageToken)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	queryText, err := s.expandSavedSearchQueryForRequest(r.Context(), req.Query)
	if err != nil {
		if errors.Is(err, core.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list files", nil)
		return
	}
	sort := normalizeFileSort(req.Sort)
	order := normalizeSortOrder(req.Order)
	pageResult, err := s.listFilesPage(r.Context(), queryText, page, sort, order)
	if err != nil {
		if errors.Is(err, core.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
			return
		}
		if strings.Contains(err.Error(), "page_token") {
			writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list files", nil)
		return
	}
	response := FileListResponse{
		Files:             make([]FileDTO, 0, len(pageResult.Items)),
		NextPageToken:     pageResult.NextPageToken,
		PreviousPageToken: PageOffsetToken(page.Offset - page.Limit),
	}
	response.TotalCount = page.Offset + len(pageResult.Items)
	if pageResult.NextPageToken != "" {
		response.TotalCount++
	}
	includeAggregates := req.IncludeFacets
	if search, ok := s.library.(SearchLibrary); ok && includeAggregates {
		libraryCountKnown := false
		if kind, err := search.KindFacets(r.Context(), queryText); err == nil {
			response.Facets.Kind = kind
			total := 0
			for _, facet := range kind {
				total += facet.Count
			}
			response.TotalCount = total
			if strings.TrimSpace(queryText) == "" {
				// Kind facets partition all locations, so their sum is also the
				// exact unfiltered library count without another aggregate pass.
				response.LibraryCount = total
				libraryCountKnown = true
			}
		} else if total, err := s.countFiles(r.Context(), queryText); err == nil {
			// Preserve the exact count when facet aggregation is unavailable.
			response.TotalCount = total
			if strings.TrimSpace(queryText) == "" {
				response.LibraryCount = total
				libraryCountKnown = true
			}
		}
		if !libraryCountKnown {
			if counter, ok := s.library.(QueryLibraryCount); ok {
				if total, err := counter.LibraryCountForQuery(r.Context(), queryText); err == nil {
					response.LibraryCount = total
				}
			} else if total, err := search.LibraryCount(r.Context()); err == nil {
				response.LibraryCount = total
			}
		}
	}
	for _, file := range pageResult.Items {
		response.Files = append(response.Files, s.fileDTO(r.Context(), file, false))
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) listFilesPage(ctx context.Context, query string, page Page, sort string, order string) (PageResult[types.FileInfo], error) {
	if search, ok := s.library.(SearchLibrary); ok {
		return search.ListFilesSearch(ctx, query, page, sort, order)
	}
	if paged, ok := s.library.(PagedLibrary); ok {
		return paged.ListFilesPage(ctx, query, page)
	}
	files, err := s.library.ListFiles(ctx, query)
	if err != nil {
		return PageResult[types.FileInfo]{}, err
	}
	return PaginateInMemory(files, page), nil
}

func (s *Server) countFiles(ctx context.Context, queryText string) (int, error) {
	if counter, ok := s.library.(FileCountLibrary); ok {
		return counter.CountFiles(ctx, queryText)
	}
	page := Page{Limit: 1, Offset: 0}
	result, err := s.listFilesPage(ctx, queryText, page, "name", "asc")
	if err != nil {
		return 0, err
	}
	if result.NextPageToken != "" {
		return len(result.Items) + 1, nil
	}
	return len(result.Items), nil
}

func (s *Server) handleFile(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/v1/files/")
	parts := strings.Split(rest, "/")
	if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 1 {
		if !s.requireAdmin(w, r) {
			return
		}
		s.handleDeleteFile(w, r, parts[0])
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET, DELETE")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	file, err := s.getFileByPublicID(r.Context(), parts[0])
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load file", nil)
		return
	}
	if len(parts) == 2 {
		switch parts[1] {
		case "content":
			s.media.ServeContent(w, r, file)
		case "download":
			s.media.ServeDownload(w, r, file)
		case "thumbnail":
			s.media.ServeDerivative(w, r, file, "thumbnail")
		case "preview":
			s.media.ServeDerivative(w, r, file, "preview")
		case "lossless":
			s.media.ServeLosslessJPEG(w, r, file)
		case "pdf":
			s.media.ServePDF(w, r, file, parts[0])
		default:
			writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, s.fileDTO(r.Context(), file, true))
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	limit, _ := strconvAtoiDefault(r.URL.Query().Get("limit"), 200)
	if limit > 500 {
		limit = 500
	}
	tags, err := s.library.ListTags(r.Context(), r.URL.Query().Get("counts") == "true", limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load tags", nil)
		return
	}
	response := TagListResponse{Tags: tags}
	if search, ok := s.library.(SearchLibrary); ok {
		if kind, err := search.KindFacets(r.Context(), ""); err == nil {
			response.Facets.Kind = kind
			for _, facet := range kind {
				response.LibraryCount += facet.Count
			}
		} else if total, err := search.LibraryCount(r.Context()); err == nil {
			response.LibraryCount = total
		}
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request, publicID string) {
	var req struct {
		Mode string `json:"mode"`
	}
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid_request", "request body must be valid JSON", nil)
			return
		}
	}
	if req.Mode == "" {
		req.Mode = "untrack"
	}
	if req.Mode != "untrack" && req.Mode != "delete" {
		writeError(w, http.StatusBadRequest, "invalid_request", "mode must be untrack or delete", nil)
		return
	}
	if _, ok := s.library.(PublicFileLibrary); !ok {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file mutation service is not configured", nil)
		return
	}

	var deleted bool
	var err error
	if req.Mode == "delete" {
		deleted, err = s.deleteManagedFile(r.Context(), publicID)
	} else {
		deleted, err = s.deleteFileByPublicID(r.Context(), publicID)
	}
	if errors.Is(err, ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	if errors.Is(err, ErrFileNotManaged) {
		writeError(w, http.StatusConflict, "file_not_managed", "file is outside configured upload targets; untrack it instead", nil)
		return
	}
	if err != nil {
		action := "untrack"
		if req.Mode == "delete" {
			action = "delete"
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to "+action+" file", nil)
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"mode": req.Mode, "removed_locations": 1})
}

func (s *Server) fileDTO(ctx context.Context, file types.FileInfo, includeMetadata bool) FileDTO {
	id := s.publicFileID(file)
	mediaType := mediaTypeForPath(file.Path)
	mediaKind := mediaKindForType(mediaType)
	dto := FileDTO{
		ID:              id,
		ContentID:       file.Hash,
		Name:            filepath.Base(file.Path),
		SafeDisplayPath: safeDisplayPath(file.Path),
		Size:            file.Size,
		AddedAt:         time.UnixMilli(file.AddedAt).UTC(),
		ModifiedTime:    time.Unix(file.ModTime, 0).UTC(),
		MediaType:       mediaType,
		MediaKind:       mediaKind,
		Tags:            nonNilStrings(file.Tags),
		MediaURLs: MediaURLs{
			Thumbnail: "/api/v1/files/" + id + "/thumbnail",
			Preview:   "/api/v1/files/" + id + "/preview",
			Content:   "/api/v1/files/" + id + "/content",
			Download:  "/api/v1/files/" + id + "/download",
		},
		CanDelete: s.canDeleteFilePath(fileStoragePath(file)),
	}
	if s.media.losslessJPEGAvailable(file) {
		dto.MediaURLs.Lossless = "/api/v1/files/" + id + "/lossless"
	}
	if s.cfg.Server.ExposePaths {
		dto.Path = file.Path
	}
	if file.Metadata != nil {
		if file.Metadata.MimeType != "" {
			dto.MediaType = file.Metadata.MimeType
		}
		if file.Metadata.MediaKind != "" {
			dto.MediaKind = file.Metadata.MediaKind
		}
		dto.Metadata = mediaMetadataDTO(*file.Metadata)
	} else if includeMetadata {
		if search, ok := s.library.(SearchLibrary); ok {
			if metadata, err := search.FileMetadata(ctx, file.ID); err == nil {
				dto.Metadata = metadata
			}
		} else if s.meta != nil {
			if pathProvider, ok := s.meta.(MediaMetadataPathProvider); ok {
				if metadata, err := pathProvider.Metadata(ctx, file, mediaType, mediaKind); err == nil {
					dto.Metadata = metadata
				}
			}
		}
	}
	if strings.EqualFold(filepath.Ext(file.Path), ".cbz") && (dto.Metadata.PageCount == nil || dto.Metadata.ImageWidth == nil || dto.Metadata.ImageHeight == nil) {
		if search, ok := s.library.(SearchLibrary); ok {
			if metadata, err := search.FileMetadata(ctx, file.ID); err == nil {
				dto.Metadata = metadata
			}
		}
	}
	dto.ViewerSupport = viewerSupportForMediaKind(dto.MediaKind)
	return dto
}

func (s *Server) publicFileID(file types.FileInfo) string {
	if resolver, ok := s.library.(PublicFileLibrary); ok {
		return resolver.PublicFileID(file)
	}
	return ""
}

func (s *Server) getFileByPublicID(ctx context.Context, id string) (types.FileInfo, error) {
	if resolver, ok := s.library.(PublicFileLibrary); ok {
		return resolver.GetFileByPublicID(ctx, id)
	}
	return types.FileInfo{}, ErrNotFound
}

func (s *Server) deleteFileByPublicID(ctx context.Context, id string) (bool, error) {
	if resolver, ok := s.library.(PublicFileLibrary); ok {
		return resolver.DeleteFileByPublicID(ctx, id)
	}
	return false, ErrNotFound
}

func safeDisplayPath(path string) string {
	name := filepath.Base(path)
	parent := filepath.Base(filepath.Dir(path))
	if parent == "." || parent == string(filepath.Separator) || parent == "" {
		return name
	}
	return filepath.Join(parent, name)
}

func mediaMetadataDTO(meta types.MediaMetadata) MediaMetadata {
	return MediaMetadata{
		ImageWidth:    meta.ImageWidth,
		ImageHeight:   meta.ImageHeight,
		VideoWidth:    meta.VideoWidth,
		VideoHeight:   meta.VideoHeight,
		VideoDuration: meta.DurationSeconds,
		AudioDuration: nil,
		FrameCount:    meta.FrameCount,
		PageCount:     meta.PageCount,
	}
}

func tagDTO(name string, count *int) TagDTO {
	dto := TagDTO{Name: name, Count: count}
	if parts := strings.SplitN(name, ":", 2); len(parts) == 2 {
		dto.Namespace = parts[0]
		dto.Value = parts[1]
	} else {
		dto.Value = name
	}
	return dto
}

func mergeSuggestions(limit int, groups ...[]types.TagWithCount) []types.TagWithCount {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	seen := map[string]struct{}{}
	out := make([]types.TagWithCount, 0, limit)
	for _, group := range groups {
		for _, item := range group {
			if _, ok := seen[item.Tag]; ok {
				continue
			}
			seen[item.Tag] = struct{}{}
			out = append(out, item)
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func excludedSuggestionTags(existing string) map[string]struct{} {
	out := map[string]struct{}{}
	if strings.TrimSpace(existing) == "" {
		return out
	}
	ast, err := query.Parse(existing)
	if err != nil {
		return out
	}
	for _, tag := range query.ExtractTags(ast) {
		out[tag] = struct{}{}
	}
	return out
}

func normalizeFileSort(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "added", "modified", "name", "size", "kind":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "added"
	}
}

func normalizeSortOrder(value string) string {
	if strings.EqualFold(strings.TrimSpace(value), "asc") {
		return "asc"
	}
	return "desc"
}

func mediaTypeForPath(path string) string {
	extension := strings.ToLower(filepath.Ext(path))
	if extension == ".cbz" {
		return "application/vnd.comicbook+zip"
	}
	if typ := mime.TypeByExtension(extension); typ != "" {
		return typ
	}
	return "application/octet-stream"
}

func mediaKindForType(mediaType string) string {
	baseType := strings.TrimSpace(strings.SplitN(mediaType, ";", 2)[0])
	switch {
	case strings.EqualFold(baseType, "application/vnd.comicbook+zip"):
		return "comic"
	case strings.EqualFold(baseType, "application/pdf"):
		return "pdf"
	case mediaType == "image/gif":
		return "gif"
	case strings.HasPrefix(mediaType, "image/"):
		return "photo"
	case strings.HasPrefix(mediaType, "video/"):
		return "video"
	case strings.HasPrefix(mediaType, "audio/"):
		return "audio"
	default:
		return "other"
	}
}

func viewerSupportForMediaKind(mediaKind string) string {
	switch strings.ToLower(strings.TrimSpace(mediaKind)) {
	case "photo", "gif", "video", "audio", "comic":
		return "supported"
	default:
		return "unsupported_media_type"
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
