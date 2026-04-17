package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pflag "github.com/spf13/pflag"
)

// resetFlags resets package-level flag variables and cobra's internal "changed"
// state that persist between command executions in tests.
func resetFlags() {
	// Reset cobra's flag changed state on all commands
	for _, cmd := range rootCmd.Commands() {
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			f.Changed = false
		})
	}
	dryRun = false
	deployType = ""
	namespace = ""
	extraNames = nil
	setVars = nil
	gitURL = ""
	gitBranch = ""
	gitPath = ""
	gitSecret = ""
	helmURL = ""
	helmChart = ""
	helmVersion = ""
	updateHelmVersion = ""
	updateHelm = nil
	updateImage = ""
	updateNamespace = ""
	updateDryRun = false
	addExtraNames = nil
	addSetVars = nil
	addNamespace = ""
	addDryRun = false
	checkNamespace = ""
	checkAll = false
	checkHelm = nil
	checkStable = false
	checkInclPre = false
	fromAppIncludeCluster = false
	fromAppInteractive = false
	fromAppForce = false
	fromAppDryRun = false
	fromAppDescription = ""
}

// executeCommand runs the root command with the given args and returns stdout output.
func executeCommand(args ...string) (string, error) {
	resetFlags()
	buf := new(bytes.Buffer)
	rootCmd.SetOut(buf)
	rootCmd.SetErr(buf)
	rootCmd.SetArgs(args)
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestGenerateArgOrderVerifyPaths(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "clusters", "staging"), 0o755)
	os.MkdirAll(filepath.Join(dir, "clusters", "staging-namespaces"), 0o755)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// cluster=staging, app=webapp
	_, err := executeCommand("generate", "staging", "webapp", "-t", "core")
	if err != nil {
		t.Fatalf("generate failed: %v", err)
	}

	// Verify files went to the right cluster directory (flat layout — no app subdirectory)
	ksFile := filepath.Join(dir, "clusters", "staging", "webapp-kustomization.yaml")
	content, err := os.ReadFile(ksFile)
	if err != nil {
		t.Fatalf("kustomization not created at expected flat path: %v", err)
	}
	if !strings.Contains(string(content), "name: webapp") {
		t.Error("kustomization missing app name 'webapp'")
	}

	nsFile := filepath.Join(dir, "clusters", "staging-namespaces", "webapp", "namespace.yaml")
	content, err = os.ReadFile(nsFile)
	if err != nil {
		t.Fatalf("namespace not created at expected path: %v", err)
	}
	if !strings.Contains(string(content), "name: webapp") {
		t.Error("namespace missing app name 'webapp'")
	}

	// Verify wrong paths don't exist (would exist if args were swapped)
	wrongPath := filepath.Join(dir, "clusters", "webapp", "staging-kustomization.yaml")
	if _, err := os.Stat(wrongPath); err == nil {
		t.Error("files created under clusters/webapp/ — args are swapped (app used as cluster)")
	}
}

func TestUpdateArgOrder(t *testing.T) {
	dir := t.TempDir()

	// Create app files with cluster=production, app=myapp (flat layout)
	clusterDir := filepath.Join(dir, "clusters", "production")
	os.MkdirAll(clusterDir, 0o755)

	helmFile := `---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: myapp
spec:
  chart:
    spec:
      chart: myapp
      version: '1.0.0'
      sourceRef:
        kind: HelmRepository
        name: myapp
`
	os.WriteFile(filepath.Join(clusterDir, "myapp-helm.yml"), []byte(helmFile), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Args: <cluster> <app> → update production myapp
	_, err := executeCommand("update", "production", "myapp", "--helm-version", "2.0.0")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Verify the file was updated in the correct location (flat)
	content, err := os.ReadFile(filepath.Join(clusterDir, "myapp-helm.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "2.0.0") {
		t.Error("helm version not updated — args may be swapped")
	}
}

func TestUpdateHelmFlag(t *testing.T) {
	dir := t.TempDir()

	clusterDir := filepath.Join(dir, "clusters", "production")
	os.MkdirAll(clusterDir, 0o755)

	helmFile := `---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: grafana
spec:
  chart:
    spec:
      chart: grafana
      version: '7.0.0'
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: loki
spec:
  chart:
    spec:
      chart: loki
      version: '5.0.0'
`
	os.WriteFile(filepath.Join(clusterDir, "myapp-helm.yml"), []byte(helmFile), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Update only grafana using --helm
	_, err := executeCommand("update", "production", "myapp", "--helm", "grafana:8.0.0")
	if err != nil {
		t.Fatalf("update --helm failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(clusterDir, "myapp-helm.yml"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "'8.0.0'") {
		t.Error("grafana version not updated")
	}
	if !strings.Contains(string(content), "'5.0.0'") {
		t.Error("loki version should not have changed")
	}
}

func TestUpdateHelmVersionFailsWithMultiple(t *testing.T) {
	dir := t.TempDir()

	clusterDir := filepath.Join(dir, "clusters", "production")
	os.MkdirAll(clusterDir, 0o755)

	helmFile := `---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: grafana
spec:
  chart:
    spec:
      chart: grafana
      version: '7.0.0'
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: loki
spec:
  chart:
    spec:
      chart: loki
      version: '5.0.0'
`
	os.WriteFile(filepath.Join(clusterDir, "myapp-helm.yml"), []byte(helmFile), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// --helm-version should fail with multiple HelmReleases
	_, err := executeCommand("update", "production", "myapp", "--helm-version", "9.0.0")
	if err == nil {
		t.Fatal("expected error when using --helm-version with multiple HelmReleases")
	}
	if !strings.Contains(err.Error(), "multiple HelmReleases") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAddArgOrder(t *testing.T) {
	dir := t.TempDir()

	// Create namespaces dir with kustomization.yaml for the app
	nsDir := filepath.Join(dir, "clusters", "production-namespaces", "myapp")
	os.MkdirAll(nsDir, 0o755)
	ksContent := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- namespace.yaml
`
	os.WriteFile(filepath.Join(nsDir, "kustomization.yaml"), []byte(ksContent), 0o644)
	os.WriteFile(filepath.Join(nsDir, "namespace.yaml"), []byte("apiVersion: v1\nkind: Namespace\nmetadata:\n  name: myapp\n"), 0o644)

	// Create cluster dir
	os.MkdirAll(filepath.Join(dir, "clusters", "production", "myapp"), 0o755)

	// Create a template extra
	extraDir := filepath.Join(dir, ".flaxx", "templates", "test-extra")
	os.MkdirAll(extraDir, 0o755)
	os.WriteFile(filepath.Join(extraDir, "_meta.yaml"), []byte("name: test-extra\ndescription: test\ntarget: namespaces\n"), 0o644)
	os.WriteFile(filepath.Join(extraDir, "test.yaml"), []byte("# test file for {{.App}}\n"), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Args: <cluster> <app> → add production myapp
	_, err := executeCommand("add", "production", "myapp", "-e", "test-extra")
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	// Verify extra was rendered in the correct namespaces dir
	testFile := filepath.Join(nsDir, "test.yaml")
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("extra file not created at expected path: %v", err)
	}
	if !strings.Contains(string(content), "# test file for myapp") {
		t.Error("extra file has wrong app name — args may be swapped")
	}
}

func TestCheckArgOrder(t *testing.T) {
	dir := t.TempDir()

	// Create app files with cluster=production, app=myapp — but NO helm resources
	// to test that it looks in the right directory
	appDir := filepath.Join(dir, "clusters", "production", "myapp")
	os.MkdirAll(appDir, 0o755)
	os.WriteFile(filepath.Join(appDir, "myapp-kustomization.yaml"), []byte("kind: Kustomization\n"), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Args: <cluster> <app> → check production myapp
	// This should fail with "no Helm charts or container images found for myapp"
	// If args were swapped, it would fail with a different error about "myapp" directory not found
	_, err := executeCommand("check", "production", "myapp")
	if err == nil {
		t.Fatal("expected error (no helm/images), got nil")
	}
	if !strings.Contains(err.Error(), "myapp") {
		t.Errorf("error should reference app name 'myapp', got: %v", err)
	}

	// Verify it does NOT look in clusters/myapp/production/ (swapped)
	wrongDir := filepath.Join(dir, "clusters", "myapp", "production")
	if _, err := os.Stat(wrongDir); err == nil {
		t.Error("checked wrong directory — args are swapped")
	}
}

func TestTemplateFromAppRoundTrip(t *testing.T) {
	dir := t.TempDir()

	// Scaffold a helm app via the `generate` command so we exercise the
	// real code path and can round-trip through `template from-app`.
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	if _, err := executeCommand("generate", "lab", "podinfo", "-t", "core-helm",
		"--helm-url", "https://stefanprodan.github.io/podinfo",
		"--helm-version", "6.5.0"); err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Add a parameterizable ingress resource to the namespace dir.
	ingressPath := filepath.Join(dir, "clusters", "lab-namespaces", "podinfo", "ingress.yaml")
	ingress := `---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: podinfo
spec:
  rules:
    - host: podinfo.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: podinfo
                port:
                  number: 80
`
	os.WriteFile(ingressPath, []byte(ingress), 0o644)

	out, err := executeCommand("template", "from-app", "lab", "podinfo", "my-podinfo")
	if err != nil {
		t.Fatalf("template from-app: %v (out: %s)", err, out)
	}

	meta, err := os.ReadFile(filepath.Join(dir, ".flaxx", "templates", "my-podinfo", "_meta.yaml"))
	if err != nil {
		t.Fatalf("_meta.yaml missing: %v", err)
	}
	if !strings.Contains(string(meta), "chart_version") {
		t.Errorf("_meta.yaml missing chart_version:\n%s", string(meta))
	}
	if !strings.Contains(string(meta), "ingress_host") {
		t.Errorf("_meta.yaml missing ingress_host:\n%s", string(meta))
	}

	ingressTemplate, err := os.ReadFile(filepath.Join(dir, ".flaxx", "templates", "my-podinfo", "ingress.yaml"))
	if err != nil {
		t.Fatalf("ingress.yaml missing: %v", err)
	}
	its := string(ingressTemplate)
	if strings.Contains(its, "podinfo.example.com") {
		t.Errorf("ingress.yaml should have ingress host replaced:\n%s", its)
	}
	if !strings.Contains(its, "{{.ingress_host}}") {
		t.Errorf("ingress.yaml should contain {{.ingress_host}}:\n%s", its)
	}
}

func TestTemplateFromAppIncludeCluster(t *testing.T) {
	dir := t.TempDir()
	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	if _, err := executeCommand("generate", "lab", "podinfo", "-t", "core-helm",
		"--helm-url", "https://stefanprodan.github.io/podinfo",
		"--helm-version", "6.5.0"); err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := executeCommand("template", "from-app", "lab", "podinfo", "my-full",
		"--include-cluster"); err != nil {
		t.Fatalf("template from-app: %v", err)
	}

	clusterFile := filepath.Join(dir, ".flaxx", "templates", "my-full", "cluster", "podinfo-helm.yml")
	if _, err := os.Stat(clusterFile); err != nil {
		t.Fatalf("cluster/ file missing: %v", err)
	}
	nsFile := filepath.Join(dir, ".flaxx", "templates", "my-full", "namespaces", "namespace.yaml")
	if _, err := os.Stat(nsFile); err != nil {
		t.Fatalf("namespaces/ file missing: %v", err)
	}
	meta, _ := os.ReadFile(filepath.Join(dir, ".flaxx", "templates", "my-full", "_meta.yaml"))
	if !strings.Contains(string(meta), "target: split") {
		t.Errorf("_meta.yaml should have target split:\n%s", string(meta))
	}
}
