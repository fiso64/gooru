package serve

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gooru.local/internal/query"
	"gooru.local/internal/securekey"
	"gopkg.in/yaml.v3"
)

const (
	DefaultListenAddress  = "127.0.0.1:5678"
	DefaultGridSize       = 200
	DefaultGridType       = "square"
	DefaultPaginationMode = "infinite"
	DefaultItemsPerPage   = 60
	DefaultUITheme        = "default"
	MinGridSize           = 64
	MaxGridSize           = 1024
)

type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Database   DatabaseConfig   `yaml:"database"`
	Encryption EncryptionConfig `yaml:"encryption"`
	Auth       AuthConfig       `yaml:"auth"`
	Uploads    UploadsConfig    `yaml:"uploads"`
	Media      MediaConfig      `yaml:"media"`
	Tools      ToolsConfig      `yaml:"tools"`
	Logging    LoggingConfig    `yaml:"logging"`
	UI         UIConfig         `yaml:"ui"`
}

type ServerConfig struct {
	Listen              string        `yaml:"listen"`
	PublicURL           string        `yaml:"public_url"`
	CORSOrigins         []string      `yaml:"cors_origins"`
	ExposePaths         bool          `yaml:"expose_paths"`
	FrontendDir         string        `yaml:"frontend_dir"`
	MaxRequestBodyBytes int64         `yaml:"max_request_body_bytes"`
	ReadTimeout         time.Duration `yaml:"-"`
	WriteTimeout        time.Duration `yaml:"-"`
	IdleTimeout         time.Duration `yaml:"-"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type EncryptionConfig struct {
	Enabled        bool   `yaml:"enabled"`
	KeyFile        string `yaml:"key_file"`
	OpaqueURLState bool   `yaml:"opaque_url_state"`
	Key            []byte `yaml:"-"`
}

type AuthConfig struct {
	Enabled                      bool          `yaml:"enabled"`
	SessionTTLRaw                string        `yaml:"session_ttl"`
	SessionTTL                   time.Duration `yaml:"-"`
	CookieName                   string        `yaml:"cookie_name"`
	CookieSecure                 string        `yaml:"cookie_secure"`
	CookieSameSite               string        `yaml:"cookie_same_site"`
	AllowUnsafeNoAuthNonLoopback bool          `yaml:"allow_unsafe_no_auth_non_loopback"`
}

type UploadsConfig struct {
	Enabled          bool           `yaml:"enabled"`
	Targets          []UploadTarget `yaml:"targets"`
	MaxFileSizeBytes int64          `yaml:"max_file_size_bytes"`
	PreserveModTime  bool           `yaml:"preserve_modtime"`
}

type UploadTarget struct {
	ID              string   `yaml:"id"`
	Name            string   `yaml:"name"`
	Path            string   `yaml:"path"`
	AddedAtStrategy string   `yaml:"added_at_strategy"`
	DefaultTags     []string `yaml:"default_tags"`
}

type MediaConfig struct {
	CacheDir           string `yaml:"cache_dir"`
	ThumbnailSizes     []int  `yaml:"thumbnail_sizes"`
	ThumbnailFormat    string `yaml:"thumbnail_format"`
	PreviewSize        int    `yaml:"preview_size"`
	PreviewEnabled     bool   `yaml:"preview_enabled"`
	PreviewJPEGQuality int    `yaml:"preview_jpeg_quality"`
	LosslessJPEGTranscode bool `yaml:"lossless_jpeg_transcode"`
	TranscodeCBZPages bool `yaml:"transcode_cbz_pages"`
}

type ToolsConfig struct {
	FFmpegPath  string `yaml:"ffmpeg_path"`
	FFprobePath string `yaml:"ffprobe_path"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
}

type UIConfig struct {
	Theme                    string   `yaml:"theme"`
	AccentColor              string   `yaml:"accent_color"`
	FontStyle                string   `yaml:"font_style"`
	FontStyleConfigured      bool     `yaml:"-"`
	GridSize                 int      `yaml:"grid_size"`
	GridType                 string   `yaml:"grid_type"`
	HiddenTags               []string `yaml:"hidden_tags"`
	LoadFullMediaByDefault   bool     `yaml:"load_full_media_by_default"`
	PreferLosslessFullImage bool     `yaml:"prefer_lossless_full_image"`
	FullscreenMediaByDefault bool     `yaml:"fullscreen_media_by_default"`
	HoverPlayVideos          bool     `yaml:"hover_play_videos"`
	HoverPlayGIFs            bool     `yaml:"hover_play_gifs"`
	ViewerFitMode            string   `yaml:"viewer_fit_mode"`
	ViewerActualSizeFitCap   bool     `yaml:"viewer_actual_size_fit_cap"`
	ViewerScaling            string   `yaml:"viewer_scaling"`
	PaginationMode           string   `yaml:"pagination_mode"`
	ItemsPerPage             int      `yaml:"items_per_page"`
}

func (cfg LoggingConfig) SlogLevel() slog.Level {
	switch strings.ToLower(strings.TrimSpace(cfg.Level)) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type Overrides struct {
	Listen       string
	PublicURL    string
	AuthToken    string
	DatabasePath string
}

func DefaultConfig(dbPath string) Config {
	return Config{
		Server: ServerConfig{
			Listen:              DefaultListenAddress,
			FrontendDir:         "frontend/build",
			MaxRequestBodyBytes: 0,
			ReadTimeout:         15 * time.Second,
			WriteTimeout:        30 * time.Second,
			IdleTimeout:         2 * time.Minute,
		},
		Database:   DatabaseConfig{Path: dbPath},
		Encryption: EncryptionConfig{OpaqueURLState: true},
		Auth: AuthConfig{
			Enabled:        true,
			SessionTTLRaw:  "720h",
			SessionTTL:     720 * time.Hour,
			CookieName:     "gooru_session",
			CookieSecure:   "auto",
			CookieSameSite: "lax",
		},
		Uploads: UploadsConfig{Enabled: false, PreserveModTime: true},
		Media: MediaConfig{
			ThumbnailSizes:     []int{256, 512},
			ThumbnailFormat:    "jpeg",
			PreviewSize:        1280,
			PreviewEnabled:     true,
			PreviewJPEGQuality: derivativeJPEGQuality,
			LosslessJPEGTranscode: true,
			TranscodeCBZPages: true,
		},
		Tools:   ToolsConfig{FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"},
		Logging: LoggingConfig{Level: "info"},
		UI: UIConfig{
			Theme:                  DefaultUITheme,
			FontStyle:              "comic",
			HoverPlayVideos:        false,
			HoverPlayGIFs:          true,
			GridSize:               DefaultGridSize,
			GridType:               DefaultGridType,
			ViewerFitMode:          "fit_window",
			ViewerActualSizeFitCap: true,
			ViewerScaling:          "smooth",
			PreferLosslessFullImage: true,
			PaginationMode:         DefaultPaginationMode,
			ItemsPerPage:           DefaultItemsPerPage,
		},
	}
}

func DefaultYAML(dbPath string) ([]byte, error) {
	cfg := DefaultConfig(dbPath)
	return yaml.Marshal(cfg)
}

func LoadConfig(path string, dbPath string, overrides Overrides) (Config, error) {
	cfg := DefaultConfig(dbPath)
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return Config{}, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := rejectDeprecatedAuthTokenConfig(data); err != nil {
			return Config{}, fmt.Errorf("parse config %q: %w", path, err)
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		decoder.KnownFields(true)
		if err := decoder.Decode(&cfg); err != nil {
			return Config{}, fmt.Errorf("parse config %q: %w", path, err)
		}
	}

	if overrides.Listen != "" {
		cfg.Server.Listen = overrides.Listen
	}
	if overrides.PublicURL != "" {
		cfg.Server.PublicURL = overrides.PublicURL
	}
	if overrides.DatabasePath != "" {
		cfg.Database.Path = overrides.DatabasePath
	}

	if overrides.AuthToken != "" {
		return Config{}, errors.New("--auth-token is no longer supported; create a DB-backed admin with 'gooru user create-admin'")
	}
	if err := cfg.ResolveSecrets(); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg *Config) ResolveSecrets() error {
	cfg.Encryption.KeyFile = strings.TrimSpace(cfg.Encryption.KeyFile)
	if !cfg.Encryption.Enabled {
		cfg.Encryption.Key = nil
		return nil
	}

	if cfg.Encryption.KeyFile != "" {
		_, hasEnvKey := os.LookupEnv(securekey.EnvKey)
		hasEnvKeyFile := strings.TrimSpace(os.Getenv(securekey.EnvKeyFile)) != ""
		if hasEnvKey || hasEnvKeyFile {
			return fmt.Errorf("resolve encryption key: configure exactly one encryption key source: encryption.key_file, %s, or %s", securekey.EnvKey, securekey.EnvKeyFile)
		}
		key, err := securekey.Load(securekey.Source{File: cfg.Encryption.KeyFile})
		if err != nil {
			return fmt.Errorf("resolve encryption key: %w", err)
		}
		cfg.Encryption.Key = key
		return nil
	}

	key, configured, err := securekey.LoadProcess()
	if err != nil {
		return fmt.Errorf("resolve encryption key: %w", err)
	}
	if !configured {
		return fmt.Errorf("encryption.enabled requires an encryption key via encryption.key_file, %s, or %s", securekey.EnvKey, securekey.EnvKeyFile)
	}
	cfg.Encryption.Key = key
	return nil
}

func rejectDeprecatedAuthTokenConfig(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	top := root.Content[0]
	for i := 0; i+1 < len(top.Content); i += 2 {
		if top.Content[i].Value != "auth" || top.Content[i+1].Kind != yaml.MappingNode {
			continue
		}
		auth := top.Content[i+1]
		for j := 0; j+1 < len(auth.Content); j += 2 {
			switch auth.Content[j].Value {
			case "token", "token_env", "token_file":
				return errors.New("auth.token, auth.token_env, and auth.token_file are no longer supported; create DB-backed users with 'gooru user create-admin'")
			}
		}
	}
	return nil
}

func (cfg *Config) Validate() error {
	var errs []error
	if strings.TrimSpace(cfg.Server.Listen) == "" {
		errs = append(errs, errors.New("server.listen is required"))
	} else if _, _, err := net.SplitHostPort(cfg.Server.Listen); err != nil {
		errs = append(errs, fmt.Errorf("server.listen must be host:port: %w", err))
	}
	if strings.TrimSpace(cfg.Database.Path) == "" {
		errs = append(errs, errors.New("database.path is required"))
	}
	if cfg.Encryption.Enabled && len(cfg.Encryption.Key) != securekey.Size {
		errs = append(errs, errors.New("encryption.enabled requires a resolved 256-bit encryption key"))
	}
	if cfg.Server.MaxRequestBodyBytes < 0 {
		errs = append(errs, errors.New("server.max_request_body_bytes must be zero or greater"))
	}
	if cfg.Media.CacheDir != "" && !filepath.IsAbs(cfg.Media.CacheDir) {
		errs = append(errs, errors.New("media.cache_dir must be absolute when set"))
	}
	if len(cfg.Media.ThumbnailSizes) == 0 {
		errs = append(errs, errors.New("media.thumbnail_sizes must contain at least one size"))
	}
	for _, size := range cfg.Media.ThumbnailSizes {
		if size <= 0 || size > 4096 {
			errs = append(errs, fmt.Errorf("media.thumbnail_sizes contains invalid size %d", size))
		}
	}
	if cfg.Media.PreviewSize <= 0 {
		errs = append(errs, errors.New("media.preview_size must be greater than zero"))
	}
	if cfg.Media.PreviewJPEGQuality < 1 || cfg.Media.PreviewJPEGQuality > 100 {
		errs = append(errs, errors.New("media.preview_jpeg_quality must be between 1 and 100"))
	}
	if cfg.Media.ThumbnailFormat != "jpeg" && cfg.Media.ThumbnailFormat != "png" {
		errs = append(errs, errors.New("media.thumbnail_format must be one of: jpeg, png"))
	}
	if cfg.Uploads.Enabled && !hasUploadTarget(cfg.Uploads.Targets) {
		errs = append(errs, errors.New("uploads.enabled requires at least one uploads.targets entry"))
	}
	seenTargets := make(map[string]struct{}, len(cfg.Uploads.Targets))
	for i := range cfg.Uploads.Targets {
		target := cfg.Uploads.Targets[i]
		id := strings.TrimSpace(target.ID)
		name := strings.TrimSpace(target.Name)
		path := strings.TrimSpace(target.Path)
		addedAtStrategy := strings.TrimSpace(target.AddedAtStrategy)
		if addedAtStrategy == "" {
			addedAtStrategy = "queue"
		}
		cfg.Uploads.Targets[i].ID = id
		cfg.Uploads.Targets[i].Name = name
		cfg.Uploads.Targets[i].Path = path
		cfg.Uploads.Targets[i].AddedAtStrategy = addedAtStrategy
		for tagIndex := range cfg.Uploads.Targets[i].DefaultTags {
			value := strings.TrimSpace(cfg.Uploads.Targets[i].DefaultTags[tagIndex])
			cfg.Uploads.Targets[i].DefaultTags[tagIndex] = value
			if err := query.ValidateTag(value); err != nil {
				errs = append(errs, fmt.Errorf("uploads target %q default_tags: %w", id, err))
			}
		}
		if id == "" {
			errs = append(errs, fmt.Errorf("uploads.targets[%d].id is required", i))
		} else if !validUploadTargetID(id) {
			errs = append(errs, fmt.Errorf("uploads target id %q must contain only letters, numbers, underscores, or hyphens", id))
		} else if _, exists := seenTargets[id]; exists {
			errs = append(errs, fmt.Errorf("uploads target id %q is duplicated", id))
		} else {
			seenTargets[id] = struct{}{}
		}
		if name == "" {
			errs = append(errs, fmt.Errorf("uploads target %q name is required", id))
		}
		switch addedAtStrategy {
		case "queue", "reverse_queue", "modtime":
		default:
			errs = append(errs, fmt.Errorf("uploads target %q added_at_strategy must be one of: queue, reverse_queue, modtime", id))
		}
		if path == "" {
			errs = append(errs, fmt.Errorf("uploads target %q path is required", id))
		} else if !filepath.IsAbs(path) {
			errs = append(errs, fmt.Errorf("uploads target %q path must be absolute", id))
		} else if err := cfg.validateUploadTargetProtection(cfg.Uploads.Targets[i]); err != nil {
			errs = append(errs, err)
		}
	}
	if cfg.Auth.SessionTTLRaw == "" {
		cfg.Auth.SessionTTLRaw = "720h"
	}
	sessionTTL, err := time.ParseDuration(cfg.Auth.SessionTTLRaw)
	if err != nil {
		errs = append(errs, fmt.Errorf("auth.session_ttl must be a duration such as 720h: %w", err))
	} else if sessionTTL <= 0 {
		errs = append(errs, errors.New("auth.session_ttl must be greater than zero"))
	} else {
		cfg.Auth.SessionTTL = sessionTTL
	}
	if strings.TrimSpace(cfg.Auth.CookieName) == "" {
		errs = append(errs, errors.New("auth.cookie_name is required"))
	}
	if cfg.Auth.CookieSecure == "" {
		cfg.Auth.CookieSecure = "auto"
	}
	switch strings.ToLower(cfg.Auth.CookieSecure) {
	case "auto", "true", "false":
	default:
		errs = append(errs, errors.New("auth.cookie_secure must be one of: auto, true, false"))
	}
	if cfg.Auth.CookieSameSite == "" {
		cfg.Auth.CookieSameSite = "lax"
	}
	switch strings.ToLower(cfg.Auth.CookieSameSite) {
	case "lax", "strict", "none":
	default:
		errs = append(errs, errors.New("auth.cookie_same_site must be one of: lax, strict, none"))
	}
	if !cfg.Auth.Enabled && !cfg.Auth.AllowUnsafeNoAuthNonLoopback && !isLoopbackListen(cfg.Server.Listen) {
		errs = append(errs, errors.New("refusing auth.enabled=false on non-loopback server.listen; bind to loopback or set auth.allow_unsafe_no_auth_non_loopback for trusted development"))
	}
	cfg.UI.Theme = strings.ToLower(strings.TrimSpace(cfg.UI.Theme))
	if cfg.UI.Theme == "" {
		cfg.UI.Theme = DefaultUITheme
	}
	switch cfg.UI.Theme {
	case "default", "booru-light", "booru-dark":
	default:
		errs = append(errs, errors.New("ui.theme must be one of: default, booru-light, booru-dark"))
	}
	cfg.UI.AccentColor = strings.TrimSpace(cfg.UI.AccentColor)
	if cfg.UI.AccentColor != "" && !accentColorPattern.MatchString(cfg.UI.AccentColor) {
		errs = append(errs, errors.New("ui.accent_color must be a six-digit hex color such as #2f80ed"))
	}
	cfg.UI.FontStyle = strings.ToLower(strings.TrimSpace(cfg.UI.FontStyle))
	if cfg.UI.FontStyle == "" {
		cfg.UI.FontStyle = "comic"
	}
	switch cfg.UI.FontStyle {
	case "editorial", "modern", "comic":
	default:
		errs = append(errs, errors.New("ui.font_style must be one of: editorial, modern, comic"))
	}
	if cfg.UI.GridSize < MinGridSize || cfg.UI.GridSize > MaxGridSize {
		errs = append(errs, fmt.Errorf("ui.grid_size must be between %d and %d pixels", MinGridSize, MaxGridSize))
	}
	cfg.UI.GridType = strings.ToLower(strings.TrimSpace(cfg.UI.GridType))
	if cfg.UI.GridType == "" {
		cfg.UI.GridType = DefaultGridType
	}
	switch cfg.UI.GridType {
	case "square", "fit", "tile":
	default:
		errs = append(errs, errors.New("ui.grid_type must be one of: square, fit, tile"))
	}
	normalizedHiddenTags := make([]string, 0, len(cfg.UI.HiddenTags))
	seenHiddenTags := make(map[string]struct{}, len(cfg.UI.HiddenTags))
	for i, rawTag := range cfg.UI.HiddenTags {
		tag := strings.TrimSpace(rawTag)
		if err := query.ValidateTag(tag); err != nil {
			errs = append(errs, fmt.Errorf("ui.hidden_tags[%d]: %w", i, err))
			continue
		}
		if _, exists := seenHiddenTags[tag]; exists {
			continue
		}
		seenHiddenTags[tag] = struct{}{}
		normalizedHiddenTags = append(normalizedHiddenTags, tag)
	}
	cfg.UI.HiddenTags = normalizedHiddenTags
	cfg.UI.ViewerFitMode = strings.ToLower(strings.TrimSpace(cfg.UI.ViewerFitMode))
	if cfg.UI.ViewerFitMode == "" {
		cfg.UI.ViewerFitMode = "fit_window"
	}
	if cfg.UI.ViewerFitMode == "screen" {
		cfg.UI.ViewerFitMode = "fit_window"
	}
	switch cfg.UI.ViewerFitMode {
	case "fit_window", "fit_down_only", "original_size_if_fit", "actual":
	default:
		errs = append(errs, errors.New("ui.viewer_fit_mode must be one of: fit_window, fit_down_only, original_size_if_fit, actual"))
	}
	cfg.UI.ViewerScaling = strings.ToLower(strings.TrimSpace(cfg.UI.ViewerScaling))
	if cfg.UI.ViewerScaling == "" {
		cfg.UI.ViewerScaling = "smooth"
	}
	switch cfg.UI.ViewerScaling {
	case "smooth", "nearest":
	default:
		errs = append(errs, errors.New("ui.viewer_scaling must be one of: smooth, nearest"))
	}
	cfg.UI.PaginationMode = strings.ToLower(strings.TrimSpace(cfg.UI.PaginationMode))
	if cfg.UI.PaginationMode == "" {
		cfg.UI.PaginationMode = DefaultPaginationMode
	}
	switch cfg.UI.PaginationMode {
	case "infinite", "paged":
	default:
		errs = append(errs, errors.New("ui.pagination_mode must be one of: infinite, paged"))
	}
	if cfg.UI.ItemsPerPage <= 0 || cfg.UI.ItemsPerPage > MaxPageLimit {
		errs = append(errs, fmt.Errorf("ui.items_per_page must be between 1 and %d", MaxPageLimit))
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	cfg.Logging.Level = strings.ToLower(strings.TrimSpace(cfg.Logging.Level))
	switch cfg.Logging.Level {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, errors.New("logging.level must be one of: debug, info, warn, error"))
	}
	return errors.Join(errs...)
}

func hasUploadTarget(targets []UploadTarget) bool {
	for _, target := range targets {
		if strings.TrimSpace(target.ID) != "" && strings.TrimSpace(target.Path) != "" {
			return true
		}
	}
	return false
}

var uploadTargetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)
var accentColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

func validUploadTargetID(id string) bool {
	return uploadTargetIDPattern.MatchString(id)
}

func isLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
