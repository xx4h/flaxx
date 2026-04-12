package checker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseImageRef(t *testing.T) {
	tests := []struct {
		ref      string
		wantReg  string
		wantRepo string
		wantTag  string
	}{
		{"nginx:1.25", "registry-1.docker.io", "library/nginx", "1.25"},
		{"org/app:v2.0", "registry-1.docker.io", "org/app", "v2.0"},
		{"ghcr.io/org/app:1.0.0", "ghcr.io", "org/app", "1.0.0"},
		{"registry.example.com/org/app:latest", "registry.example.com", "org/app", "latest"},
		{"registry.example.com/org/app", "registry.example.com", "org/app", ""},
		{"nginx", "registry-1.docker.io", "library/nginx", ""},
		{"localhost/myapp:dev", "localhost", "myapp", "dev"},
		{"reg.io:5000/app:v1", "reg.io:5000", "app", "v1"},
		{"ghcr.io/org/app@sha256:abc123", "ghcr.io", "org/app", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ref, func(t *testing.T) {
			reg, repo, tag := ParseImageRef(tt.ref)
			if reg != tt.wantReg {
				t.Errorf("registry = %q, want %q", reg, tt.wantReg)
			}
			if repo != tt.wantRepo {
				t.Errorf("repo = %q, want %q", repo, tt.wantRepo)
			}
			if tag != tt.wantTag {
				t.Errorf("tag = %q, want %q", tag, tt.wantTag)
			}
		})
	}
}

func TestScanImages(t *testing.T) {
	dir := t.TempDir()

	deployFile := `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
spec:
  template:
    spec:
      containers:
      - name: app
        image: ghcr.io/org/myapp:1.2.3
      - name: sidecar
        image: nginx:1.25
`
	if err := os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte(deployFile), 0o644); err != nil {
		t.Fatal(err)
	}

	images, err := ScanImages(dir)
	if err != nil {
		t.Fatalf("ScanImages failed: %v", err)
	}

	if len(images) != 2 {
		t.Fatalf("got %d images, want 2", len(images))
	}

	if images[0].Container != "app" || images[0].Image != "ghcr.io/org/myapp:1.2.3" {
		t.Errorf("first image: container=%q image=%q", images[0].Container, images[0].Image)
	}
	if images[0].Registry != "ghcr.io" || images[0].Repo != "org/myapp" || images[0].Tag != "1.2.3" {
		t.Errorf("first image parsed: reg=%q repo=%q tag=%q", images[0].Registry, images[0].Repo, images[0].Tag)
	}

	if images[1].Container != "sidecar" || images[1].Tag != "1.25" {
		t.Errorf("second image: container=%q tag=%q", images[1].Container, images[1].Tag)
	}
}

func TestScanImages_StatefulSet(t *testing.T) {
	dir := t.TempDir()

	ssFile := `---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: db
spec:
  template:
    spec:
      containers:
      - name: postgres
        image: postgres:15.2
`
	if err := os.WriteFile(filepath.Join(dir, "statefulset.yaml"), []byte(ssFile), 0o644); err != nil {
		t.Fatal(err)
	}

	images, err := ScanImages(dir)
	if err != nil {
		t.Fatalf("ScanImages failed: %v", err)
	}

	if len(images) != 1 {
		t.Fatalf("got %d images, want 1", len(images))
	}

	if images[0].Container != "postgres" {
		t.Errorf("container = %q, want %q", images[0].Container, "postgres")
	}
}

func TestScanImages_SkipsNonWorkloads(t *testing.T) {
	dir := t.TempDir()

	nsFile := `---
apiVersion: v1
kind: Namespace
metadata:
  name: myapp
`
	if err := os.WriteFile(filepath.Join(dir, "namespace.yaml"), []byte(nsFile), 0o644); err != nil {
		t.Fatal(err)
	}

	images, err := ScanImages(dir)
	if err != nil {
		t.Fatalf("ScanImages failed: %v", err)
	}

	if len(images) != 0 {
		t.Errorf("got %d images, want 0", len(images))
	}
}

func TestCheckImage(t *testing.T) {
	tags := tagList{
		Name: "org/myapp",
		Tags: []string{"1.0.0", "1.2.3", "1.5.0", "2.0.0", "latest"},
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tags)
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")

	origFetch := fetchTagsFunc
	fetchTagsFunc = func(client *http.Client, registry, repository string) ([]string, error) {
		return fetchTags(server.Client(), registry, repository)
	}
	defer func() { fetchTagsFunc = origFetch }()

	info := ImageInfo{
		Container: "app",
		Image:     host + "/org/myapp:1.2.3",
		Registry:  host,
		Repo:      "org/myapp",
		Tag:       "1.2.3",
	}

	result, err := CheckImage(info)
	if err != nil {
		t.Fatalf("CheckImage failed: %v", err)
	}

	if result.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "2.0.0")
	}

	if len(result.AvailableUpdates) != 2 {
		t.Errorf("got %d updates, want 2: %v", len(result.AvailableUpdates), result.AvailableUpdates)
	}
}
