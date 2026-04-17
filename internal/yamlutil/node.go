// Package yamlutil contains helpers for manipulating YAML documents via
// gopkg.in/yaml.v3 nodes, so comments and unrelated fields survive round-trips.
package yamlutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SplitYAMLDocuments decodes a byte slice into one *yaml.Node per document.
func SplitYAMLDocuments(data []byte) ([]*yaml.Node, error) {
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

// mappingNode unwraps a DocumentNode to its root mapping if needed.
func mappingNode(node *yaml.Node) *yaml.Node {
	n := node
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		return nil
	}
	return n
}

// GetMapValue returns the value node for key in a mapping, or nil if absent.
func GetMapValue(node *yaml.Node, key string) *yaml.Node {
	n := mappingNode(node)
	if n == nil {
		return nil
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// GetSequenceValue returns the value node for key if it is a sequence.
func GetSequenceValue(node *yaml.Node, key string) *yaml.Node {
	val := GetMapValue(node, key)
	if val != nil && val.Kind == yaml.SequenceNode {
		return val
	}
	return nil
}

// GetScalarValue returns the scalar string for key, or "" if absent/non-scalar.
func GetScalarValue(node *yaml.Node, key string) string {
	val := GetMapValue(node, key)
	if val != nil && val.Kind == yaml.ScalarNode {
		return val.Value
	}
	return ""
}

// SetScalarValue updates an existing scalar value. Returns false if the key
// was not present.
func SetScalarValue(node *yaml.Node, key string, value string) bool {
	n := mappingNode(node)
	if n == nil {
		return false
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			n.Content[i+1].Value = value
			n.Content[i+1].Tag = ""
			return true
		}
	}
	return false
}

// AddMapEntry appends key:value (scalar string) to a mapping.
func AddMapEntry(node *yaml.Node, key, value string) {
	n := mappingNode(node)
	if n == nil {
		return
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	n.Content = append(n.Content, keyNode, valNode)
}

// SetOrAddScalar ensures key:value exists as a scalar entry, updating if
// present or appending if missing.
func SetOrAddScalar(node *yaml.Node, key, value string) {
	if !SetScalarValue(node, key, value) {
		AddMapEntry(node, key, value)
	}
}

// AddMapSequence appends key with an empty SequenceNode value.
func AddMapSequence(node *yaml.Node, key string) {
	n := mappingNode(node)
	if n == nil {
		return
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: key}
	valNode := &yaml.Node{Kind: yaml.SequenceNode, Style: yaml.FlowStyle}
	n.Content = append(n.Content, keyNode, valNode)
}

// RemoveMapEntry removes a key and its value from a mapping. Returns true if
// an entry was removed.
func RemoveMapEntry(node *yaml.Node, key string) bool {
	n := mappingNode(node)
	if n == nil {
		return false
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == key {
			n.Content = append(n.Content[:i], n.Content[i+2:]...)
			return true
		}
	}
	return false
}

// RenameMapKey rewrites the scalar key name while preserving the value node
// (so nested structure and comments survive). Returns true if the key was
// found and renamed.
func RenameMapKey(node *yaml.Node, oldKey, newKey string) bool {
	n := mappingNode(node)
	if n == nil {
		return false
	}
	for i := 0; i < len(n.Content)-1; i += 2 {
		if n.Content[i].Value == oldKey {
			n.Content[i].Value = newKey
			return true
		}
	}
	return false
}

// FindYAMLFiles returns every *.yaml / *.yml file directly under dir.
func FindYAMLFiles(dir string) ([]string, error) {
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

// WriteDocuments serializes docs back to filePath, or prints a dry-run preview.
// Returns the path (relative to cwd when possible) that was (or would be) written.
func WriteDocuments(filePath string, docs []*yaml.Node, dryRun bool) (string, error) {
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

	if err := os.WriteFile(filePath, []byte(buf.String()), 0o644); err != nil { //nolint:gosec // YAML config files need to be readable
		return "", fmt.Errorf("writing %s: %w", filePath, err)
	}

	return rel, nil
}
