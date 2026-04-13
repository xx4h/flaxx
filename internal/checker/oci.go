package checker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
)

// tagList represents the OCI distribution API tag list response.
type tagList struct {
	Name string   `json:"name"`
	Tags []string `json:"tags"`
}

// tokenResponse represents a Bearer token response from an auth endpoint.
type tokenResponse struct {
	Token string `json:"token"`
}

// fetchTagsFunc is the function used to fetch tags. Replaceable for testing.
var fetchTagsFunc = fetchTags

func defaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second}
}

// FetchOCIVersions queries an OCI registry for available tags of a chart,
// sorted newest first.
func FetchOCIVersions(repoURL, chartName string) ([]*semver.Version, error) {
	// Parse OCI URL: oci://registry.example.com/path -> registry.example.com, path/chartName
	registry, repository, err := parseOCIURL(repoURL, chartName)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 30 * time.Second}

	tags, err := fetchTagsFunc(client, registry, repository)
	if err != nil {
		return nil, err
	}

	var versions []*semver.Version
	for _, tag := range tags {
		v, err := semver.NewVersion(tag)
		if err != nil {
			continue // skip non-semver tags
		}
		versions = append(versions, v)
	}

	sort.Sort(sort.Reverse(semver.Collection(versions)))

	return versions, nil
}

// parseOCIURL extracts the registry host and repository path from an OCI URL.
// Input: oci://registry.example.com/charts, chartName
// Output: registry.example.com, charts/chartName
func parseOCIURL(repoURL, chartName string) (string, string, error) {
	url := strings.TrimPrefix(repoURL, "oci://")
	if url == repoURL {
		return "", "", fmt.Errorf("not an OCI URL: %s", repoURL)
	}

	url = strings.TrimRight(url, "/")

	parts := strings.SplitN(url, "/", 2)
	registry := parts[0]
	path := ""
	if len(parts) > 1 {
		path = parts[1]
	}

	repository := chartName
	if path != "" {
		repository = path + "/" + chartName
	}

	return registry, repository, nil
}

// fetchTags retrieves the full tag list from an OCI registry, handling
// token-based auth and pagination via Link headers.
func fetchTags(client *http.Client, registry, repository string) ([]string, error) {
	tagsURL := fmt.Sprintf("https://%s/v2/%s/tags/list", registry, repository)

	// Initial request to discover auth requirements
	resp, err := client.Get(tagsURL)
	if err != nil {
		return nil, fmt.Errorf("fetching tags from %s: %w", tagsURL, err)
	}
	defer resp.Body.Close()

	var token string

	// Handle 401 with Bearer token challenge
	if resp.StatusCode == http.StatusUnauthorized {
		token, err = requestToken(client, resp, registry, repository)
		if err != nil {
			return nil, fmt.Errorf("authenticating to %s: %w", registry, err)
		}

		req, err := http.NewRequest("GET", tagsURL, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)

		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching tags from %s: %w", tagsURL, err)
		}
		defer resp.Body.Close()
	}

	// Read first page and follow pagination
	var allTags []string
	for {
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetching tags from %s: HTTP %d", tagsURL, resp.StatusCode)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("reading response: %w", err)
		}

		var tags tagList
		if unmarshalErr := json.Unmarshal(body, &tags); unmarshalErr != nil {
			return nil, fmt.Errorf("parsing tags: %w", unmarshalErr)
		}
		allTags = append(allTags, tags.Tags...)

		// Check for next page via Link header
		nextURL := parseNextLink(resp.Header.Get("Link"), registry)
		if nextURL == "" {
			break
		}

		req, err := http.NewRequest("GET", nextURL, nil)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}

		resp, err = client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetching next page: %w", err)
		}
		defer resp.Body.Close()
	}

	return allTags, nil
}

// linkNextRegex matches the "next" relation in a Link header.
var linkNextRegex = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// parseNextLink extracts the next page URL from a Link header.
// Handles both absolute URLs and relative paths.
func parseNextLink(header, registry string) string {
	if header == "" {
		return ""
	}
	matches := linkNextRegex.FindStringSubmatch(header)
	if len(matches) < 2 {
		return ""
	}
	url := matches[1]
	// Some registries return relative URLs
	if strings.HasPrefix(url, "/") {
		url = "https://" + registry + url
	}
	return url
}

// wwwAuthRegex parses the Www-Authenticate header for Bearer token challenges.
var wwwAuthRegex = regexp.MustCompile(`(\w+)="([^"]*)"`)

// requestToken attempts to obtain a Bearer token using the Www-Authenticate challenge.
func requestToken(client *http.Client, resp *http.Response, registry, repository string) (string, error) {
	wwwAuth := resp.Header.Get("Www-Authenticate")
	if wwwAuth == "" {
		return "", fmt.Errorf("no Www-Authenticate header in 401 response from %s", registry)
	}

	params := make(map[string]string)
	for _, match := range wwwAuthRegex.FindAllStringSubmatch(wwwAuth, -1) {
		params[match[1]] = match[2]
	}

	realm, ok := params["realm"]
	if !ok {
		return "", fmt.Errorf("no realm in Www-Authenticate header from %s", registry)
	}

	req, err := http.NewRequest("GET", realm, nil)
	if err != nil {
		return "", err
	}

	q := req.URL.Query()
	if service, ok := params["service"]; ok {
		q.Set("service", service)
	}
	if scope, ok := params["scope"]; ok {
		q.Set("scope", scope)
	} else {
		q.Set("scope", fmt.Sprintf("repository:%s:pull", repository))
	}
	req.URL.RawQuery = q.Encode()

	tokenResp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesting token from %s: %w", realm, err)
	}
	defer tokenResp.Body.Close()

	if tokenResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token request to %s returned HTTP %d", realm, tokenResp.StatusCode)
	}

	body, err := io.ReadAll(tokenResp.Body)
	if err != nil {
		return "", err
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", fmt.Errorf("parsing token response: %w", err)
	}

	if tr.Token == "" {
		return "", fmt.Errorf("empty token from %s", realm)
	}

	return tr.Token, nil
}
