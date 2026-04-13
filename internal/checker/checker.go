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
	ChartName      string
	CurrentVersion string
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

// ScanApp reads the cluster directory for an app and extracts Helm chart info
// by finding the HelmRelease and HelmRepository resources.
// If appFilter is non-empty, only files with that prefix are scanned (for flat layouts).
func ScanApp(clusterDir string, appFilter string) (*HelmInfo, error) {
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

	var info HelmInfo

	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, err)
		}

		resources, err := parseYAMLDocuments(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", filePath, err)
		}

		for _, res := range resources {
			switch res.Kind {
			case "HelmRelease":
				info.ChartName = res.Spec.Chart.Spec.Chart
				info.CurrentVersion = strings.Trim(res.Spec.Chart.Spec.Version, "'\"")
				info.Namespace = res.Metadata.Namespace
			case "HelmRepository":
				info.RepoURL = res.Spec.URL
				info.RepoType = res.Spec.Type
			}
		}
	}

	if info.RepoURL == "" {
		return nil, fmt.Errorf("no HelmRepository found in %s", clusterDir)
	}
	if info.ChartName == "" {
		return nil, fmt.Errorf("no HelmRelease found in %s", clusterDir)
	}

	return &info, nil
}

// ScanCluster scans all app subdirectories in a cluster directory and returns
// HelmInfo for each app that has a HelmRelease and HelmRepository.
// Apps without Helm resources are silently skipped.
func ScanCluster(clusterDir string) ([]*HelmInfo, error) {
	entries, err := os.ReadDir(clusterDir)
	if err != nil {
		return nil, fmt.Errorf("reading cluster directory %s: %w", clusterDir, err)
	}

	var results []*HelmInfo
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		appDir := filepath.Join(clusterDir, e.Name())
		info, err := ScanApp(appDir, "")
		if err != nil {
			// Skip apps without Helm resources
			continue
		}
		info.App = e.Name()
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
				Chart   string `yaml:"chart"`
				Version string `yaml:"version"`
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
