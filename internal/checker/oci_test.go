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
		url      string
		chart    string
		wantReg  string
		wantRepo string
		wantErr  bool
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

func TestFetchOCIVersions_Pagination(t *testing.T) {
	page1 := tagList{Name: "org/myapp", Tags: []string{"1.0.0", "1.1.0"}}
	page2 := tagList{Name: "org/myapp", Tags: []string{"2.0.0", "2.1.0"}}
	page3 := tagList{Name: "org/myapp", Tags: []string{"3.0.0"}}

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/org/myapp/tags/list" && r.URL.Query().Get("last") == "":
			w.Header().Set("Link", `</v2/org/myapp/tags/list?last=1.1.0>; rel="next"`)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(page1)
		case r.URL.Path == "/v2/org/myapp/tags/list" && r.URL.Query().Get("last") == "1.1.0":
			w.Header().Set("Link", `</v2/org/myapp/tags/list?last=2.1.0>; rel="next"`)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(page2)
		case r.URL.Path == "/v2/org/myapp/tags/list" && r.URL.Query().Get("last") == "2.1.0":
			// Last page, no Link header
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(page3)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	host := strings.TrimPrefix(server.URL, "https://")
	repoURL := "oci://" + host + "/org"

	origFetch := fetchTagsFunc
	fetchTagsFunc = func(client *http.Client, registry, repository string) ([]string, error) {
		return fetchTags(server.Client(), registry, repository)
	}
	defer func() { fetchTagsFunc = origFetch }()

	versions, err := FetchOCIVersions(repoURL, "myapp")
	if err != nil {
		t.Fatalf("FetchOCIVersions failed: %v", err)
	}

	if len(versions) != 5 {
		t.Fatalf("got %d versions, want 5 (across 3 pages)", len(versions))
	}

	if versions[0].Original() != "3.0.0" {
		t.Errorf("latest version = %q, want %q", versions[0].Original(), "3.0.0")
	}
}

func TestParseNextLink(t *testing.T) {
	tests := []struct {
		header   string
		registry string
		want     string
	}{
		{`</v2/org/app/tags/list?last=v1.0>; rel="next"`, "ghcr.io", "https://ghcr.io/v2/org/app/tags/list?last=v1.0"},
		{`<https://ghcr.io/v2/org/app/tags/list?last=v1.0>; rel="next"`, "ghcr.io", "https://ghcr.io/v2/org/app/tags/list?last=v1.0"},
		{`</v2/org/app/tags/list?n=100&last=v2.0>; rel="next"`, "reg.io", "https://reg.io/v2/org/app/tags/list?n=100&last=v2.0"},
		{"", "ghcr.io", ""},
		{`<https://example.com/something>; rel="other"`, "ghcr.io", ""},
	}

	for _, tt := range tests {
		got := parseNextLink(tt.header, tt.registry)
		if got != tt.want {
			t.Errorf("parseNextLink(%q, %q) = %q, want %q", tt.header, tt.registry, got, tt.want)
		}
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
