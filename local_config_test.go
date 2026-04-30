package sailor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

func writeSailorConfigToHome(t *testing.T, home string, cfg localSailorConfig) {
	t.Helper()
	dir := filepath.Join(home, ".sailor")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	data, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(dir, "config"), data, 0644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func sampleConfig() localSailorConfig {
	var cfg localSailorConfig
	cfg.Manifest.Envs = []localSailorEnv{
		{Name: "local", Host: "http://localhost:7766"},
		{Name: "sit", Host: "http://sit.example.com"},
	}
	cfg.Env = "sit"
	cfg.Token = "test-token"
	cfg.User = "dev@example.com"
	return cfg
}

func baseConn() *opts.ConnectionOption {
	return &opts.ConnectionOption{Namespace: "testns", App: "testapp"}
}

func overrideHome(t *testing.T, home string) {
	t.Helper()
	orig := os.Getenv("HOME")
	os.Setenv("HOME", home)
	t.Cleanup(func() { os.Setenv("HOME", orig) })
}

func TestBuildConnectionFromLocalConfig_Valid(t *testing.T) {
	home := t.TempDir()
	overrideHome(t, home)
	writeSailorConfigToHome(t, home, sampleConfig())

	result, err := buildConnectionFromLocalConfig(baseConn())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.Addr != "http://sit.example.com" {
		t.Errorf("expected Addr 'http://sit.example.com', got '%s'", result.Addr)
	}
	if result.Token != "test-token" {
		t.Errorf("expected Token 'test-token', got '%s'", result.Token)
	}
	if result.Env != "sit" {
		t.Errorf("expected Env 'sit', got '%s'", result.Env)
	}
	if result.Namespace != "testns" {
		t.Errorf("expected Namespace 'testns', got '%s'", result.Namespace)
	}
	if result.App != "testapp" {
		t.Errorf("expected App 'testapp', got '%s'", result.App)
	}
}

func TestBuildConnectionFromLocalConfig_NotFound(t *testing.T) {
	home := t.TempDir() // empty — no .sailor/config
	overrideHome(t, home)

	_, err := buildConnectionFromLocalConfig(baseConn())
	if err != ErrLocalConfigNotFound {
		t.Errorf("expected ErrLocalConfigNotFound, got %v", err)
	}
}

func TestBuildConnectionFromLocalConfig_EnvNotFound(t *testing.T) {
	home := t.TempDir()
	overrideHome(t, home)

	var cfg localSailorConfig
	cfg.Manifest.Envs = []localSailorEnv{
		{Name: "local", Host: "http://localhost:7766"},
	}
	cfg.Env = "production" // not present in manifest
	cfg.Token = "tok"
	writeSailorConfigToHome(t, home, cfg)

	_, err := buildConnectionFromLocalConfig(baseConn())
	if err != ErrLocalConfigEnvNotFound {
		t.Errorf("expected ErrLocalConfigEnvNotFound, got %v", err)
	}
}

func TestBuildConnectionFromLocalConfig_InvalidJSON(t *testing.T) {
	home := t.TempDir()
	overrideHome(t, home)

	dir := filepath.Join(home, ".sailor")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "config"), []byte("not-valid-json{{{"), 0644)

	_, err := buildConnectionFromLocalConfig(baseConn())
	if err != ErrLocalConfigInvalid {
		t.Errorf("expected ErrLocalConfigInvalid, got %v", err)
	}
}
