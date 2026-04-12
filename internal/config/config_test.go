package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Defaults.Interval != "2m" {
		t.Errorf("expected interval 2m, got %s", cfg.Defaults.Interval)
	}
	if cfg.Defaults.Timeout != "1m" {
		t.Errorf("expected timeout 1m, got %s", cfg.Defaults.Timeout)
	}
	if cfg.Defaults.Prune != false {
		t.Error("expected prune false")
	}
	if cfg.Paths.ClusterDir != "clusters/{{.Cluster}}" {
		t.Errorf("unexpected cluster dir: %s", cfg.Paths.ClusterDir)
	}
	if cfg.TemplatesDir != ".flaxx/templates" {
		t.Errorf("unexpected templates dir: %s", cfg.TemplatesDir)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/.flaxx.yaml")
	if err != nil {
		t.Fatalf("expected no error for missing file, got: %v", err)
	}
	if cfg.Defaults.Interval != "2m" {
		t.Error("expected defaults when file missing")
	}
}

func TestLoadValidFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".flaxx.yaml")

	content := `
defaults:
  interval: 5m
  timeout: 3m
  prune: true
paths:
  cluster_dir: "my-clusters/{{.Cluster}}"
`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Defaults.Interval != "5m" {
		t.Errorf("expected interval 5m, got %s", cfg.Defaults.Interval)
	}
	if cfg.Defaults.Timeout != "3m" {
		t.Errorf("expected timeout 3m, got %s", cfg.Defaults.Timeout)
	}
	if cfg.Defaults.Prune != true {
		t.Error("expected prune true")
	}
	if cfg.Paths.ClusterDir != "my-clusters/{{.Cluster}}" {
		t.Errorf("unexpected cluster dir: %s", cfg.Paths.ClusterDir)
	}
}

func TestFindConfigFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".flaxx.yaml")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	subDir := filepath.Join(dir, "a", "b", "c")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}

	found := FindConfigFile(subDir)
	if found != cfgPath {
		t.Errorf("expected %s, got %s", cfgPath, found)
	}
}

func TestFindConfigFileNotFound(t *testing.T) {
	dir := t.TempDir()
	found := FindConfigFile(dir)
	if found != "" {
		t.Errorf("expected empty, got %s", found)
	}
}
