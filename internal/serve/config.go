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
	Token                        string `yaml:"token"`
	TokenEnv                     string `yaml:"token_env"`
	TokenFile                    string `yaml:"token_file"`
	AllowUnsafeNoAuthNonLoopback bool   `yaml:"allow_unsafe_no_auth_non_loopback"`
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
		Uploads:  UploadsConfig{Enabled: false},
		Media: MediaConfig{
			ThumbnailSizes:  []int{256, 512},
			ThumbnailFormat: "jpeg",
			PreviewSize:     1280,
		},
		Jobs: JobsConfig{
			CompletedTTLRaw: "1h",
			CompletedTTL:    time.Hour,
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
		token, err := normalizeToken("auth token override", overrides.AuthToken)
		if err != nil {
			return Config{}, err
		}
		cfg.Auth.Token = token
		cfg.Auth.TokenEnv = ""
		cfg.Auth.TokenFile = ""
	} else {
		if err := cfg.ResolveSecrets(); err != nil {
			return Config{}, err
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (cfg *Config) ResolveSecrets() error {
	sources := 0
	if cfg.Auth.Token != "" {
		token, err := normalizeToken("auth.token", cfg.Auth.Token)
		if err != nil {
			return err
		}
		cfg.Auth.Token = token
		sources++
	}
	if cfg.Auth.TokenEnv != "" {
		sources++
	}
	if cfg.Auth.TokenFile != "" {
		sources++
	}
	if sources > 1 {
		return errors.New("configure only one auth token source: auth.token, auth.token_env, or auth.token_file")
	}
	if cfg.Auth.TokenEnv != "" {
		value, ok := os.LookupEnv(cfg.Auth.TokenEnv)
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("auth.token_env %q is set but the environment variable is empty or unset", cfg.Auth.TokenEnv)
		}
		cfg.Auth.Token = strings.TrimSpace(value)
	}
	if cfg.Auth.TokenFile != "" {
		data, err := os.ReadFile(cfg.Auth.TokenFile)
		if err != nil {
			return fmt.Errorf("read auth.token_file %q: %w", cfg.Auth.TokenFile, err)
		}
		token := strings.TrimSpace(string(data))
		if token == "" {
			return fmt.Errorf("auth.token_file %q is empty", cfg.Auth.TokenFile)
		}
		cfg.Auth.Token = token
	}
	return nil
}

func normalizeToken(name string, value string) (string, error) {
	token := strings.TrimSpace(value)
	if token == "" {
		return "", fmt.Errorf("%s must not be blank", name)
	}
	return token, nil
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
	if cfg.Auth.Token == "" && !cfg.Auth.AllowUnsafeNoAuthNonLoopback && !isLoopbackListen(cfg.Server.Listen) {
		errs = append(errs, errors.New("refusing unauthenticated non-loopback server.listen; set auth.token/token_env/token_file or auth.allow_unsafe_no_auth_non_loopback"))
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
