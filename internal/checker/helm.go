package checker

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xx4h/flaxx/internal/cache"
)

// repoIndex represents a Helm repository index.yaml.
type repoIndex struct {
	Entries map[string][]chartEntry `yaml:"entries"`
}

type chartEntry struct {
	Version string `yaml:"version"`
}

// FetchHelmVersions queries a Helm repository and returns available versions
// for the given chart, sorted newest first.
//
// Results are cached via the package-level cache (see SetCache). Cache hits
// skip the HTTP round-trip and the full index.yaml parse.
func FetchHelmVersions(repoURL, chartName string) ([]TaggedVersion, error) {
	key := cache.Key(cachePrefixHelm, repoURL, chartName)

	var rawTags []string
	hit, _ := activeCache.Get(key, &rawTags)
	if hit {
		return ParseTaggedVersions(rawTags), nil
	}

	rawTags, err := fetchHelmRawVersions(repoURL, chartName)
	if err != nil {
		return nil, err
	}

	_ = activeCache.Set(key, rawTags)

	return ParseTaggedVersions(rawTags), nil
}

// fetchHelmRawVersions performs the live HTTP fetch and returns the raw,
// unparsed version strings from the repository index.
func fetchHelmRawVersions(repoURL, chartName string) ([]string, error) {
	indexURL := strings.TrimRight(repoURL, "/") + "/index.yaml"

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(indexURL)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", indexURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: HTTP %d", indexURL, resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", indexURL, err)
	}

	var idx repoIndex
	if err := yaml.Unmarshal(body, &idx); err != nil {
		return nil, fmt.Errorf("parsing index from %s: %w", indexURL, err)
	}

	entries, ok := idx.Entries[chartName]
	if !ok {
		available := make([]string, 0, len(idx.Entries))
		for name := range idx.Entries {
			available = append(available, name)
		}
		return nil, fmt.Errorf("chart %q not found in repository; available charts: %s",
			chartName, strings.Join(available, ", "))
	}

	rawTags := make([]string, 0, len(entries))
	for _, entry := range entries {
		rawTags = append(rawTags, entry.Version)
	}
	return rawTags, nil
}

// CheckHelm fetches available versions and compares against the current version.
// Supports both standard Helm repositories and OCI registries.
// The filter mode controls which version channels are shown.
func CheckHelm(info *HelmInfo, mode FilterMode) (*CheckResult, error) {
	var versions []TaggedVersion
	var err error

	if info.RepoType == "oci" {
		versions, err = FetchOCIVersions(info.RepoURL, info.ChartName)
	} else {
		versions, err = FetchHelmVersions(info.RepoURL, info.ChartName)
	}
	if err != nil {
		return nil, err
	}

	result := &CheckResult{
		HelmInfo: *info,
	}

	// Find versions newer than current
	if info.CurrentVersion != "" {
		current := ParseVersion(info.CurrentVersion)
		if current != nil {
			filtered := FilterTaggedVersions(versions, current, mode)
			for _, tv := range filtered {
				if tv.Version.GreaterThan(current) {
					result.AvailableUpdates = append(result.AvailableUpdates, tv.Tag)
				}
			}
			if len(filtered) > 0 {
				result.LatestVersion = filtered[0].Tag
			}
		}
	} else {
		// No current version pinned — filter and show latest versions
		filtered := FilterTaggedVersions(versions, nil, mode)
		if len(filtered) > 0 {
			result.LatestVersion = filtered[0].Tag
		}
		limit := 10
		if len(filtered) < limit {
			limit = len(filtered)
		}
		for _, tv := range filtered[:limit] {
			result.AvailableUpdates = append(result.AvailableUpdates, tv.Tag)
		}
	}

	return result, nil
}
