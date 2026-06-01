package serve

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const DefaultListenAddress = "127.0.0.1:5678"

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	Auth     AuthConfig     `yaml:"auth"`
	Uploads  UploadsConfig  `yaml:"uploads"`
	Media    MediaConfig    `yaml:"media"`
	Jobs     JobsConfig     `yaml:"jobs"`
	Tools    ToolsConfig    `yaml:"tools"`
	Logging  LoggingConfig  `yaml:"logging"`
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
	Enabled          bool              `yaml:"enabled"`
	Directories      []UploadDirectory `yaml:"directories"`
	MaxFileSizeBytes int64             `yaml:"max_file_size_bytes"`
}

type UploadDirectory struct {
	Path string `yaml:"path"`
	Name string `yaml:"name"`
}

type MediaConfig struct {
	CacheDir        string `yaml:"cache_dir"`
	ThumbnailSizes  []int  `yaml:"thumbnail_sizes"`
	ThumbnailFormat string `yaml:"thumbnail_format"`
	PreviewSize     int    `yaml:"preview_size"`
}

type JobsConfig struct {
	CompletedTTLRaw string        `yaml:"completed_ttl"`
	CompletedTTL    time.Duration `yaml:"-"`
	MaxQueued       int           `yaml:"max_queued"`
	MaxRunning      int           `yaml:"max_running"`
	MaxResultBytes  int64         `yaml:"max_result_bytes"`
}

type ToolsConfig struct {
	FFmpegPath  string `yaml:"ffmpeg_path"`
	FFprobePath string `yaml:"ffprobe_path"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
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
			MaxRequestBodyBytes: 32 << 20,
			ReadTimeout:         15 * time.Second,
			WriteTimeout:        30 * time.Second,
			IdleTimeout:         2 * time.Minute,
		},
		Database: DatabaseConfig{Path: dbPath},
		Auth: AuthConfig{
			Enabled:        true,
			SessionTTLRaw:  "720h",
			SessionTTL:     720 * time.Hour,
			CookieName:     "gooru_session",
			CookieSecure:   "auto",
			CookieSameSite: "lax",
		},
		Uploads: UploadsConfig{Enabled: false},
		Media: MediaConfig{
			ThumbnailSizes:  []int{256, 512},
			ThumbnailFormat: "jpeg",
			PreviewSize:     1280,
		},
		Jobs: JobsConfig{
			CompletedTTLRaw: "1h",
			CompletedTTL:    time.Hour,
			MaxQueued:       100,
			MaxRunning:      2,
			MaxResultBytes:  10 << 20,
		},
		Tools:   ToolsConfig{FFmpegPath: "ffmpeg", FFprobePath: "ffprobe"},
		Logging: LoggingConfig{Level: "info"},
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
	if cfg.Server.MaxRequestBodyBytes <= 0 {
		errs = append(errs, errors.New("server.max_request_body_bytes must be greater than zero"))
	}
	if cfg.Media.CacheDir != "" {
		if !filepath.IsAbs(cfg.Media.CacheDir) {
			errs = append(errs, errors.New("media.cache_dir must be absolute when set"))
		}
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
	if cfg.Media.ThumbnailFormat != "jpeg" && cfg.Media.ThumbnailFormat != "png" {
		errs = append(errs, errors.New("media.thumbnail_format must be one of: jpeg, png"))
	}
	if cfg.Jobs.CompletedTTLRaw == "" {
		cfg.Jobs.CompletedTTLRaw = "1h"
	}
	ttl, err := time.ParseDuration(cfg.Jobs.CompletedTTLRaw)
	if err != nil {
		errs = append(errs, fmt.Errorf("jobs.completed_ttl must be a duration such as 1h: %w", err))
	} else if ttl <= 0 {
		errs = append(errs, errors.New("jobs.completed_ttl must be greater than zero"))
	} else {
		cfg.Jobs.CompletedTTL = ttl
	}
	if cfg.Jobs.MaxQueued <= 0 {
		errs = append(errs, errors.New("jobs.max_queued must be greater than zero"))
	}
	if cfg.Jobs.MaxRunning <= 0 {
		errs = append(errs, errors.New("jobs.max_running must be greater than zero"))
	}
	if cfg.Jobs.MaxResultBytes <= 0 {
		errs = append(errs, errors.New("jobs.max_result_bytes must be greater than zero"))
	}
	if cfg.Uploads.Enabled && !hasUploadDirectory(cfg.Uploads.Directories) {
		errs = append(errs, errors.New("uploads.enabled requires at least one uploads.directories entry with a non-empty path"))
	}
	for _, dir := range cfg.Uploads.Directories {
		if strings.TrimSpace(dir.Path) == "" {
			continue
		}
		if !filepath.IsAbs(dir.Path) {
			errs = append(errs, fmt.Errorf("uploads directory %q path must be absolute", dir.Name))
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
	return errors.Join(errs...)
}

func hasUploadDirectory(dirs []UploadDirectory) bool {
	for _, dir := range dirs {
		if strings.TrimSpace(dir.Path) != "" {
			return true
		}
	}
	return false
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
