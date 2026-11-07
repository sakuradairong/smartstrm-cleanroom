package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRegressionInvalidPluginsRejected(t *testing.T) {
	for _, p := range []PluginConfig{{Type: "unknown-plugin"}, {Type: "replace_regex", Pattern: "["}, {Type: "request_delay", DelayMS: 60001}, {Type: "custom_strm_filename", Template: "../unsafe.strm"}} {
		t.Run(p.Type, func(t *testing.T) {
			cfg := Config{PublicURL: "http://example.test", WebhookToken: "secret", Plugins: []PluginConfig{p}}
			if e := cfg.Validate(); e == nil {
				t.Fatal("invalid plugin accepted by Validate")
			}
		})
	}
}

func TestConfigAndBackupRejectOversizedValidPrefix(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.json")
	body := `{"version":1,"public_url":"http://example.test","webhook_token":"secret"}` + strings.Repeat(" ", maximumConfigBytes)
	if err := os.WriteFile(filename, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadDisk(filename); err == nil {
		t.Fatal("accepted oversized config")
	}
	if err := os.WriteFile(filename+".bak", []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreBackup(filename); err == nil {
		t.Fatal("accepted oversized backup")
	}
}
func TestRegressionConfigPrivatePermissions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	cfg := Config{PublicURL: "http://example.test", WebhookToken: "secret", Admin: AdminConfig{Username: "admin", Password: "sensitive"}}
	if e := Save(p, cfg); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(p)
	if e != nil {
		t.Fatal(e)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions=%#o, documented contract=0600", info.Mode().Perm())
	}
}
func TestRegressionConfigTrailingDocument(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	data, e := json.Marshal(Config{Version: 1, PublicURL: "http://example.test", WebhookToken: "secret"})
	if e != nil {
		t.Fatal(e)
	}
	data = append(data, []byte("\n{\"unexpected\":true}")...)
	if e = os.WriteFile(p, data, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e = LoadDisk(p); e == nil {
		t.Fatal("LoadDisk accepted a second JSON document")
	}
}

func TestPrivatePermissionsAcrossBackupRestoreAndExternalEdit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	cfg := Config{PublicURL: "http://example.test", WebhookToken: "first-token"}
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	// A previous interrupted backup may have left a broadly readable temp file.
	if err := os.WriteFile(p+".bak.tmp", []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg.WebhookToken = "second-token"
	if err := Save(p, cfg); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{p, p + ".bak"} {
		info, err := os.Stat(name)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("%s: info=%v err=%v", name, info, err)
		}
	}
	restored, err := Restore(p)
	if err != nil || restored.WebhookToken != "first-token" {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	if err := os.Chmod(p, 0644); err != nil {
		t.Fatal(err)
	}
	if err := RewritePermissions(p); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("rewrite: info=%v err=%v", info, err)
	}
}
