package importer

import (
	"encoding/json"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// marshalUnstructured converts an unstructured.Unstructured to YAML with a
// stable top-level key order (apiVersion, kind, metadata, spec, data, …).
//
// yaml.v3 emits keys in insertion order, but the JSON intermediate we go
// through loses the original ordering. We rebuild the document so the file
// reads the way a human would write it.
func marshalUnstructured(obj *unstructured.Unstructured) (string, error) {
	// JSON round-trip first: ensures numeric/bool values keep the right
	// YAML tag instead of being emitted as strings.
	raw, err := json.Marshal(obj.Object)
	if err != nil {
		return "", err
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(raw, &generic); err != nil {
		return "", err
	}

	root := buildOrderedNode(generic)

	var buf strings.Builder
	buf.WriteString("---\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// preferredTopLevelOrder is the Kubernetes convention: apiVersion first,
// kind second, metadata third, spec / data / rules / subjects / roleRef
// fourth, then everything else alphabetical.
var preferredTopLevelOrder = []string{
	"apiVersion",
	"kind",
	"metadata",
	"spec",
	"data",
	"stringData",
	"rules",
	"subjects",
	"roleRef",
	"type",
	"binaryData",
	"immutable",
	"automountServiceAccountToken",
}

func buildOrderedNode(m map[string]interface{}) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	added := make(map[string]bool)

	for _, key := range preferredTopLevelOrder {
		if v, ok := m[key]; ok {
			node.Content = append(node.Content, scalarNode(key), toYAMLNode(v))
			added[key] = true
		}
	}
	// Remaining keys in alphabetical order for reproducibility.
	remaining := make([]string, 0, len(m))
	for k := range m {
		if !added[k] {
			remaining = append(remaining, k)
		}
	}
	sortStrings(remaining)
	for _, k := range remaining {
		node.Content = append(node.Content, scalarNode(k), toYAMLNode(m[k]))
	}
	return node
}

func toYAMLNode(v interface{}) *yaml.Node {
	switch val := v.(type) {
	case map[string]interface{}:
		// Nested maps: alphabetical order — simpler and predictable.
		node := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sortStrings(keys)
		for _, k := range keys {
			node.Content = append(node.Content, scalarNode(k), toYAMLNode(val[k]))
		}
		return node
	case []interface{}:
		node := &yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range val {
			node.Content = append(node.Content, toYAMLNode(item))
		}
		return node
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case bool:
		if val {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"}
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"}
	case float64:
		// JSON numbers are always float64; emit as int when exact.
		if val == float64(int64(val)) {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", int64(val))}
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: fmt.Sprintf("%v", val)}
	case string:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: val}
	default:
		return &yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%v", val)}
	}
}

func scalarNode(v string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
}

// sortStrings is stdlib sort.Strings under a local name so this file has
// exactly one import from the stdlib that actually does work at runtime.
// (Keeps the import list tight.)
func sortStrings(s []string) {
	// Simple insertion sort; slices here are always tiny (object keys).
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
