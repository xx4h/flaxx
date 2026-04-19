package importer

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// skippedKinds lists GVKs that never belong in a Git-managed namespace dir.
// Keys are "<kind>.<group>" ("" group for core v1). We match on kind+group
// rather than GVR so the list survives API version bumps.
var skippedKinds = map[string]string{
	"Event.":                         "ephemeral runtime event",
	"Event.events.k8s.io":            "ephemeral runtime event",
	"Endpoints.":                     "managed by the Service controller",
	"EndpointSlice.discovery.k8s.io": "managed by the Service controller",
	"Lease.coordination.k8s.io":      "leader-election state",
	"Pod.":                           "workload-controller owned; the controller is adopted instead",
	"PodMetrics.metrics.k8s.io":      "runtime metric, not configuration",
	"NodeMetrics.metrics.k8s.io":     "runtime metric, not configuration",
	"ControllerRevision.apps":        "internal rollout state",
	"ReplicaSet.apps":                "owned by Deployment",
}

// shouldSkipGVK decides whether to skip an entire kind. It does not consider
// individual objects (that's shouldSkipObject's job).
func shouldSkipGVK(gvk schema.GroupVersionKind, includeSecrets bool) (string, bool) {
	key := gvk.Kind + "." + gvk.Group
	if reason, ok := skippedKinds[key]; ok {
		return reason, true
	}

	if gvk.Kind == "Secret" && gvk.Group == "" && !includeSecrets {
		return "secrets are opt-in via --include-secrets", true
	}

	return "", false
}

// shouldSkipObject decides whether an individual object should be dropped.
// Currently this filters anything owned by another resource (ReplicaSets
// created by Deployments, Pods created by ReplicaSets, generated Jobs, …)
// plus a few special cases — Helm release secrets, default ServiceAccount
// tokens, etc.
func shouldSkipObject(obj *unstructured.Unstructured, gvk schema.GroupVersionKind, _ bool) (string, bool) {
	if len(obj.GetOwnerReferences()) > 0 {
		return "owned by " + obj.GetOwnerReferences()[0].Kind, true
	}

	// Helm manages its own release secrets; adopting them would cause Flux
	// to recreate them on the next reconcile.
	if gvk.Kind == "Secret" && gvk.Group == "" {
		if strings.HasPrefix(obj.GetName(), "sh.helm.release.v1.") {
			return "Helm release-state secret", true
		}
		// Auto-mounted service-account tokens created by older clusters.
		if t, ok, _ := unstructured.NestedString(obj.Object, "type"); ok && t == "kubernetes.io/service-account-token" {
			return "auto-populated service-account token", true
		}
	}

	// Default objects every namespace ships with. They exist on every
	// cluster whether we adopt them or not; writing them into Git adds
	// noise without benefit.
	if gvk.Kind == "ServiceAccount" && gvk.Group == "" && obj.GetName() == "default" {
		return "built-in default ServiceAccount", true
	}
	if gvk.Kind == "ConfigMap" && gvk.Group == "" && obj.GetName() == "kube-root-ca.crt" {
		return "cluster-managed CA bundle", true
	}

	return "", false
}
