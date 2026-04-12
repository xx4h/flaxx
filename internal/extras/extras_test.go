package extras

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverEmpty(t *testing.T) {
	dir := t.TempDir()
	extras, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(extras) != 0 {
		t.Errorf("expected 0 extras, got %d", len(extras))
	}
}

func TestDiscoverNonexistent(t *testing.T) {
	extras, err := Discover("/nonexistent/path")
	if err != nil {
		t.Fatal(err)
	}
	if extras != nil {
		t.Error("expected nil for nonexistent dir")
	}
}

func setupTestExtra(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	vsoDir := filepath.Join(dir, "vso")
	if err := os.MkdirAll(vsoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	meta := `name: vso
description: Vault Secret Operator
target: namespaces
variables:
  vault_mount:
    description: Vault auth mount
    default: "{{.Cluster}}-auth-mount"
  vault_role:
    description: Vault role
    default: "{{.Cluster}}-{{.App}}-role"
`
	if err := os.WriteFile(filepath.Join(vsoDir, "_meta.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}

	sa := `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{.App}}-vault-sa
`
	if err := os.WriteFile(filepath.Join(vsoDir, "serviceaccount.yaml"), []byte(sa), 0o644); err != nil {
		t.Fatal(err)
	}

	vsoConfig := `---
apiVersion: secrets.hashicorp.com/v1beta1
kind: VaultAuth
metadata:
  name: vso-auth
spec:
  method: kubernetes
  mount: {{.vault_mount}}
  kubernetes:
    role: {{.vault_role}}
    serviceAccount: {{.App}}-vault-sa
    audiences:
      - vault
`
	if err := os.WriteFile(filepath.Join(vsoDir, "vso-config.yaml"), []byte(vsoConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	return dir
}

func TestDiscoverFindsExtra(t *testing.T) {
	dir := setupTestExtra(t)

	found, err := Discover(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(found) != 1 {
		t.Fatalf("expected 1 extra, got %d", len(found))
	}
	if found[0].Meta.Name != "vso" {
		t.Errorf("expected name vso, got %s", found[0].Meta.Name)
	}
	if found[0].Meta.Target != "namespaces" {
		t.Errorf("expected target namespaces, got %s", found[0].Meta.Target)
	}
	if len(found[0].Files) != 2 {
		t.Errorf("expected 2 files, got %d", len(found[0].Files))
	}
}

func TestFindByName(t *testing.T) {
	dir := setupTestExtra(t)
	found, _ := Discover(dir)

	extra := FindByName(found, "vso")
	if extra == nil {
		t.Fatal("expected to find vso extra")
	}

	missing := FindByName(found, "nonexistent")
	if missing != nil {
		t.Error("expected nil for missing extra")
	}
}

func TestResolveVariables(t *testing.T) {
	meta := Meta{
		Variables: map[string]Variable{
			"vault_mount": {Default: "{{.Cluster}}-auth-mount"},
			"vault_role":  {Default: "{{.Cluster}}-{{.App}}-role"},
		},
	}
	data := ExtraData{
		App:       "myapp",
		Cluster:   "k8s",
		Namespace: "myapp",
		Vars:      map[string]string{},
	}

	vars, err := ResolveVariables(meta, data)
	if err != nil {
		t.Fatal(err)
	}

	if vars["vault_mount"] != "k8s-auth-mount" {
		t.Errorf("expected k8s-auth-mount, got %s", vars["vault_mount"])
	}
	if vars["vault_role"] != "k8s-myapp-role" {
		t.Errorf("expected k8s-myapp-role, got %s", vars["vault_role"])
	}
}

func TestResolveVariablesWithOverride(t *testing.T) {
	meta := Meta{
		Variables: map[string]Variable{
			"vault_mount": {Default: "{{.Cluster}}-auth-mount"},
		},
	}
	data := ExtraData{
		App:     "myapp",
		Cluster: "k8s",
		Vars:    map[string]string{"vault_mount": "custom-mount"},
	}

	vars, err := ResolveVariables(meta, data)
	if err != nil {
		t.Fatal(err)
	}

	if vars["vault_mount"] != "custom-mount" {
		t.Errorf("expected custom-mount, got %s", vars["vault_mount"])
	}
}

func TestRenderFile(t *testing.T) {
	dir := setupTestExtra(t)
	filePath := filepath.Join(dir, "vso", "serviceaccount.yaml")

	data := ExtraData{
		App:       "myapp",
		Cluster:   "k8s",
		Namespace: "myapp",
		Vars:      map[string]string{},
	}

	result, err := RenderFile(filePath, data)
	if err != nil {
		t.Fatal(err)
	}

	expected := "name: myapp-vault-sa"
	if !contains(result, expected) {
		t.Errorf("expected %q in output, got:\n%s", expected, result)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
