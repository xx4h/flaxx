package importer

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestShouldSkipGVK(t *testing.T) {
	tests := []struct {
		name           string
		gvk            schema.GroupVersionKind
		includeSecrets bool
		wantSkip       bool
	}{
		{"Pod is skipped", schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Pod"}, false, true},
		{"Event is skipped", schema.GroupVersionKind{Group: "events.k8s.io", Version: "v1", Kind: "Event"}, false, true},
		{"ReplicaSet is skipped", schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "ReplicaSet"}, false, true},
		{"EndpointSlice is skipped", schema.GroupVersionKind{Group: "discovery.k8s.io", Version: "v1", Kind: "EndpointSlice"}, false, true},
		{"Deployment is kept", schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, false, false},
		{"Service is kept", schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Service"}, false, false},
		{"ConfigMap is kept", schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, false, false},
		{"Secret skipped by default", schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, false, true},
		{"Secret kept with flag", schema.GroupVersionKind{Group: "", Version: "v1", Kind: "Secret"}, true, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, skip := shouldSkipGVK(tc.gvk, tc.includeSecrets)
			if skip != tc.wantSkip {
				t.Errorf("shouldSkipGVK(%s) skip=%v, want %v", tc.gvk, skip, tc.wantSkip)
			}
		})
	}
}

func TestShouldSkipObject_OwnedObjectsSkipped(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "cm"},
	}}
	obj.SetOwnerReferences([]metav1.OwnerReference{{Kind: "Deployment", Name: "owner"}})

	reason, skip := shouldSkipObject(obj, schema.GroupVersionKind{Kind: "ConfigMap"}, false)
	if !skip {
		t.Fatal("object with ownerReference should be skipped")
	}
	if reason == "" {
		t.Error("skip reason should not be empty")
	}
}

func TestShouldSkipObject_HelmReleaseSecretSkipped(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "sh.helm.release.v1.grafana.v1"},
		"type":       "helm.sh/release.v1",
	}}
	_, skip := shouldSkipObject(obj, schema.GroupVersionKind{Kind: "Secret", Group: ""}, true)
	if !skip {
		t.Fatal("Helm release secret should be skipped even with --include-secrets")
	}
}

func TestShouldSkipObject_SATokenSkipped(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]interface{}{"name": "default-token-abc"},
		"type":       "kubernetes.io/service-account-token",
	}}
	_, skip := shouldSkipObject(obj, schema.GroupVersionKind{Kind: "Secret", Group: ""}, true)
	if !skip {
		t.Fatal("service-account-token secret should be skipped")
	}
}

func TestShouldSkipObject_DefaultSAskipped(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata":   map[string]interface{}{"name": "default"},
	}}
	_, skip := shouldSkipObject(obj, schema.GroupVersionKind{Kind: "ServiceAccount", Group: ""}, false)
	if !skip {
		t.Fatal("default ServiceAccount should be skipped")
	}
}

func TestShouldSkipObject_KubeRootCAMapSkipped(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "kube-root-ca.crt"},
	}}
	_, skip := shouldSkipObject(obj, schema.GroupVersionKind{Kind: "ConfigMap", Group: ""}, false)
	if !skip {
		t.Fatal("kube-root-ca.crt ConfigMap should be skipped")
	}
}

func TestShouldSkipObject_UserConfigMapKept(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]interface{}{"name": "my-app-config"},
	}}
	_, skip := shouldSkipObject(obj, schema.GroupVersionKind{Kind: "ConfigMap", Group: ""}, false)
	if skip {
		t.Fatal("user-created ConfigMap should NOT be skipped")
	}
}
