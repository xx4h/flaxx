package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetFlags resets package-level flag variables that persist between cobra
// command executions in tests.
func resetFlags() {
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
	updateImage = ""
	updateNamespace = ""
	updateDryRun = false
	addExtraNames = nil
	addSetVars = nil
	addNamespace = ""
	addDryRun = false
	checkNamespace = ""
	checkAll = false
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

	// Verify files went to the right cluster directory
	ksFile := filepath.Join(dir, "clusters", "staging", "webapp", "webapp-kustomization.yaml")
	content, err := os.ReadFile(ksFile)
	if err != nil {
		t.Fatalf("kustomization not created at expected path: %v", err)
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
	wrongDir := filepath.Join(dir, "clusters", "webapp")
	if _, err := os.Stat(wrongDir); err == nil {
		t.Error("files created under clusters/webapp/ — args are swapped (app used as cluster)")
	}
}

func TestUpdateArgOrder(t *testing.T) {
	dir := t.TempDir()

	// Create app files with cluster=production, app=myapp
	appDir := filepath.Join(dir, "clusters", "production", "myapp")
	os.MkdirAll(appDir, 0o755)

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
	os.WriteFile(filepath.Join(appDir, "myapp-helm.yml"), []byte(helmFile), 0o644)

	origDir, _ := os.Getwd()
	os.Chdir(dir)
	defer os.Chdir(origDir)

	// Args: <cluster> <app> → update production myapp
	_, err := executeCommand("update", "production", "myapp", "--helm-version", "2.0.0")
	if err != nil {
		t.Fatalf("update failed: %v", err)
	}

	// Verify the file was updated in the correct location
	content, err := os.ReadFile(filepath.Join(appDir, "myapp-helm.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "2.0.0") {
		t.Error("helm version not updated — args may be swapped")
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
