package checker

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseOCIURL(t *testing.T) {
	tests := []struct {
		url       string
		chart     string
		wantReg   string
		wantRepo  string
		wantErr   bool
	}{
		{"oci://registry.example.com/charts", "myapp", "registry.example.com", "charts/myapp", false},
		{"oci://ghcr.io/org", "mychart", "ghcr.io", "org/mychart", false},
		{"oci://registry.example.com", "myapp", "registry.example.com", "myapp", false},
		{"oci://registry.example.com/charts/", "myapp", "registry.example.com", "charts/myapp", false},
		{"https://not-oci.example.com", "myapp", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			reg, repo, err := parseOCIURL(tt.url, tt.chart)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr = %v", err, tt.wantErr)
			}
			if reg != tt.wantReg {
				t.Errorf("registry = %q, want %q", reg, tt.wantReg)
			}
			if repo != tt.wantRepo {
				t.Errorf("repository = %q, want %q", repo, tt.wantRepo)
			}
		})
	}
}

func TestFetchOCIVersions(t *testing.T) {
	tags := tagList{
		Name: "charts/myapp",
		Tags: []string{"1.0.0", "1.1.0", "2.0.0", "latest", "invalid"},
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/charts/myapp/tags/list" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tags)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Extract host from test server URL (https://127.0.0.1:PORT)
	host := strings.TrimPrefix(server.URL, "https://")
	repoURL := "oci://" + host + "/charts"

	// Use the test server's client which trusts its TLS cert
	origFetch := fetchTagsFunc
	fetchTagsFunc = func(client *http.Client, registry, repository string) ([]string, error) {
		return fetchTags(server.Client(), registry, repository)
	}
	defer func() { fetchTagsFunc = origFetch }()

	versions, err := FetchOCIVersions(repoURL, "myapp")
	if err != nil {
		t.Fatalf("FetchOCIVersions failed: %v", err)
	}

	// Should have 3 semver versions (latest and invalid are skipped)
	if len(versions) != 3 {
		t.Fatalf("got %d versions, want 3", len(versions))
	}

	// Should be sorted newest first
	if versions[0].Original() != "2.0.0" {
		t.Errorf("first version = %q, want %q", versions[0].Original(), "2.0.0")
	}
}

func TestFetchOCIVersions_WithTokenAuth(t *testing.T) {
	tags := tagList{
		Name: "charts/myapp",
		Tags: []string{"1.0.0", "2.0.0"},
	}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/charts/myapp/tags/list":
			auth := r.Header.Get("Authorization")
			if auth != "Bearer test-token" {
				realm := fmt.Sprintf("https://%s/token", r.Host)
				w.Header().Set("Www-Authenticate",
					fmt.Sprintf(`Bearer realm="%s",service="%s",scope="repository:charts/myapp:pull"`, realm, r.Host))
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tags)
		case "/token":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(tokenResponse{Token: "test-token"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	repoURL := "oci://" + host + "/charts"

	origFetch := fetchTagsFunc
	fetchTagsFunc = func(client *http.Client, registry, repository string) ([]string, error) {
		return fetchTags(server.Client(), registry, repository)
	}
	defer func() { fetchTagsFunc = origFetch }()

	versions, err := FetchOCIVersions(repoURL, "myapp")
	if err != nil {
		t.Fatalf("FetchOCIVersions failed: %v", err)
	}

	if len(versions) != 2 {
		t.Fatalf("got %d versions, want 2", len(versions))
	}
}

func TestCheckHelm_OCI(t *testing.T) {
	tags := tagList{
		Name: "charts/myapp",
		Tags: []string{"1.0.0", "1.2.3", "1.5.0", "2.0.0"},
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

	info := &HelmInfo{
		App:            "myapp",
		ChartName:      "myapp",
		CurrentVersion: "1.2.3",
		RepoURL:        "oci://" + host + "/charts",
		RepoType:       "oci",
	}

	result, err := CheckHelm(info)
	if err != nil {
		t.Fatalf("CheckHelm failed: %v", err)
	}

	if result.LatestVersion != "2.0.0" {
		t.Errorf("LatestVersion = %q, want %q", result.LatestVersion, "2.0.0")
	}

	if len(result.AvailableUpdates) != 2 {
		t.Errorf("got %d updates, want 2: %v", len(result.AvailableUpdates), result.AvailableUpdates)
	}
}
