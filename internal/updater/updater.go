package updater

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xx4h/flaxx/internal/yamlutil"
)

type Options struct {
	App       string
	Cluster   string
	Namespace string

	HelmVersion string
	Image       string

	DryRun bool
}

type Result struct {
	UpdatedFiles []string
}

// UpdateHelmVersion finds the HelmRelease in the cluster app directory and updates
// the chart version field. Deprecated: use UpdateHelmCharts instead.
func UpdateHelmVersion(dir string, version string, dryRun bool) (string, error) {
	results, err := UpdateHelmCharts(dir, map[string]string{"": version}, dryRun)
	if err != nil {
		return "", err
	}
	if len(results) == 0 {
		return "", fmt.Errorf("no HelmRelease found in %s", dir)
	}
	return results[0], nil
}

// UpdateHelmCharts updates the chart version for one or more HelmReleases.
// The updates map is keyed by chart name → version. An empty key "" matches
// any single HelmRelease (for backwards compatibility with --helm-version).
func UpdateHelmCharts(dir string, updates map[string]string, dryRun bool) ([]string, error) {
	files, err := yamlutil.FindYAMLFiles(dir)
	if err != nil {
		return nil, err
	}

	_, matchAny := updates[""]
	if matchAny {
		count, countErr := countHelmReleases(files)
		if countErr != nil {
			return nil, countErr
		}
		if count > 1 {
			return nil, fmt.Errorf("multiple HelmReleases found; use --helm chart:version to specify which to update")
		}
	}

	var updatedFiles []string
	matched := make(map[string]bool)

	for _, filePath := range files {
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, readErr)
		}

		docs, parseErr := yamlutil.SplitYAMLDocuments(data)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", filePath, parseErr)
		}

		fileUpdated := false
		for _, doc := range docs {
			kind := yamlutil.GetScalarValue(doc, "kind")
			if kind != "HelmRelease" {
				continue
			}

			specNode := yamlutil.GetMapValue(doc, "spec")
			if specNode == nil {
				continue
			}
			chartNode := yamlutil.GetMapValue(specNode, "chart")
			if chartNode == nil {
				continue
			}
			chartSpecNode := yamlutil.GetMapValue(chartNode, "spec")
			if chartSpecNode == nil {
				continue
			}

			chartName := yamlutil.GetScalarValue(chartSpecNode, "chart")

			version, ok := updates[chartName]
			if !ok && matchAny {
				version = updates[""]
				ok = true
			}
			if !ok {
				continue
			}

			yamlutil.SetOrAddScalar(chartSpecNode, "version", version)
			fileUpdated = true
			matched[chartName] = true
		}

		if fileUpdated {
			file, writeErr := yamlutil.WriteDocuments(filePath, docs, dryRun)
			if writeErr != nil {
				return nil, writeErr
			}
			updatedFiles = append(updatedFiles, file)
		}
	}

	if len(updatedFiles) == 0 {
		return nil, fmt.Errorf("no matching HelmRelease found in %s", dir)
	}

	return updatedFiles, nil
}

func countHelmReleases(files []string) (int, error) {
	count := 0
	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return 0, fmt.Errorf("reading %s: %w", filePath, err)
		}
		docs, err := yamlutil.SplitYAMLDocuments(data)
		if err != nil {
			return 0, fmt.Errorf("parsing %s: %w", filePath, err)
		}
		for _, doc := range docs {
			if yamlutil.GetScalarValue(doc, "kind") == "HelmRelease" {
				count++
			}
		}
	}
	return count, nil
}

// UpdateImage finds a Deployment/StatefulSet/DaemonSet in the namespaces app
// directory and updates the container image. Format: "image:tag" or
// "name=image:tag" for multi-container pods.
func UpdateImage(dir string, imageSpec string, dryRun bool) (string, error) {
	files, err := yamlutil.FindYAMLFiles(dir)
	if err != nil {
		return "", err
	}

	containerName := ""
	newImage := imageSpec
	if parts := strings.SplitN(imageSpec, "=", 2); len(parts) == 2 {
		containerName = parts[0]
		newImage = parts[1]
	}

	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", filePath, err)
		}

		docs, err := yamlutil.SplitYAMLDocuments(data)
		if err != nil {
			return "", fmt.Errorf("parsing %s: %w", filePath, err)
		}

		updated := false
		for _, doc := range docs {
			kind := yamlutil.GetScalarValue(doc, "kind")
			if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
				continue
			}

			specNode := yamlutil.GetMapValue(doc, "spec")
			if specNode == nil {
				continue
			}
			templateNode := yamlutil.GetMapValue(specNode, "template")
			if templateNode == nil {
				continue
			}
			templateSpecNode := yamlutil.GetMapValue(templateNode, "spec")
			if templateSpecNode == nil {
				continue
			}
			containersNode := yamlutil.GetSequenceValue(templateSpecNode, "containers")
			if containersNode == nil {
				continue
			}

			for _, container := range containersNode.Content {
				if container.Kind != yaml.MappingNode {
					continue
				}
				if containerName != "" {
					name := yamlutil.GetScalarValue(container, "name")
					if name != containerName {
						continue
					}
				}
				if yamlutil.SetScalarValue(container, "image", newImage) {
					updated = true
					break
				}
			}
		}

		if updated {
			return yamlutil.WriteDocuments(filePath, docs, dryRun)
		}
	}

	return "", fmt.Errorf("no matching container found in %s", dir)
}
