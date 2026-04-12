package updater

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
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
// the chart version field.
func UpdateHelmVersion(dir string, version string, dryRun bool) (string, error) {
	files, err := findYAMLFiles(dir)
	if err != nil {
		return "", err
	}

	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("reading %s: %w", filePath, err)
		}

		docs, err := splitYAMLDocuments(data)
		if err != nil {
			return "", fmt.Errorf("parsing %s: %w", filePath, err)
		}

		updated := false
		for _, doc := range docs {
			kind := getScalarValue(doc, "kind")
			if kind != "HelmRelease" {
				continue
			}

			// Navigate: spec.chart.spec.version
			specNode := getMapValue(doc, "spec")
			if specNode == nil {
				continue
			}
			chartNode := getMapValue(specNode, "chart")
			if chartNode == nil {
				continue
			}
			chartSpecNode := getMapValue(chartNode, "spec")
			if chartSpecNode == nil {
				continue
			}

			if setScalarValue(chartSpecNode, "version", "'"+version+"'") {
				updated = true
			} else {
				// Add version field if it doesn't exist
				addMapEntry(chartSpecNode, "version", "'"+version+"'")
				updated = true
			}
		}

		if updated {
			return writeDocuments(filePath, docs, dryRun)
		}
	}

	return "", fmt.Errorf("no HelmRelease found in %s", dir)
}

// UpdateImage finds a Deployment in the namespaces app directory and updates
// the container image. Format: "image:tag" or "name=image:tag" for multi-container pods.
func UpdateImage(dir string, imageSpec string, dryRun bool) (string, error) {
	files, err := findYAMLFiles(dir)
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

		docs, err := splitYAMLDocuments(data)
		if err != nil {
			return "", fmt.Errorf("parsing %s: %w", filePath, err)
		}

		updated := false
		for _, doc := range docs {
			kind := getScalarValue(doc, "kind")
			if kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
				continue
			}

			// Navigate: spec.template.spec.containers
			specNode := getMapValue(doc, "spec")
			if specNode == nil {
				continue
			}
			templateNode := getMapValue(specNode, "template")
			if templateNode == nil {
				continue
			}
			templateSpecNode := getMapValue(templateNode, "spec")
			if templateSpecNode == nil {
				continue
			}
			containersNode := getSequenceValue(templateSpecNode, "containers")
			if containersNode == nil {
				continue
			}

			for _, container := range containersNode.Content {
				if container.Kind != yaml.MappingNode {
					continue
				}
				if containerName != "" {
					name := getScalarValue(container, "name")
					if name != containerName {
						continue
					}
				}
				if setScalarValue(container, "image", newImage) {
					updated = true
					break
				}
			}
		}

		if updated {
			return writeDocuments(filePath, docs, dryRun)
		}
	}

	return "", fmt.Errorf("no matching container found in %s", dir)
}

func splitYAMLDocuments(data []byte) ([]*yaml.Node, error) {
	var docs []*yaml.Node
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var doc yaml.Node
		err := decoder.Decode(&doc)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			return nil, err
		}
		docs = append(docs, &doc)
	}
	return docs, nil
}

func getMapValue(node *yaml.Node, key string) *yaml.Node {
	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func getSequenceValue(node *yaml.Node, key string) *yaml.Node {
	val := getMapValue(node, key)
	if val != nil && val.Kind == yaml.SequenceNode {
		return val
	}
	return nil
}

func getScalarValue(node *yaml.Node, key string) string {
	val := getMapValue(node, key)
	if val != nil && val.Kind == yaml.ScalarNode {
		return val.Value
	}
	return ""
}

func setScalarValue(node *yaml.Node, key string, value string) bool {
	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1].Value = value
			n.Content[i+1].Style = 0
			return true
		}
	}
	return false
}

func addMapEntry(node *yaml.Node, key, value string) {
	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	n.Content = append(n.Content, keyNode, valNode)
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

func writeDocuments(filePath string, docs []*yaml.Node, dryRun bool) (string, error) {
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)

	for _, doc := range docs {
		if err := encoder.Encode(doc); err != nil {
			return "", fmt.Errorf("encoding %s: %w", filePath, err)
		}
	}
	encoder.Close()

	rel, err := filepath.Rel(".", filePath)
	if err != nil {
		rel = filePath
	}

	if dryRun {
		fmt.Printf("--- %s (updated) ---\n%s\n", rel, buf.String())
		return rel, nil
	}

	if err := os.WriteFile(filePath, []byte(buf.String()), 0o644); err != nil {
		return "", fmt.Errorf("writing %s: %w", filePath, err)
	}

	return rel, nil
}
