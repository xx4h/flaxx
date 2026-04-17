package extractor

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xx4h/flaxx/internal/config"
)

// setupApp creates a minimal flaxx-shaped app directory tree in root:
//
//	clusters/lab/myapp-kustomization.yaml   (flux Kustomization)
//	clusters/lab/myapp-helm.yml             (HelmRepository + HelmRelease)
//	clusters/lab-namespaces/myapp/namespace.yaml
//	clusters/lab-namespaces/myapp/kustomization.yaml
//	clusters/lab-namespaces/myapp/ingress.yaml
func setupApp(t *testing.T, root, cluster, app string) {
	t.Helper()
	clusterDir := filepath.Join(root, "clusters", cluster)
	nsDir := filepath.Join(root, "clusters", cluster+"-namespaces", app)
	if err := os.MkdirAll(clusterDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	helm := fmt.Sprintf(`---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: %s
  namespace: flux-system
spec:
  url: https://charts.example.com/
  interval: 2m
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: %s
  namespace: %s
spec:
  chart:
    spec:
      chart: podinfo
      version: '1.2.3'
      sourceRef:
        kind: HelmRepository
        name: %s
        namespace: flux-system
`, app, app, app, app)
	if err := os.WriteFile(filepath.Join(clusterDir, app+"-helm.yml"), []byte(helm), 0o644); err != nil {
		t.Fatal(err)
	}

	ks := fmt.Sprintf(`---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: %s
  namespace: flux-system
spec:
  interval: 2m
  path: ./clusters/%s-namespaces/%s
  prune: false
  sourceRef:
    kind: GitRepository
    name: flux-system
  targetNamespace: %s
`, app, cluster, app, app)
	if err := os.WriteFile(filepath.Join(clusterDir, app+"-kustomization.yaml"), []byte(ks), 0o644); err != nil {
		t.Fatal(err)
	}

	ns := fmt.Sprintf(`---
apiVersion: v1
kind: Namespace
metadata:
  name: %s
`, app)
	if err := os.WriteFile(filepath.Join(nsDir, "namespace.yaml"), []byte(ns), 0o644); err != nil {
		t.Fatal(err)
	}

	nsKs := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- namespace.yaml
- ingress.yaml
`
	if err := os.WriteFile(filepath.Join(nsDir, "kustomization.yaml"), []byte(nsKs), 0o644); err != nil {
		t.Fatal(err)
	}

	ingress := fmt.Sprintf(`---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %s
spec:
  rules:
    - host: %s.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: %s
                port:
                  number: 80
`, app, app, app)
	if err := os.WriteFile(filepath.Join(nsDir, "ingress.yaml"), []byte(ingress), 0o644); err != nil {
		t.Fatal(err)
	}
}

func defaultCfg() config.Config {
	return config.DefaultConfig()
}

func TestExtractBasic(t *testing.T) {
	root := t.TempDir()
	setupApp(t, root, "lab", "myapp")

	opts := ExtractOptions{
		App:          "myapp",
		Cluster:      "lab",
		TemplateName: "my-template",
		Description:  "Test template",
		Cfg:          defaultCfg(),
		RepoRoot:     root,
	}

	result, err := Extract(opts)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	meta, err := os.ReadFile(filepath.Join(result.TemplateDir, "_meta.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	metaStr := string(meta)
	if !strings.Contains(metaStr, "name: my-template") {
		t.Errorf("_meta.yaml missing name: %s", metaStr)
	}
	if !strings.Contains(metaStr, "target: namespaces") {
		t.Errorf("_meta.yaml should have target namespaces: %s", metaStr)
	}
	if !strings.Contains(metaStr, "chart_version") {
		t.Errorf("_meta.yaml should contain chart_version: %s", metaStr)
	}
	if !strings.Contains(metaStr, "1.2.3") {
		t.Errorf("_meta.yaml should contain chart_version default 1.2.3: %s", metaStr)
	}
	if !strings.Contains(metaStr, "ingress_host") {
		t.Errorf("_meta.yaml should contain ingress_host: %s", metaStr)
	}
	if !strings.Contains(metaStr, "helm_url") {
		t.Errorf("_meta.yaml should contain helm_url: %s", metaStr)
	}

	// Namespace-only extraction: no cluster/ or namespaces/ subdirs
	ingressContent, err := os.ReadFile(filepath.Join(result.TemplateDir, "ingress.yaml"))
	if err != nil {
		t.Fatalf("ingress.yaml not written: %v", err)
	}
	is := string(ingressContent)
	if !strings.Contains(is, "{{.App}}") {
		t.Errorf("ingress.yaml should contain {{.App}}: %s", is)
	}
	if !strings.Contains(is, "{{.ingress_host}}") {
		t.Errorf("ingress.yaml should contain {{.ingress_host}}: %s", is)
	}
	if strings.Contains(is, "myapp") {
		t.Errorf("ingress.yaml should not contain literal app name: %s", is)
	}
}

func TestExtractIncludeCluster(t *testing.T) {
	root := t.TempDir()
	setupApp(t, root, "lab", "myapp")

	opts := ExtractOptions{
		App:            "myapp",
		Cluster:        "lab",
		TemplateName:   "my-full",
		IncludeCluster: true,
		Cfg:            defaultCfg(),
		RepoRoot:       root,
	}

	result, err := Extract(opts)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if result.Target != "split" {
		t.Errorf("expected target split, got %s", result.Target)
	}

	clusterFile := filepath.Join(result.TemplateDir, "cluster", "myapp-helm.yml")
	if _, err := os.Stat(clusterFile); err != nil {
		t.Errorf("cluster/myapp-helm.yml not written: %v", err)
	}
	nsFile := filepath.Join(result.TemplateDir, "namespaces", "namespace.yaml")
	if _, err := os.Stat(nsFile); err != nil {
		t.Errorf("namespaces/namespace.yaml not written: %v", err)
	}

	// HelmRelease in cluster/ should have its version parameterized.
	helmContent, _ := os.ReadFile(clusterFile)
	hs := string(helmContent)
	if !strings.Contains(hs, "{{.chart_version}}") {
		t.Errorf("helm file should contain {{.chart_version}}: %s", hs)
	}
	if strings.Contains(hs, "1.2.3") {
		t.Errorf("helm file should not contain literal version 1.2.3: %s", hs)
	}
}

func TestExtractDryRunWritesNothing(t *testing.T) {
	root := t.TempDir()
	setupApp(t, root, "lab", "myapp")

	var buf bytes.Buffer
	opts := ExtractOptions{
		App:          "myapp",
		Cluster:      "lab",
		TemplateName: "dry",
		Cfg:          defaultCfg(),
		RepoRoot:     root,
		DryRun:       true,
		Stdout:       &buf,
	}

	result, err := Extract(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(result.TemplateDir); err == nil {
		t.Error("dry-run should not create template dir")
	}
	if !strings.Contains(buf.String(), "_meta.yaml") {
		t.Errorf("dry-run output should mention _meta.yaml, got:\n%s", buf.String())
	}
}

func TestExtractForceOverwrites(t *testing.T) {
	root := t.TempDir()
	setupApp(t, root, "lab", "myapp")

	cfg := defaultCfg()
	tmplPath := filepath.Join(root, cfg.TemplatesDir, "existing")
	if err := os.MkdirAll(tmplPath, 0o755); err != nil {
		t.Fatal(err)
	}
	// Sentinel stale file that --force should remove.
	if err := os.WriteFile(filepath.Join(tmplPath, "stale.yaml"), []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := ExtractOptions{
		App:          "myapp",
		Cluster:      "lab",
		TemplateName: "existing",
		Cfg:          cfg,
		RepoRoot:     root,
	}

	if _, err := Extract(opts); err == nil {
		t.Error("expected error when template exists and --force not set")
	}

	opts.Force = true
	if _, err := Extract(opts); err != nil {
		t.Fatalf("force extract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmplPath, "stale.yaml")); err == nil {
		t.Error("stale file should have been removed by --force")
	}
}

// scriptedPrompter returns pre-programmed Choices for each candidate.
type scriptedPrompter struct {
	answers map[string]Choice
}

func (s *scriptedPrompter) Ask(c Candidate) (Choice, error) {
	if choice, ok := s.answers[c.Name]; ok {
		return choice, nil
	}
	return Choice{Keep: true, Name: c.Name}, nil
}

func TestExtractInteractiveSkipAndRename(t *testing.T) {
	root := t.TempDir()
	setupApp(t, root, "lab", "myapp")

	prompter := &scriptedPrompter{answers: map[string]Choice{
		"ingress_host":  {Keep: false},                      // skip
		"chart_version": {Keep: true, Name: "helm_version"}, // rename
	}}

	opts := ExtractOptions{
		App:          "myapp",
		Cluster:      "lab",
		TemplateName: "interactive",
		Cfg:          defaultCfg(),
		RepoRoot:     root,
		Interactive:  true,
		Prompter:     prompter,
	}

	result, err := Extract(opts)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}

	if _, ok := result.Variables["ingress_host"]; ok {
		t.Error("ingress_host should have been skipped")
	}
	if _, ok := result.Variables["helm_version"]; !ok {
		t.Error("renamed variable helm_version missing")
	}

	ingressContent, _ := os.ReadFile(filepath.Join(result.TemplateDir, "ingress.yaml"))
	ics := string(ingressContent)
	if strings.Contains(ics, "{{.ingress_host}}") {
		t.Error("ingress_host should not appear in output")
	}
	if !strings.Contains(ics, "{{.App}}.example.com") {
		t.Errorf("host should still be parameterized via {{.App}}: %s", ics)
	}
}

func TestSanitizeKey(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"main", "main"},
		{"foo-bar", "foo_bar"},
		{"foo.bar", "foo_bar"},
		{"", "main"},
	}
	for _, tc := range cases {
		got := sanitizeKey(tc.in)
		if got != tc.out {
			t.Errorf("sanitizeKey(%q)=%q want %q", tc.in, got, tc.out)
		}
	}
}

func TestBuildMetaYAMLStable(t *testing.T) {
	vars := map[string]any{}
	_ = vars
	// Two runs should produce identical output.
	got1, err := buildMetaYAML("x", "y", "namespaces", nil)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := buildMetaYAML("x", "y", "namespaces", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got1 != got2 {
		t.Errorf("non-deterministic meta output")
	}
}
