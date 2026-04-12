package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xx4h/flaxx/internal/config"
)

func setupExistingApp(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()

	// Create existing app directories
	clusterDir := filepath.Join(dir, "clusters", "k8s", "myapp")
	nsDir := filepath.Join(dir, "clusters", "k8s-namespaces", "myapp")
	os.MkdirAll(clusterDir, 0o755)
	os.MkdirAll(nsDir, 0o755)

	// Create existing kustomization.yaml
	ks := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- namespace.yaml
`
	os.WriteFile(filepath.Join(nsDir, "kustomization.yaml"), []byte(ks), 0o644)
	os.WriteFile(filepath.Join(nsDir, "namespace.yaml"), []byte(""), 0o644)

	return dir, dir
}

func setupTestExtra(t *testing.T, repoRoot string) {
	t.Helper()
	extraDir := filepath.Join(repoRoot, ".flaxx", "templates", "vso")
	os.MkdirAll(extraDir, 0o755)

	meta := `name: vso
description: Vault Secret Operator
target: namespaces
variables:
  vault_mount:
    description: Vault auth mount
    default: "{{.Cluster}}-auth-mount"
`
	os.WriteFile(filepath.Join(extraDir, "_meta.yaml"), []byte(meta), 0o644)

	sa := `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{.App}}-vault-sa
`
	os.WriteFile(filepath.Join(extraDir, "serviceaccount.yaml"), []byte(sa), 0o644)
}

func TestRunAddExtra(t *testing.T) {
	dir, repoRoot := setupExistingApp(t)
	setupTestExtra(t, repoRoot)

	cfg := config.DefaultConfig()
	opts := AddOptions{
		App:     "myapp",
		Cluster: "k8s",
		Extras:  []string{"vso"},
		Sets:    map[string]string{},
	}

	result, err := RunAdd(cfg, opts, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 2 {
		t.Errorf("expected 2 files, got %d: %v", len(result.Files), result.Files)
	}

	// Check serviceaccount was created
	saPath := filepath.Join(dir, "clusters", "k8s-namespaces", "myapp", "serviceaccount.yaml")
	content, err := os.ReadFile(saPath)
	if err != nil {
		t.Fatalf("serviceaccount.yaml not created: %v", err)
	}
	if !strings.Contains(string(content), "myapp-vault-sa") {
		t.Error("serviceaccount missing app name")
	}

	// Check kustomization.yaml was updated
	ksPath := filepath.Join(dir, "clusters", "k8s-namespaces", "myapp", "kustomization.yaml")
	ksContent, err := os.ReadFile(ksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(ksContent), "- serviceaccount.yaml") {
		t.Error("kustomization.yaml not updated with serviceaccount.yaml")
	}
}

func TestRunAddFailsIfAppNotExists(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	opts := AddOptions{
		App:     "nonexistent",
		Cluster: "k8s",
		Extras:  []string{"vso"},
		Sets:    map[string]string{},
	}

	_, err := RunAdd(cfg, opts, dir)
	if err == nil {
		t.Fatal("expected error for nonexistent app")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'does not exist' error, got: %v", err)
	}
}

func TestRunAddNoExtras(t *testing.T) {
	dir, _ := setupExistingApp(t)

	cfg := config.DefaultConfig()
	opts := AddOptions{
		App:     "myapp",
		Cluster: "k8s",
		Extras:  []string{},
		Sets:    map[string]string{},
	}

	_, err := RunAdd(cfg, opts, dir)
	if err == nil {
		t.Fatal("expected error for no extras")
	}
}
