package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sakuradairong/smartstrm-cleanroom/internal/cronexpr"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/pathpolicy"
	"github.com/sakuradairong/smartstrm-cleanroom/internal/urlpolicy"
)

const CurrentVersion = 1

var RedactedSecret = strings.Repeat("*", 8)

type Config struct {
	Version      int               `json:"version"`
	Listen       string            `json:"listen"`
	PublicURL    string            `json:"public_url"`
	WebhookToken string            `json:"webhook_token"`
	Admin        AdminConfig       `json:"admin"`
	Storages     []StorageConfig   `json:"storages"`
	Tasks        []TaskConfig      `json:"tasks"`
	Plugins      []PluginConfig    `json:"plugins"`
	Integrations IntegrationConfig `json:"integrations"`
	TMDB         TMDBConfig        `json:"tmdb"`
	MediaTools   MediaToolsConfig  `json:"media_tools"`
	History      HistoryConfig     `json:"history"`
	MediaProxy   MediaProxyConfig  `json:"media_proxy"`
	Path         string            `json:"-"`
}

// ManagedConfig contains settings that are edited through the management UI.
// Bootstrap settings such as listen addresses and administrator credentials are
// intentionally excluded so this value cannot overwrite them.
type ManagedConfig struct {
	Storages     []StorageConfig   `json:"storages"`
	Tasks        []TaskConfig      `json:"tasks"`
	Plugins      []PluginConfig    `json:"plugins"`
	Integrations IntegrationConfig `json:"integrations"`
	TMDB         TMDBConfig        `json:"tmdb"`
	MediaTools   MediaToolsConfig  `json:"media_tools"`
	History      HistoryConfig     `json:"history"`
	MediaProxy   MediaProxyConfig  `json:"media_proxy"`
}

type MediaProxyConfig struct {
	Enabled             bool   `json:"enabled"`
	Listen              string `json:"listen"`
	Upstream            string `json:"upstream"`
	ServerType          string `json:"server_type"`
	RewritePlaybackInfo bool   `json:"rewrite_playback_info"`
}

type HistoryConfig struct {
	Path       string `json:"path"`
	MaxEntries int    `json:"max_entries"`
}

type MediaToolsConfig struct {
	FFmpeg         string `json:"ffmpeg"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type TMDBConfig struct {
	APIKey       string `json:"api_key"`
	Language     string `json:"language"`
	CacheMinutes int    `json:"cache_minutes,omitempty"`
	BaseURL      string `json:"base_url,omitempty"`
	ImageBaseURL string `json:"image_base_url,omitempty"`
	ProxyURL     string `json:"proxy_url,omitempty"`
}

type IntegrationConfig struct {
	CloudDrive2 map[string]string `json:"clouddrive2"`
	MoviePilot  map[string]string `json:"moviepilot"`
}

type AdminConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type StorageConfig struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Root     string `json:"root"`
	Endpoint string `json:"endpoint"`
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

type TaskConfig struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	StorageID    string         `json:"storage_id"`
	Source       string         `json:"source"`
	Destination  string         `json:"destination"`
	Schedule     string         `json:"schedule"`
	FileIDMode   bool           `json:"file_id_mode"`
	DirTimeCheck bool           `json:"dir_time_check"`
	Watch        bool           `json:"watch"`
	Incremental  bool           `json:"incremental"`
	SyncDelete   bool           `json:"sync_delete"`
	MediaExt     []string       `json:"media_ext"`
	CopyExt      []string       `json:"copy_ext"`
	MinSize      int64          `json:"min_size"`
	MaxSize      int64          `json:"max_size"`
	Plugins      []PluginConfig `json:"plugins"`
	KeepLocal    []string       `json:"keep_local"`
}

type PluginConfig struct {
	Type          string `json:"type"`
	Pattern       string `json:"pattern"`
	Replacement   string `json:"replacement"`
	Template      string `json:"template,omitempty"`
	MatchMode     string `json:"match_mode,omitempty"`
	FilterMode    string `json:"filter_mode,omitempty"`
	DirectoryOnly bool   `json:"directory_only,omitempty"`
	CaseSensitive bool   `json:"case_sensitive,omitempty"`
	DelayMS       int    `json:"delay_ms,omitempty"`
	MaxBytes      int    `json:"max_bytes,omitempty"`
}

func Load(path string) (Config, error) {
	cfg, err := LoadDisk(path)
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var metadata struct {
		Version *int `json:"version"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Config{}, err
	}
	if metadata.Version == nil {
		if err := Save(path, cfg); err != nil {
			return Config{}, fmt.Errorf("migrate config to version %d: %w", CurrentVersion, err)
		}
		cfg.Path = path
	}
	applyEnv(&cfg)
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func LoadDisk(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if cfg.Version != CurrentVersion {
		return Config{}, fmt.Errorf("unsupported config version %d (current %d)", cfg.Version, CurrentVersion)
	}
	cfg.Path = path
	if cfg.Listen == "" {
		cfg.Listen = ":8024"
	}
	for index := range cfg.Tasks {
		if absolute, absErr := filepath.Abs(cfg.Tasks[index].Destination); absErr == nil {
			cfg.Tasks[index].Destination = absolute
		}
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func Save(path string, cfg Config) error {
	if path == "" {
		return errors.New("configuration path is not available")
	}
	cfg.Version, cfg.Path = CurrentVersion, ""
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate configuration: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encode configuration: %w", err)
	}
	data = append(data, '\n')
	if err := writeAtomic(path, data, true); err != nil {
		return fmt.Errorf("write configuration: %w", err)
	}
	return nil
}

func Restore(path string) (Config, error) {
	backup := path + ".bak"
	cfg, err := LoadDisk(backup)
	if err != nil {
		return Config{}, fmt.Errorf("validate backup: %w", err)
	}
	cfg.Version, cfg.Path = CurrentVersion, ""
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return Config{}, err
	}
	data = append(data, '\n')
	if err := writeAtomic(path, data, false); err != nil {
		return Config{}, err
	}
	cfg.Path = path
	return cfg, nil
}

func writeAtomic(path string, data []byte, backup bool) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	if backup {
		if current, err := os.ReadFile(path); err == nil {
			if err := os.WriteFile(path+".bak.tmp", current, 0o600); err != nil {
				return err
			}
			if err := os.Rename(path+".bak.tmp", path+".bak"); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	temporary, err := os.CreateTemp(directory, ".smartstrm-config-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return err
	}
	dir, err := os.Open(directory)
	if err == nil {
		defer dir.Close()
		err = dir.Sync()
	}
	return err
}

func (c Config) Redacted() Config {
	c.Path = ""
	c.Storages = append([]StorageConfig(nil), c.Storages...)
	if c.WebhookToken != "" {
		c.WebhookToken = RedactedSecret
	}
	if c.Admin.Password != "" {
		c.Admin.Password = RedactedSecret
	}
	if c.TMDB.APIKey != "" {
		c.TMDB.APIKey = RedactedSecret
	}
	for index := range c.Storages {
		if c.Storages[index].Password != "" {
			c.Storages[index].Password = RedactedSecret
		}
		if c.Storages[index].Token != "" {
			c.Storages[index].Token = RedactedSecret
		}
	}
	return c
}

func (c Config) Managed() ManagedConfig {
	return ManagedConfig{
		Storages:     c.Storages,
		Tasks:        c.Tasks,
		Plugins:      c.Plugins,
		Integrations: c.Integrations,
		TMDB:         c.TMDB,
		MediaTools:   c.MediaTools,
		History:      c.History,
		MediaProxy:   c.MediaProxy,
	}
}

func (c *Config) ApplyManaged(managed ManagedConfig) {
	c.Storages = managed.Storages
	c.Tasks = managed.Tasks
	c.Plugins = managed.Plugins
	c.Integrations = managed.Integrations
	c.TMDB = managed.TMDB
	c.MediaTools = managed.MediaTools
	c.History = managed.History
	c.MediaProxy = managed.MediaProxy
}

func PreserveSecrets(candidate *Config, current Config) {
	if candidate.WebhookToken == "" || candidate.WebhookToken == RedactedSecret {
		candidate.WebhookToken = current.WebhookToken
	}
	if candidate.Admin.Password == "" || candidate.Admin.Password == RedactedSecret {
		candidate.Admin.Password = current.Admin.Password
	}
	if candidate.TMDB.APIKey == "" || candidate.TMDB.APIKey == RedactedSecret {
		candidate.TMDB.APIKey = current.TMDB.APIKey
	}
	existing := make(map[string]StorageConfig, len(current.Storages))
	for _, item := range current.Storages {
		existing[item.ID] = item
	}
	for index := range candidate.Storages {
		previous := existing[candidate.Storages[index].ID]
		if candidate.Storages[index].Password == "" || candidate.Storages[index].Password == RedactedSecret {
			candidate.Storages[index].Password = previous.Password
		}
		if candidate.Storages[index].Token == "" || candidate.Storages[index].Token == RedactedSecret {
			candidate.Storages[index].Token = previous.Token
		}
	}
}

func (c Config) Validate() error {
	if c.Version != 0 && c.Version != CurrentVersion {
		return fmt.Errorf("unsupported config version %d", c.Version)
	}
	if c.WebhookToken == "" {
		return errors.New("webhook_token must not be empty")
	}
	if c.PublicURL == "" {
		return errors.New("public_url must not be empty")
	}
	publicURL, err := url.Parse(c.PublicURL)
	if err != nil || (publicURL.Scheme != "http" && publicURL.Scheme != "https") || publicURL.Host == "" || publicURL.User != nil || publicURL.RawQuery != "" || publicURL.Fragment != "" {
		return errors.New("public_url must be an absolute HTTP(S) URL")
	}
	if c.Admin.Username != "" && c.Admin.Password == "" {
		return errors.New("admin.password must not be empty when admin.username is set")
	}
	if c.History.Path != "" {
		cleaned := filepath.Clean(c.History.Path)
		if !filepath.IsAbs(cleaned) || cleaned == string(filepath.Separator) {
			return errors.New("history.path must be an absolute non-root path")
		}
	}
	if c.History.MaxEntries < 0 || c.History.MaxEntries > 100000 {
		return errors.New("history.max_entries must be between 0 and 100000")
	}
	if c.MediaProxy.Enabled {
		if c.MediaProxy.Listen == "" {
			return errors.New("media_proxy.listen must not be empty when enabled")
		}
		if _, err := net.ResolveTCPAddr("tcp", c.MediaProxy.Listen); err != nil {
			return fmt.Errorf("media_proxy.listen is invalid: %w", err)
		}
		upstream, err := url.Parse(c.MediaProxy.Upstream)
		if err != nil || (upstream.Scheme != "http" && upstream.Scheme != "https") || upstream.Host == "" || upstream.User != nil || upstream.RawQuery != "" || upstream.Fragment != "" {
			return errors.New("media_proxy.upstream must be an HTTP(S) URL without credentials, query, or fragment")
		}
		if c.MediaProxy.ServerType != "" && c.MediaProxy.ServerType != "emby" && c.MediaProxy.ServerType != "jellyfin" {
			return errors.New("media_proxy.server_type must be emby or jellyfin")
		}
		if c.MediaProxy.RewritePlaybackInfo && c.MediaProxy.ServerType == "" {
			return errors.New("media_proxy.server_type is required when rewrite_playback_info is enabled")
		}
	}
	storageIDs := make(map[string]struct{}, len(c.Storages))
	for _, storage := range c.Storages {
		if storage.ID == "" || storage.Type == "" {
			return errors.New("every storage needs id and type")
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(storage.ID) {
			return fmt.Errorf("storage id %q may only contain letters, numbers, _ and -", storage.ID)
		}
		if _, exists := storageIDs[storage.ID]; exists {
			return fmt.Errorf("duplicate storage id %q", storage.ID)
		}
		switch strings.ToLower(storage.Type) {
		case "local":
			if _, err := pathpolicy.AbsoluteNonRoot(storage.Root); err != nil {
				return fmt.Errorf("storage %q root: %w", storage.ID, err)
			}
		case "webdav", "openlist", "quark", "115", "189", "123":
			if _, err := urlpolicy.ParseHTTP(storage.Endpoint, false); err != nil {
				return fmt.Errorf("storage %q endpoint: %w", storage.ID, err)
			}
		case "ani_open", "ani-open":
			if storage.Endpoint != "" {
				if _, err := urlpolicy.ParseHTTP(storage.Endpoint, true); err != nil {
					return fmt.Errorf("storage %q endpoint: %w", storage.ID, err)
				}
			}
		default:
			return fmt.Errorf("storage %q has unsupported type %q", storage.ID, storage.Type)
		}
		storageIDs[storage.ID] = struct{}{}
	}
	taskIDs := make(map[string]struct{}, len(c.Tasks))
	if err := validatePlugins("global", c.Plugins); err != nil {
		return err
	}
	for _, task := range c.Tasks {
		if task.ID == "" || task.StorageID == "" || task.Destination == "" {
			return errors.New("every task needs id, storage_id and destination")
		}
		if _, exists := taskIDs[task.ID]; exists {
			return fmt.Errorf("duplicate task id %q", task.ID)
		}
		taskIDs[task.ID] = struct{}{}
		if _, exists := storageIDs[task.StorageID]; !exists {
			return fmt.Errorf("task %q references unknown storage %q", task.ID, task.StorageID)
		}
		destination := filepath.Clean(task.Destination)
		if !filepath.IsAbs(destination) || destination == string(filepath.Separator) {
			return fmt.Errorf("task %q destination must be an absolute non-root path", task.ID)
		}
		if err := validatePlugins("task "+task.ID, task.Plugins); err != nil {
			return err
		}
		if task.Schedule != "" {
			if _, err := cronexpr.Parse(task.Schedule); err != nil {
				return fmt.Errorf("task %q schedule: %w", task.ID, err)
			}
		}
	}
	return nil
}

func validatePlugins(scope string, plugins []PluginConfig) error {
	for _, plugin := range plugins {
		if err := ValidatePlugin(plugin); err != nil {
			return fmt.Errorf("%s: %w", scope, err)
		}
	}
	return nil
}

func ValidatePlugin(plugin PluginConfig) error {
	if plugin.Type != "filename_skip" {
		return nil
	}
	if plugin.Pattern == "" {
		return fmt.Errorf("filename_skip pattern must not be empty")
	}
	if plugin.MatchMode != "" && plugin.MatchMode != "literal" && plugin.MatchMode != "regex" {
		return fmt.Errorf("filename_skip match_mode must be literal or regex")
	}
	if plugin.FilterMode != "" && plugin.FilterMode != "exclude" && plugin.FilterMode != "include" {
		return fmt.Errorf("filename_skip filter_mode must be exclude or include")
	}
	if plugin.MatchMode == "regex" {
		pattern := plugin.Pattern
		if !plugin.CaseSensitive {
			pattern = "(?i)" + pattern
		}
		if _, err := regexp.Compile(pattern); err != nil {
			return fmt.Errorf("filename_skip pattern: %w", err)
		}
	}
	return nil
}

func applyEnv(cfg *Config) {
	if value := os.Getenv("PORT"); value != "" {
		cfg.Listen = ":" + strings.TrimPrefix(value, ":")
	}
	if value := os.Getenv("ADMIN_USERNAME"); value != "" {
		cfg.Admin.Username = value
	}
	if value := os.Getenv("ADMIN_PASSWORD"); value != "" {
		cfg.Admin.Password = value
	}
	if value := os.Getenv("WEBHOOK_TOKEN"); value != "" {
		cfg.WebhookToken = value
	}
}
