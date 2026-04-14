package checker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// HelmInfo holds the extracted Helm chart information from an app's files.
type HelmInfo struct {
	App            string
	Name           string // metadata.name of the HelmRelease resource
	ChartName      string
	CurrentVersion string
	RepoName       string // name of the HelmRepository resource
	RepoURL        string
	RepoType       string // "" for standard, "oci" for OCI
	Namespace      string
}

// CheckResult holds the result of a version check for a single app.
type CheckResult struct {
	HelmInfo
	LatestVersion    string
	AvailableUpdates []string
}

// helmRelease holds parsed HelmRelease data for matching to repositories.
type helmRelease struct {
	Name           string // metadata.name
	ChartName      string
	CurrentVersion string
	Namespace      string
	SourceRefName  string
}

// helmRepository holds parsed HelmRepository data.
type helmRepository struct {
	Name     string
	URL      string
	RepoType string
}

// ScanAllHelm reads the cluster directory for an app and extracts all Helm chart
// info by matching HelmRelease resources to their HelmRepository via sourceRef.
// If appFilter is non-empty, only files with that prefix are scanned (for flat layouts).
func ScanAllHelm(clusterDir string, appFilter string) ([]HelmInfo, error) {
	files, err := findYAMLFiles(clusterDir)
	if err != nil {
		return nil, err
	}

	if appFilter != "" {
		var filtered []string
		for _, f := range files {
			base := filepath.Base(f)
			if strings.HasPrefix(base, appFilter+"-") || strings.HasPrefix(base, appFilter+".") {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	var releases []helmRelease
	repos := make(map[string]helmRepository) // keyed by metadata.name

	for _, filePath := range files {
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, readErr)
		}

		resources, parseErr := parseYAMLDocuments(data)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", filePath, parseErr)
		}

		for _, res := range resources {
			switch res.Kind {
			case "HelmRelease":
				releases = append(releases, helmRelease{
					Name:           res.Metadata.Name,
					ChartName:      res.Spec.Chart.Spec.Chart,
					CurrentVersion: strings.Trim(res.Spec.Chart.Spec.Version, "'\""),
					Namespace:      res.Metadata.Namespace,
					SourceRefName:  res.Spec.Chart.Spec.SourceRef.Name,
				})
			case "HelmRepository":
				repos[res.Metadata.Name] = helmRepository{
					Name:     res.Metadata.Name,
					URL:      res.Spec.URL,
					RepoType: res.Spec.Type,
				}
			}
		}
	}

	if len(releases) == 0 {
		return nil, nil
	}

	var results []HelmInfo
	for _, rel := range releases {
		info := HelmInfo{
			Name:           rel.Name,
			ChartName:      rel.ChartName,
			CurrentVersion: rel.CurrentVersion,
			Namespace:      rel.Namespace,
			RepoName:       rel.SourceRefName,
		}
		// Match to repository by sourceRef name
		if repo, ok := repos[rel.SourceRefName]; ok {
			info.RepoURL = repo.URL
			info.RepoType = repo.RepoType
		}
		results = append(results, info)
	}

	return results, nil
}

// resource is a generic struct for parsing Kubernetes YAML resources.
type resource struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Name      string `yaml:"name"`
		Namespace string `yaml:"namespace"`
	} `yaml:"metadata"`
	Spec struct {
		// HelmRelease fields
		Chart struct {
			Spec struct {
				Chart     string `yaml:"chart"`
				Version   string `yaml:"version"`
				SourceRef struct {
					Kind string `yaml:"kind"`
					Name string `yaml:"name"`
				} `yaml:"sourceRef"`
			} `yaml:"spec"`
		} `yaml:"chart"`
		// HelmRepository fields
		URL  string `yaml:"url"`
		Type string `yaml:"type"`
	} `yaml:"spec"`
}

func parseYAMLDocuments(data []byte) ([]resource, error) {
	var resources []resource
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var res resource
		err := decoder.Decode(&res)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if res.Kind != "" {
			resources = append(resources, res)
		}
	}
	return resources, nil
}

func findYAMLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	return files, nil
}
