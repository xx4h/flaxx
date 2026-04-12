package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpdateHelmVersion(t *testing.T) {
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
      version: '1.0.0'
      sourceRef:
        kind: HelmRepository
        name: myapp
  interval: 0h10m0s
`
	os.WriteFile(filepath.Join(dir, "myapp-helm.yml"), []byte(helmFile), 0o644)

	file, err := UpdateHelmVersion(dir, "2.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if file == "" {
		t.Fatal("expected file path")
	}

	content, err := os.ReadFile(filepath.Join(dir, "myapp-helm.yml"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "'2.0.0'") {
		t.Errorf("version not updated, got:\n%s", string(content))
	}
}

func TestUpdateHelmVersionDryRun(t *testing.T) {
	dir := t.TempDir()

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
	os.WriteFile(filepath.Join(dir, "myapp-helm.yml"), []byte(helmFile), 0o644)

	_, err := UpdateHelmVersion(dir, "2.0.0", true)
	if err != nil {
		t.Fatal(err)
	}

	// Verify file was NOT changed
	content, err := os.ReadFile(filepath.Join(dir, "myapp-helm.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "2.0.0") {
		t.Error("dry run should not modify file")
	}
}

func TestUpdateHelmVersionNoHelmRelease(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "kustomization.yaml"), []byte("kind: Kustomization\n"), 0o644)

	_, err := UpdateHelmVersion(dir, "2.0.0", false)
	if err == nil {
		t.Fatal("expected error when no HelmRelease found")
	}
}

func TestUpdateImage(t *testing.T) {
	dir := t.TempDir()

	deployment := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
      - name: myapp
        image: registry/myapp:v1.0.0
        ports:
        - containerPort: 8080
`
	os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte(deployment), 0o644)

	file, err := UpdateImage(dir, "registry/myapp:v2.0.0", false)
	if err != nil {
		t.Fatal(err)
	}
	if file == "" {
		t.Fatal("expected file path")
	}

	content, err := os.ReadFile(filepath.Join(dir, "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "registry/myapp:v2.0.0") {
		t.Errorf("image not updated, got:\n%s", string(content))
	}
}

func TestUpdateImageByName(t *testing.T) {
	dir := t.TempDir()

	deployment := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
      - name: main
        image: registry/main:v1.0.0
      - name: sidecar
        image: registry/sidecar:v1.0.0
`
	os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte(deployment), 0o644)

	_, err := UpdateImage(dir, "sidecar=registry/sidecar:v2.0.0", false)
	if err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "deployment.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(string(content), "registry/sidecar:v2.0.0") {
		t.Error("sidecar image not updated")
	}
	if !strings.Contains(string(content), "registry/main:v1.0.0") {
		t.Error("main image should not have changed")
	}
}

func TestUpdateImageNoDeployment(t *testing.T) {
	dir := t.TempDir()

	os.WriteFile(filepath.Join(dir, "service.yaml"), []byte("kind: Service\n"), 0o644)

	_, err := UpdateImage(dir, "registry/myapp:v2.0.0", false)
	if err == nil {
		t.Fatal("expected error when no Deployment found")
	}
}
