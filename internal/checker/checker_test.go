package checker

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestScanAllHelm(t *testing.T) {
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

	infos, err := ScanAllHelm(dir, "")
	if err != nil {
		t.Fatalf("ScanAllHelm failed: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}

	info := infos[0]
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

func TestScanAllHelm_Multiple(t *testing.T) {
	dir := t.TempDir()

	helmFile := `---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: grafana
  namespace: monitoring
spec:
  url: https://grafana.github.io/helm-charts
---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: prometheus
  namespace: monitoring
spec:
  url: https://prometheus-community.github.io/helm-charts
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: grafana
  namespace: monitoring
spec:
  chart:
    spec:
      chart: grafana
      version: '7.0.0'
      sourceRef:
        kind: HelmRepository
        name: grafana
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: prometheus
  namespace: monitoring
spec:
  chart:
    spec:
      chart: prometheus
      version: '25.0.0'
      sourceRef:
        kind: HelmRepository
        name: prometheus
`
	if err := os.WriteFile(filepath.Join(dir, "monitoring-helm.yml"), []byte(helmFile), 0o644); err != nil {
		t.Fatal(err)
	}

	infos, err := ScanAllHelm(dir, "")
	if err != nil {
		t.Fatalf("ScanAllHelm failed: %v", err)
	}

	if len(infos) != 2 {
		t.Fatalf("got %d infos, want 2", len(infos))
	}

	// Verify both charts found with correct repo URLs matched by sourceRef
	charts := make(map[string]HelmInfo)
	for _, info := range infos {
		charts[info.ChartName] = info
	}

	grafana, ok := charts["grafana"]
	if !ok {
		t.Fatal("grafana chart not found")
	}
	if grafana.RepoURL != "https://grafana.github.io/helm-charts" {
		t.Errorf("grafana RepoURL = %q", grafana.RepoURL)
	}
	if grafana.CurrentVersion != "7.0.0" {
		t.Errorf("grafana CurrentVersion = %q", grafana.CurrentVersion)
	}

	prom, ok := charts["prometheus"]
	if !ok {
		t.Fatal("prometheus chart not found")
	}
	if prom.RepoURL != "https://prometheus-community.github.io/helm-charts" {
		t.Errorf("prometheus RepoURL = %q", prom.RepoURL)
	}
}

func TestScanAllHelm_OCI(t *testing.T) {
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

	infos, err := ScanAllHelm(dir, "")
	if err != nil {
		t.Fatalf("ScanAllHelm failed: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}
	if infos[0].RepoType != "oci" {
		t.Errorf("RepoType = %q, want %q", infos[0].RepoType, "oci")
	}
}

func TestScanAllHelm_NoHelmRelease(t *testing.T) {
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

	infos, err := ScanAllHelm(dir, "")
	if err != nil {
		t.Fatal("expected nil error for no helm resources")
	}
	if len(infos) != 0 {
		t.Errorf("got %d infos, want 0", len(infos))
	}
}

func TestScanAllHelm_AppFilter(t *testing.T) {
	dir := t.TempDir()

	// Two apps in same directory (flat layout)
	for _, app := range []string{"app1", "app2"} {
		helmFile := fmt.Sprintf(`---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: %s
spec:
  url: https://charts.example.com/%s
---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: %s
spec:
  chart:
    spec:
      chart: %s
      version: '1.0.0'
      sourceRef:
        kind: HelmRepository
        name: %s
`, app, app, app, app, app)
		if err := os.WriteFile(filepath.Join(dir, app+"-helm.yml"), []byte(helmFile), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Filter to app1 only
	infos, err := ScanAllHelm(dir, "app1")
	if err != nil {
		t.Fatalf("ScanAllHelm failed: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}
	if infos[0].ChartName != "app1" {
		t.Errorf("ChartName = %q, want %q", infos[0].ChartName, "app1")
	}
}

func TestScanAllHelm_NoVersionPinned(t *testing.T) {
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

	infos, err := ScanAllHelm(dir, "")
	if err != nil {
		t.Fatalf("ScanAllHelm failed: %v", err)
	}

	if len(infos) != 1 {
		t.Fatalf("got %d infos, want 1", len(infos))
	}
	if infos[0].CurrentVersion != "" {
		t.Errorf("CurrentVersion = %q, want empty", infos[0].CurrentVersion)
	}
}
