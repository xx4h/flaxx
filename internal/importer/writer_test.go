package importer

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestMarshalUnstructured_KeyOrder(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"replicas": float64(3),
		},
		"kind":       "Deployment",
		"metadata":   map[string]interface{}{"name": "grafana"},
		"apiVersion": "apps/v1",
	}}

	yaml, err := marshalUnstructured(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Check top-level ordering: apiVersion then kind then metadata then spec.
	idxAPI := strings.Index(yaml, "apiVersion:")
	idxKind := strings.Index(yaml, "kind:")
	idxMeta := strings.Index(yaml, "metadata:")
	idxSpec := strings.Index(yaml, "spec:")

	if idxAPI >= idxKind || idxKind >= idxMeta || idxMeta >= idxSpec {
		t.Errorf("unexpected key order:\n%s", yaml)
	}
}

func TestMarshalUnstructured_NumbersStayNumbers(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"spec": map[string]interface{}{
			"replicas": float64(3),
		},
	}}

	yaml, err := marshalUnstructured(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(yaml, "replicas: 3") {
		t.Errorf("expected `replicas: 3` (no quotes), got:\n%s", yaml)
	}
}

func TestMarshalUnstructured_StartsWithDocumentMarker(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "c"},
	}}

	yaml, err := marshalUnstructured(obj)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.HasPrefix(yaml, "---\n") {
		t.Errorf("marshalled YAML should start with `---`, got: %q", yaml[:10])
	}
}
