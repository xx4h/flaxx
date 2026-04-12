package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestScanApp(t *testing.T) {
	dir := t.TempDir()

	helmFile := `---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: myapp
  namespace: myapp
spec:
  interval: 0h10m0s
  url: https://charts.example.com
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: myapp
  namespace: myapp
spec:
  chart:
    spec:
      chart: myapp
      version: '1.2.3'
      sourceRef:
        kind: HelmRepository
        name: myapp
        namespace: myapp
  interval: 0h10m0s
  values: {}
`
	if err := os.WriteFile(filepath.Join(dir, "myapp-helm.yml"), []byte(helmFile), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := ScanApp(dir)
	if err != nil {
		t.Fatalf("ScanApp failed: %v", err)
	}

	if info.ChartName != "myapp" {
		t.Errorf("ChartName = %q, want %q", info.ChartName, "myapp")
	}
	if info.CurrentVersion != "1.2.3" {
		t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "1.2.3")
	}
	if info.RepoURL != "https://charts.example.com" {
		t.Errorf("RepoURL = %q, want %q", info.RepoURL, "https://charts.example.com")
	}
	if info.Namespace != "myapp" {
		t.Errorf("Namespace = %q, want %q", info.Namespace, "myapp")
	}
}

func TestScanApp_OCI(t *testing.T) {
	dir := t.TempDir()

	helmFile := `---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: myapp
  namespace: myapp
spec:
  type: oci
  url: oci://registry.example.com/charts
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: myapp
  namespace: myapp
spec:
  chart:
    spec:
      chart: myapp
      version: '2.0.0'
      sourceRef:
        kind: HelmRepository
        name: myapp
        namespace: myapp
`
	if err := os.WriteFile(filepath.Join(dir, "myapp-helm.yml"), []byte(helmFile), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := ScanApp(dir)
	if err != nil {
		t.Fatalf("ScanApp failed: %v", err)
	}

	if info.RepoType != "oci" {
		t.Errorf("RepoType = %q, want %q", info.RepoType, "oci")
	}
}

func TestScanApp_NoHelmRelease(t *testing.T) {
	dir := t.TempDir()

	ksFile := `---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: myapp
  namespace: flux-system
spec:
  interval: 2m
`
	if err := os.WriteFile(filepath.Join(dir, "myapp-kustomization.yaml"), []byte(ksFile), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ScanApp(dir)
	if err == nil {
		t.Fatal("expected error for missing HelmRepository, got nil")
	}
}

func TestScanCluster(t *testing.T) {
	dir := t.TempDir()

	// Create two app dirs with Helm resources
	for _, app := range []string{"app1", "app2"} {
		appDir := filepath.Join(dir, app)
		if err := os.MkdirAll(appDir, 0o755); err != nil {
			t.Fatal(err)
		}
		helmFile := fmt.Sprintf(`---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: %s
  namespace: %s
spec:
  url: https://charts.example.com/%s
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: %s
  namespace: %s
spec:
  chart:
    spec:
      chart: %s
      version: '1.0.0'
`, app, app, app, app, app, app)
		if err := os.WriteFile(filepath.Join(appDir, app+"-helm.yml"), []byte(helmFile), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a non-helm app dir (should be skipped)
	nonHelmDir := filepath.Join(dir, "app3")
	if err := os.MkdirAll(nonHelmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ksFile := `---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: app3
`
	if err := os.WriteFile(filepath.Join(nonHelmDir, "app3-kustomization.yaml"), []byte(ksFile), 0o644); err != nil {
		t.Fatal(err)
	}

	infos, err := ScanCluster(dir)
	if err != nil {
		t.Fatalf("ScanCluster failed: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("got %d infos, want 2", len(infos))
	}

	for _, info := range infos {
		if info.App != "app1" && info.App != "app2" {
			t.Errorf("unexpected app: %s", info.App)
		}
	}
}

func TestScanApp_NoVersionPinned(t *testing.T) {
	dir := t.TempDir()

	helmFile := `---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: myapp
  namespace: myapp
spec:
  url: https://charts.example.com
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: myapp
  namespace: myapp
spec:
  chart:
    spec:
      chart: myapp
      sourceRef:
        kind: HelmRepository
        name: myapp
`
	if err := os.WriteFile(filepath.Join(dir, "myapp-helm.yml"), []byte(helmFile), 0o644); err != nil {
		t.Fatal(err)
	}

	info, err := ScanApp(dir)
	if err != nil {
		t.Fatalf("ScanApp failed: %v", err)
	}

	if info.CurrentVersion != "" {
		t.Errorf("CurrentVersion = %q, want empty", info.CurrentVersion)
	}
}
