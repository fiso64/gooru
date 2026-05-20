package serve

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	core "gooru.local/gooru"
	"gooru.local/types"
)

var ErrNotFound = errors.New("not found")

type Library interface {
	ListFiles(ctx context.Context, query string) ([]types.FileInfo, error)
	GetFile(ctx context.Context, locationID int64) (types.FileInfo, error)
	ListTags(ctx context.Context, counts bool) ([]TagDTO, error)
}

type GooruLibrary struct {
	client  *core.Client
	verbose bool
}

func NewGooruLibrary(client *core.Client, verbose bool) *GooruLibrary {
	return &GooruLibrary{client: client, verbose: verbose}
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

func (l *GooruLibrary) GetFile(ctx context.Context, locationID int64) (types.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return types.FileInfo{}, err
	}
	file, err := l.client.GetFileInfoByLocationID(locationID)
	if errors.Is(err, sql.ErrNoRows) {
		return types.FileInfo{}, ErrNotFound
	}
	return file, err
}

func (l *GooruLibrary) ListTags(ctx context.Context, counts bool) ([]TagDTO, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if counts {
		tags, err := l.client.GetAllTagsWithCounts()
		if err != nil {
			return nil, err
		}
		out := make([]TagDTO, 0, len(tags))
		for _, tag := range tags {
			count := tag.Count
			out = append(out, TagDTO{Name: tag.Tag, Count: &count})
		}
		return out, nil
	}
	tags, err := l.client.GetAllTags()
	if err != nil {
		return nil, err
	}
	out := make([]TagDTO, 0, len(tags))
	for _, tag := range tags {
		out = append(out, TagDTO{Name: tag})
	}
	return out, nil
}

type FileListResponse struct {
	Files         []FileDTO `json:"files"`
	NextPageToken string    `json:"next_page_token,omitempty"`
}

type TagListResponse struct {
	Tags []TagDTO `json:"tags"`
}

type FileDTO struct {
	ID           string    `json:"id"`
	ContentID    string    `json:"content_id"`
	Name         string    `json:"name"`
	Path         string    `json:"path,omitempty"`
	Size         int64     `json:"size"`
	ModifiedTime time.Time `json:"modified_time"`
	MediaType    string    `json:"media_type"`
	MediaKind    string    `json:"media_kind"`
	Tags         []string  `json:"tags"`
	MediaURLs    MediaURLs `json:"media_urls"`
}

type MediaURLs struct {
	Thumbnail string `json:"thumbnail"`
	Preview   string `json:"preview"`
	Content   string `json:"content"`
}

type TagDTO struct {
	Name  string `json:"name"`
	Count *int   `json:"count,omitempty"`
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	page, err := ParsePage(r.URL.Query().Get("limit"), r.URL.Query().Get("page_token"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), nil)
		return
	}
	files, err := s.library.ListFiles(r.Context(), r.URL.Query().Get("query"))
	if err != nil {
		if errors.Is(err, core.ErrInvalidQuery) {
			writeError(w, http.StatusBadRequest, "invalid_query", err.Error(), nil)
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to list files", nil)
		return
	}

	start := page.Offset
	if start > len(files) {
		start = len(files)
	}
	end := start + page.Limit
	if end > len(files) {
		end = len(files)
	}
	pageFiles := files[start:end]
	response := FileListResponse{
		Files: make([]FileDTO, 0, len(pageFiles)),
	}
	if end < len(files) {
		response.NextPageToken = NextPageToken(page.Offset, page.Limit, len(pageFiles))
	}
	for _, file := range pageFiles {
		response.Files = append(response.Files, s.fileDTO(file))
	}
	writeJSON(w, http.StatusOK, response)
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
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed", nil)
		return
	}
	locationID, err := DecodeFileID(parts[0])
	if err != nil {
		writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		return
	}
	file, err := s.library.GetFile(r.Context(), locationID)
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
		case "thumbnail":
			s.media.ServeDerivative(w, r, file, "thumbnail")
		case "preview":
			s.media.ServeDerivative(w, r, file, "preview")
		default:
			writeError(w, http.StatusNotFound, "not_found", "file not found", nil)
		}
		return
	}
	writeJSON(w, http.StatusOK, s.fileDTO(file))
}

func (s *Server) handleListTags(w http.ResponseWriter, r *http.Request) {
	if s.library == nil {
		writeError(w, http.StatusServiceUnavailable, "service_unavailable", "file library is not configured", nil)
		return
	}
	tags, err := s.library.ListTags(r.Context(), r.URL.Query().Get("counts") == "true")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to load tags", nil)
		return
	}
	writeJSON(w, http.StatusOK, TagListResponse{Tags: tags})
}

func EncodeFileID(id int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("loc:%d", id)))
}

func DecodeFileID(encoded string) (int64, error) {
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, err
	}
	value := string(raw)
	if !strings.HasPrefix(value, "loc:") {
		return 0, fmt.Errorf("invalid file id")
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, "loc:"), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid file id")
	}
	return id, nil
}

func (s *Server) fileDTO(file types.FileInfo) FileDTO {
	id := EncodeFileID(file.ID)
	mediaType := mediaTypeForPath(file.Path)
	dto := FileDTO{
		ID:           id,
		ContentID:    file.Hash,
		Name:         filepath.Base(file.Path),
		Size:         file.Size,
		ModifiedTime: time.Unix(file.ModTime, 0).UTC(),
		MediaType:    mediaType,
		MediaKind:    mediaKindForType(mediaType),
		Tags:         nonNilStrings(file.Tags),
		MediaURLs: MediaURLs{
			Thumbnail: "/api/v1/files/" + id + "/thumbnail",
			Preview:   "/api/v1/files/" + id + "/preview",
			Content:   "/api/v1/files/" + id + "/content",
		},
	}
	if s.cfg.Server.ExposePaths {
		dto.Path = file.Path
	}
	return dto
}

func mediaTypeForPath(path string) string {
	if typ := mime.TypeByExtension(strings.ToLower(filepath.Ext(path))); typ != "" {
		return typ
	}
	return "application/octet-stream"
}

func mediaKindForType(mediaType string) string {
	switch {
	case strings.HasPrefix(mediaType, "image/"):
		return "image"
	case strings.HasPrefix(mediaType, "video/"):
		return "video"
	case strings.HasPrefix(mediaType, "audio/"):
		return "audio"
	default:
		return "other"
	}
}

func nonNilStrings(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
