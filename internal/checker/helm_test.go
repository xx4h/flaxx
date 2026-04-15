package checker

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

const testIndex = `apiVersion: v1
entries:
  myapp:
    - version: "1.0.0"
    - version: "1.1.0"
    - version: "1.2.3"
    - version: "1.3.0"
    - version: "2.0.0"
    - version: "2.1.0-rc.1"
  other-chart:
    - version: "0.1.0"
`

func TestFetchHelmVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/index.yaml" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprint(w, testIndex)
	}))
	defer server.Close()

	versions, err := FetchHelmVersions(server.URL, "myapp")
	if err != nil {
		t.Fatalf("FetchHelmVersions failed: %v", err)
	}

	if len(versions) != 6 {
		t.Fatalf("got %d versions, want 6", len(versions))
	}

	// Should be sorted newest first
	if versions[0].Tag != "2.1.0-rc.1" {
		t.Errorf("first version = %q, want %q", versions[0].Tag, "2.1.0-rc.1")
	}
}

func TestFetchHelmVersions_ChartNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testIndex)
	}))
	defer server.Close()

	_, err := FetchHelmVersions(server.URL, "nonexistent")
	if err == nil {
		t.Fatal("expected error for missing chart, got nil")
	}
}

func TestCheckHelm(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, testIndex)
	}))
	defer server.Close()

	info := &HelmInfo{
		App:            "myapp",
		ChartName:      "myapp",
		CurrentVersion: "1.2.3",
		RepoURL:        server.URL,
	}

	result, err := CheckHelm(info, FilterAll)
	if err != nil {
		t.Fatalf("CheckHelm failed: %v", err)
	}

	if result.LatestVersion != "2.1.0-rc.1" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "2.1.0-rc.1")
	}

	// Available updates should be versions newer than 1.2.3
	if len(result.AvailableUpdates) != 3 {
		t.Errorf("got %d available updates, want 3: %v", len(result.AvailableUpdates), result.AvailableUpdates)
	}
}

func TestCheckHelm_UpToDate(t *testing.T) {
	index := `apiVersion: v1
entries:
  myapp:
    - version: "1.0.0"
    - version: "1.2.3"
`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, index)
	}))
	defer server.Close()

	info := &HelmInfo{
		ChartName:      "myapp",
		CurrentVersion: "1.2.3",
		RepoURL:        server.URL,
	}

	result, err := CheckHelm(info, FilterAll)
	if err != nil {
		t.Fatalf("CheckHelm failed: %v", err)
	}

	if len(result.AvailableUpdates) != 0 {
		t.Errorf("expected no updates, got %v", result.AvailableUpdates)
	}
}
