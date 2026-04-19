package generator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xx4h/flaxx/internal/config"
)

func TestRunCoreType(t *testing.T) {
	dir := t.TempDir()

	// Create cluster dirs to satisfy existence check
	clusterDir := filepath.Join(dir, "clusters", "k8s")
	nsDir := filepath.Join(dir, "clusters", "k8s-namespaces")
	os.MkdirAll(clusterDir, 0o755)
	os.MkdirAll(nsDir, 0o755)

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterSubdirs = true
	opts := Options{
		App:     "myapp",
		Cluster: "k8s",
		Type:    TypeCore,
	}

	result, err := Run(cfg, opts, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(result.Files), result.Files)
	}

	// Check namespace.yaml was created
	nsFile := filepath.Join(dir, "clusters", "k8s-namespaces", "myapp", "namespace.yaml")
	content, err := os.ReadFile(nsFile)
	if err != nil {
		t.Fatalf("namespace.yaml not created: %v", err)
	}
	if !strings.Contains(string(content), "name: myapp") {
		t.Error("namespace.yaml missing app name")
	}

	// Check kustomization.yaml was created
	ksFile := filepath.Join(dir, "clusters", "k8s-namespaces", "myapp", "kustomization.yaml")
	if _, statErr := os.Stat(ksFile); statErr != nil {
		t.Fatalf("kustomization.yaml not created: %v", statErr)
	}

	// Check flux kustomization was created
	fluxKs := filepath.Join(dir, "clusters", "k8s", "myapp", "myapp-kustomization.yaml")
	content, err = os.ReadFile(fluxKs)
	if err != nil {
		t.Fatalf("flux kustomization not created: %v", err)
	}
	if !strings.Contains(string(content), "name: flux-system") {
		t.Error("flux kustomization missing flux-system ref")
	}
}

func TestRunCoreHelmType(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "clusters", "k8s"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterSubdirs = true
	opts := Options{
		App:     "myapp",
		Cluster: "k8s",
		Type:    TypeCoreHelm,
		HelmURL: "https://charts.example.com",
	}

	result, err := Run(cfg, opts, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 4 {
		t.Errorf("expected 4 files, got %d: %v", len(result.Files), result.Files)
	}

	helmFile := filepath.Join(dir, "clusters", "k8s", "myapp", "myapp-helm.yml")
	content, err := os.ReadFile(helmFile)
	if err != nil {
		t.Fatalf("helm file not created: %v", err)
	}
	if !strings.Contains(string(content), "kind: HelmRepository") {
		t.Error("helm file missing HelmRepository")
	}
	if !strings.Contains(string(content), "kind: HelmRelease") {
		t.Error("helm file missing HelmRelease")
	}
}

func TestRunHelmWithValues(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "clusters", "k8s"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterSubdirs = true
	opts := Options{
		App:         "myapp",
		Cluster:     "k8s",
		Type:        TypeExtHelm,
		HelmURL:     "https://charts.example.com",
		HelmChart:   "myapp",
		HelmVersion: "1.2.3",
		HelmValues: map[string]interface{}{
			"replicaCount": 3,
			"image": map[string]interface{}{
				"tag": "v2",
			},
		},
	}

	if _, err := Run(cfg, opts, dir); err != nil {
		t.Fatal(err)
	}

	helmFile := filepath.Join(dir, "clusters", "k8s", "myapp", "myapp-helm.yml")
	content, err := os.ReadFile(helmFile)
	if err != nil {
		t.Fatalf("helm file not created: %v", err)
	}
	got := string(content)

	if strings.Contains(got, "values: {}") {
		t.Errorf("should not emit `values: {}` when HelmValues is set, got:\n%s", got)
	}
	for _, want := range []string{
		"  values:\n",
		"    replicaCount: 3",
		"    image:",
		"      tag: v2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRunHelmOCI(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "clusters", "k8s"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterSubdirs = true
	opts := Options{
		App:       "myapp",
		Cluster:   "k8s",
		Type:      TypeExtOCI,
		HelmURL:   "oci://ghcr.io/example/charts",
		HelmChart: "myapp",
	}

	if _, err := Run(cfg, opts, dir); err != nil {
		t.Fatal(err)
	}

	helmFile := filepath.Join(dir, "clusters", "k8s", "myapp", "myapp-helm.yml")
	content, err := os.ReadFile(helmFile)
	if err != nil {
		t.Fatalf("helm file not created: %v", err)
	}
	if !strings.Contains(string(content), "type: oci") {
		t.Errorf("OCI HelmRepository should have `type: oci`, got:\n%s", content)
	}
}

func TestRunExtGitType(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "clusters", "k8s"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterSubdirs = true
	opts := Options{
		App:       "ddns",
		Cluster:   "k8s",
		Type:      TypeExtGit,
		GitURL:    "https://git.example.com/org/ddns-server.git",
		GitBranch: "main",
		GitPath:   "./deploy/production",
		GitSecret: "git-repo-secret",
	}

	result, err := Run(cfg, opts, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 4 {
		t.Errorf("expected 4 files, got %d: %v", len(result.Files), result.Files)
	}

	// Check dual kustomization
	fluxKs := filepath.Join(dir, "clusters", "k8s", "ddns", "ddns-kustomization.yaml")
	content, err := os.ReadFile(fluxKs)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "name: ddns-app") {
		t.Error("missing dual kustomization entry")
	}
	if !strings.Contains(string(content), "name: ddns-server") {
		t.Error("missing git repo reference")
	}

	// Check git repository file
	gitFile := filepath.Join(dir, "clusters", "k8s", "ddns", "ddns-git.yml")
	content, err = os.ReadFile(gitFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "name: ddns-server") {
		t.Error("git file missing repo name")
	}
}

func TestRunDryRun(t *testing.T) {
	dir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterSubdirs = true
	opts := Options{
		App:     "myapp",
		Cluster: "k8s",
		Type:    TypeCore,
		DryRun:  true,
	}

	result, err := Run(cfg, opts, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 3 {
		t.Errorf("expected 3 files listed, got %d", len(result.Files))
	}

	// Verify no files actually created
	appDir := filepath.Join(dir, "clusters", "k8s", "myapp")
	if _, err := os.Stat(appDir); err == nil {
		t.Error("dry run should not create directories")
	}
}

func TestRunFailsIfDirExists(t *testing.T) {
	dir := t.TempDir()

	// Create the app dir to trigger conflict (subdirs mode)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s", "myapp"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterSubdirs = true
	opts := Options{
		App:     "myapp",
		Cluster: "k8s",
		Type:    TypeCore,
	}

	_, err := Run(cfg, opts, dir)
	if err == nil {
		t.Fatal("expected error when directory exists")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected 'already exists' error, got: %v", err)
	}
}

func TestRunCoreTypeFlat(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "clusters", "k8s"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	// ClusterSubdirs defaults to false (flat layout)
	opts := Options{
		App:     "myapp",
		Cluster: "k8s",
		Type:    TypeCore,
	}

	result, err := Run(cfg, opts, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 3 {
		t.Errorf("expected 3 files, got %d: %v", len(result.Files), result.Files)
	}

	// Flux kustomization should be flat in cluster dir (not in myapp/ subdirectory)
	fluxKs := filepath.Join(dir, "clusters", "k8s", "myapp-kustomization.yaml")
	content, err := os.ReadFile(fluxKs)
	if err != nil {
		t.Fatalf("flux kustomization not created at flat path: %v", err)
	}
	if !strings.Contains(string(content), "name: flux-system") {
		t.Error("flux kustomization missing flux-system ref")
	}

	// Should NOT exist in a subdirectory
	wrongPath := filepath.Join(dir, "clusters", "k8s", "myapp", "myapp-kustomization.yaml")
	if _, statErr := os.Stat(wrongPath); statErr == nil {
		t.Error("flat layout should not create per-app subdirectory")
	}

	// Parent kustomization.yaml should be auto-updated
	parentKs := filepath.Join(dir, "clusters", "k8s", "kustomization.yaml")
	parentContent, err := os.ReadFile(parentKs)
	if err != nil {
		t.Fatalf("parent kustomization.yaml not created: %v", err)
	}
	if !strings.Contains(string(parentContent), "- myapp-kustomization.yaml") {
		t.Errorf("parent kustomization.yaml missing app entry, got:\n%s", string(parentContent))
	}
}

func TestRunCoreHelmTypeFlat(t *testing.T) {
	dir := t.TempDir()

	os.MkdirAll(filepath.Join(dir, "clusters", "k8s"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "k8s-namespaces"), 0o755)

	cfg := config.DefaultConfig()
	opts := Options{
		App:     "myapp",
		Cluster: "k8s",
		Type:    TypeCoreHelm,
		HelmURL: "https://charts.example.com",
	}

	result, err := Run(cfg, opts, dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(result.Files) != 4 {
		t.Errorf("expected 4 files, got %d: %v", len(result.Files), result.Files)
	}

	// Helm file should be flat in cluster dir
	helmFile := filepath.Join(dir, "clusters", "k8s", "myapp-helm.yml")
	content, err := os.ReadFile(helmFile)
	if err != nil {
		t.Fatalf("helm file not created at flat path: %v", err)
	}
	if !strings.Contains(string(content), "kind: HelmRelease") {
		t.Error("helm file missing HelmRelease")
	}

	// Parent kustomization should list both files
	parentKs := filepath.Join(dir, "clusters", "k8s", "kustomization.yaml")
	parentContent, err := os.ReadFile(parentKs)
	if err != nil {
		t.Fatalf("parent kustomization.yaml not created: %v", err)
	}
	if !strings.Contains(string(parentContent), "- myapp-kustomization.yaml") {
		t.Error("parent kustomization.yaml missing kustomization entry")
	}
	if !strings.Contains(string(parentContent), "- myapp-helm.yml") {
		t.Error("parent kustomization.yaml missing helm entry")
	}
}

func TestDeriveGitName(t *testing.T) {
	tests := []struct {
		url      string
		expected string
	}{
		{"https://git.example.com/org/myapp-server.git", "myapp-server"},
		{"https://git.example.com/org/myapp.git", "myapp"},
		{"https://git.example.com/org/myapp", "myapp"},
		{"", ""},
	}

	for _, tt := range tests {
		got := deriveGitName(tt.url)
		if got != tt.expected {
			t.Errorf("deriveGitName(%q) = %q, want %q", tt.url, got, tt.expected)
		}
	}
}
