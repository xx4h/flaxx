package checker

import (
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
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
func FetchHelmVersions(repoURL, chartName string) ([]*semver.Version, error) {
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

	var versions []*semver.Version
	for _, entry := range entries {
		v, err := semver.NewVersion(entry.Version)
		if err != nil {
			continue // skip non-semver versions
		}
		versions = append(versions, v)
	}

	sort.Sort(sort.Reverse(semver.Collection(versions)))

	return versions, nil
}

// CheckHelm fetches available versions and compares against the current version.
// Supports both standard Helm repositories and OCI registries.
func CheckHelm(info *HelmInfo) (*CheckResult, error) {
	var versions []*semver.Version
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

	if len(versions) > 0 {
		result.LatestVersion = versions[0].Original()
	}

	// Find versions newer than current
	if info.CurrentVersion != "" {
		current, err := semver.NewVersion(info.CurrentVersion)
		if err == nil {
			for _, v := range versions {
				if v.GreaterThan(current) {
					result.AvailableUpdates = append(result.AvailableUpdates, v.Original())
				}
			}
		}
	} else {
		// No current version pinned — show latest versions
		limit := 10
		if len(versions) < limit {
			limit = len(versions)
		}
		for _, v := range versions[:limit] {
			result.AvailableUpdates = append(result.AvailableUpdates, v.Original())
		}
	}

	return result, nil
}
