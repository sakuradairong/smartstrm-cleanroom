package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidate(t *testing.T) {
	valid := Config{
		PublicURL: "http://localhost:8024", WebhookToken: "secret",
		Storages: []StorageConfig{{ID: "media", Type: "local", Root: "/media"}},
		Tasks:    []TaskConfig{{ID: "movies", StorageID: "media", Destination: "/strm"}},
	}
	tests := []struct {
		name   string
		mutate func(*Config)
		match  string
	}{
		{name: "valid"},
		{name: "empty token", mutate: func(c *Config) { c.WebhookToken = "" }, match: "webhook_token"},
		{name: "relative public URL", mutate: func(c *Config) { c.PublicURL = "/stream" }, match: "public_url"},
		{name: "unsafe storage id", mutate: func(c *Config) { c.Storages[0].ID = "../media" }, match: "storage id"},
		{name: "empty local root", mutate: func(c *Config) { c.Storages[0].Root = "" }, match: "root"},
		{name: "relative local root", mutate: func(c *Config) { c.Storages[0].Root = "media" }, match: "absolute"},
		{name: "filesystem local root", mutate: func(c *Config) { c.Storages[0].Root = "/" }, match: "filesystem root"},
		{name: "unsupported storage type", mutate: func(c *Config) { c.Storages[0].Type = "mystery" }, match: "unsupported type"},
		{name: "storage endpoint scheme", mutate: func(c *Config) {
			c.Storages[0].Type, c.Storages[0].Endpoint = "openlist", "ftp://example.test"
		}, match: "HTTP(S)"},
		{name: "storage endpoint credentials", mutate: func(c *Config) {
			c.Storages[0].Type, c.Storages[0].Endpoint = "webdav", "https://user:pass@example.test/dav"
		}, match: "credentials"},
		{name: "storage endpoint query", mutate: func(c *Config) {
			c.Storages[0].Type, c.Storages[0].Endpoint = "openlist", "https://example.test/api?token=x"
		}, match: "query"},
		{name: "storage endpoint fragment", mutate: func(c *Config) {
			c.Storages[0].Type, c.Storages[0].Endpoint = "webdav", "https://example.test/dav#fragment"
		}, match: "fragment"},
		{name: "unknown storage", mutate: func(c *Config) { c.Tasks[0].StorageID = "missing" }, match: "unknown storage"},
		{name: "empty filename skip", mutate: func(c *Config) { c.Tasks[0].Plugins = []PluginConfig{{Type: "filename_skip"}} }, match: "pattern"},
		{name: "invalid match mode", mutate: func(c *Config) {
			c.Tasks[0].Plugins = []PluginConfig{{Type: "filename_skip", Pattern: "x", MatchMode: "glob"}}
		}, match: "match_mode"},
		{name: "invalid filter mode", mutate: func(c *Config) { c.Plugins = []PluginConfig{{Type: "filename_skip", Pattern: "x", FilterMode: "keep"}} }, match: "filter_mode"},
		{name: "invalid skip regex", mutate: func(c *Config) {
			c.Tasks[0].Plugins = []PluginConfig{{Type: "filename_skip", Pattern: "[", MatchMode: "regex"}}
		}, match: "pattern"},
		{name: "invalid schedule", mutate: func(c *Config) { c.Tasks[0].Schedule = "60 * * * *" }, match: "schedule"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := valid
			cfg.Storages = append([]StorageConfig(nil), valid.Storages...)
			cfg.Tasks = append([]TaskConfig(nil), valid.Tasks...)
			if test.mutate != nil {
				test.mutate(&cfg)
			}
			err := cfg.Validate()
			if test.match == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.match)
		})
	}
}

func TestValidateAllowsANiEndpointQuery(t *testing.T) {
	cfg := Config{
		PublicURL: "http://localhost:8024", WebhookToken: "secret",
		Storages: []StorageConfig{{ID: "ani", Type: "ani_open", Endpoint: "https://example.test/feed.xml?mirror=1"}},
	}
	require.NoError(t, cfg.Validate())
}

func TestLoadMigratesLegacyAndCreatesBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := `{"public_url":"http://localhost:8024","webhook_token":"secret","storages":[],"tasks":[]}`
	require.NoError(t, os.WriteFile(path, []byte(legacy), 0o644))
	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, CurrentVersion, cfg.Version)
	assert.Equal(t, path, cfg.Path)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), `"version": 1`)
	backup, err := os.ReadFile(path + ".bak")
	require.NoError(t, err)
	assert.JSONEq(t, legacy, string(backup))
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSaveRestoreAndPreserveSecrets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	first := Config{Version: CurrentVersion, PublicURL: "http://localhost:8024", WebhookToken: "first", Admin: AdminConfig{Username: "admin", Password: "password"}, TMDB: TMDBConfig{APIKey: "key"}, Storages: []StorageConfig{{ID: "media", Type: "local", Root: "/media", Password: "storage-pass", Token: "storage-token"}}}
	require.NoError(t, Save(path, first))
	second := first
	second.WebhookToken = "second"
	require.NoError(t, Save(path, second))
	restored, err := Restore(path)
	require.NoError(t, err)
	assert.Equal(t, "first", restored.WebhookToken)
	assert.Equal(t, CurrentVersion, restored.Version)
	loaded, err := LoadDisk(path)
	require.NoError(t, err)
	assert.Equal(t, "first", loaded.WebhookToken)
	redacted := first.Redacted()
	assert.Equal(t, RedactedSecret, redacted.WebhookToken)
	assert.Equal(t, RedactedSecret, redacted.Storages[0].Token)
	PreserveSecrets(&redacted, first)
	assert.Equal(t, "first", redacted.WebhookToken)
	assert.Equal(t, "storage-token", redacted.Storages[0].Token)
}

func TestLoadRejectsUnknownAndFutureVersion(t *testing.T) {
	for name, content := range map[string]string{
		"unknown": `{"version":1,"public_url":"http://localhost","webhook_token":"x","surprise":true}`,
		"future":  `{"version":99,"public_url":"http://localhost","webhook_token":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
			_, err := LoadDisk(path)
			require.Error(t, err)
		})
	}
}

func TestRedactedConfigIsJSONSafe(t *testing.T) {
	cfg := Config{WebhookToken: "secret-value", Admin: AdminConfig{Password: "admin-password"}, TMDB: TMDBConfig{APIKey: "tmdb-key"}}
	data, err := json.Marshal(cfg.Redacted())
	require.NoError(t, err)
	text := string(data)
	assert.NotContains(t, text, "secret-value")
	assert.NotContains(t, text, "admin-password")
	assert.NotContains(t, text, "tmdb-key")
}

func TestMediaProxyValidation(t *testing.T) {
	base := Config{PublicURL: "http://localhost", WebhookToken: "secret"}
	base.MediaProxy = MediaProxyConfig{Enabled: true, Listen: ":8097", Upstream: "http://emby:8096", ServerType: "emby", RewritePlaybackInfo: true}
	require.NoError(t, base.Validate())
	base.MediaProxy.Listen = "bad address"
	require.ErrorContains(t, base.Validate(), "media_proxy.listen")
	base.MediaProxy.Listen = ":8097"
	base.MediaProxy.Upstream = "http://user:pass@emby:8096"
	require.ErrorContains(t, base.Validate(), "media_proxy.upstream")
	base.MediaProxy.Upstream = "http://emby:8096"
	base.MediaProxy.ServerType = "plex"
	require.ErrorContains(t, base.Validate(), "media_proxy.server_type")
	base.MediaProxy.ServerType = ""
	require.ErrorContains(t, base.Validate(), "media_proxy.server_type")
	base.MediaProxy = MediaProxyConfig{Enabled: false, Listen: "bad address", Upstream: "file:///tmp"}
	require.NoError(t, base.Validate(), "disabled proxy settings are inert")
}
