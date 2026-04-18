package importer

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSanitizeStripsDefaultFields(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]interface{}{
			"name":              "grafana",
			"namespace":         "monitoring",
			"resourceVersion":   "1234",
			"uid":               "abc-def",
			"generation":        float64(3),
			"creationTimestamp": "2026-04-18T09:00:00Z",
			"managedFields":     []interface{}{map[string]interface{}{"manager": "kubectl"}},
			"selfLink":          "/apis/apps/v1/...",
			"annotations": map[string]interface{}{
				"kubectl.kubernetes.io/last-applied-configuration": "{...}",
				"deployment.kubernetes.io/revision":                "4",
				"custom":                                           "keep-me",
			},
		},
		"spec": map[string]interface{}{
			"replicas": float64(2),
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"creationTimestamp": nil,
				},
			},
		},
		"status": map[string]interface{}{"availableReplicas": float64(2)},
	}}

	sanitize(obj)

	if _, ok := obj.Object["status"]; ok {
		t.Error("status not stripped")
	}

	meta := obj.Object["metadata"].(map[string]interface{})
	for _, k := range []string{"resourceVersion", "uid", "generation", "creationTimestamp", "managedFields", "selfLink"} {
		if _, ok := meta[k]; ok {
			t.Errorf("metadata.%s not stripped", k)
		}
	}

	ann := meta["annotations"].(map[string]interface{})
	if _, ok := ann["kubectl.kubernetes.io/last-applied-configuration"]; ok {
		t.Error("last-applied-configuration annotation not stripped")
	}
	if _, ok := ann["deployment.kubernetes.io/revision"]; ok {
		t.Error("deployment revision annotation not stripped")
	}
	if ann["custom"] != "keep-me" {
		t.Error("unrelated annotation was removed")
	}

	spec := obj.Object["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	tplMeta := template["metadata"].(map[string]interface{})
	if _, ok := tplMeta["creationTimestamp"]; ok {
		t.Error("pod template creationTimestamp not stripped")
	}
}

func TestSanitizeRemovesEmptyAnnotations(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "x",
			"annotations": map[string]interface{}{
				"kubectl.kubernetes.io/last-applied-configuration": "{...}",
			},
		},
	}}

	sanitize(obj)

	meta := obj.Object["metadata"].(map[string]interface{})
	if _, ok := meta["annotations"]; ok {
		t.Error("empty annotations map should be removed entirely")
	}
}

func TestSanitizeServiceStripsClusterFields(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata":   map[string]interface{}{"name": "svc", "namespace": "ns"},
		"spec": map[string]interface{}{
			"clusterIP":             "10.96.0.1",
			"clusterIPs":            []interface{}{"10.96.0.1"},
			"ipFamilies":            []interface{}{"IPv4"},
			"ipFamilyPolicy":        "SingleStack",
			"internalTrafficPolicy": "Cluster",
			"type":                  "ClusterIP",
			"ports": []interface{}{map[string]interface{}{
				"port":       float64(80),
				"targetPort": float64(8080),
			}},
		},
	}}

	sanitize(obj)

	spec := obj.Object["spec"].(map[string]interface{})
	for _, k := range []string{"clusterIP", "clusterIPs", "ipFamilies", "ipFamilyPolicy", "internalTrafficPolicy"} {
		if _, ok := spec[k]; ok {
			t.Errorf("Service.spec.%s not stripped", k)
		}
	}
	if spec["type"] != "ClusterIP" {
		t.Error("non-defaulted fields should be preserved")
	}
	if _, ok := spec["ports"]; !ok {
		t.Error("spec.ports should be preserved")
	}
}

func TestSanitizePVCStripsVolumeName(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata":   map[string]interface{}{"name": "pvc"},
		"spec": map[string]interface{}{
			"volumeName":       "pv-abc-123",
			"accessModes":      []interface{}{"ReadWriteOnce"},
			"storageClassName": "gp3",
		},
	}}

	sanitize(obj)

	spec := obj.Object["spec"].(map[string]interface{})
	if _, ok := spec["volumeName"]; ok {
		t.Error("volumeName not stripped")
	}
	if spec["storageClassName"] != "gp3" {
		t.Error("storageClassName should be preserved")
	}
}

func TestSanitizeServiceAccountStripsSecrets(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata":   map[string]interface{}{"name": "sa"},
		"secrets":    []interface{}{map[string]interface{}{"name": "sa-token-xyz"}},
	}}

	sanitize(obj)

	if _, ok := obj.Object["secrets"]; ok {
		t.Error("ServiceAccount.secrets not stripped")
	}
}
