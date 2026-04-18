package importer

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// sanitize strips runtime-only and cluster-populated fields from an object so
// the resulting YAML is suitable for checking into Git.
//
// A default cleaner runs for every object; kind-specific cleaners run
// afterwards for a handful of kinds whose defaults are cluster-assigned.
func sanitize(obj *unstructured.Unstructured) {
	defaultClean(obj)
	if cleaner, ok := perKindCleaners[obj.GetKind()]; ok {
		cleaner(obj)
	}
}

// defaultClean removes fields that are populated by the API server or are
// only meaningful on a live object.
func defaultClean(obj *unstructured.Unstructured) {
	meta, ok := obj.Object["metadata"].(map[string]interface{})
	if ok {
		for _, k := range []string{
			"creationTimestamp",
			"deletionGracePeriodSeconds",
			"deletionTimestamp",
			"generation",
			"managedFields",
			"ownerReferences",
			"resourceVersion",
			"selfLink",
			"uid",
		} {
			delete(meta, k)
		}

		if ann, annOK := meta["annotations"].(map[string]interface{}); annOK {
			delete(ann, "kubectl.kubernetes.io/last-applied-configuration")
			delete(ann, "deployment.kubernetes.io/revision")
			if len(ann) == 0 {
				delete(meta, "annotations")
			}
		}
		if labels, lOK := meta["labels"].(map[string]interface{}); lOK && len(labels) == 0 {
			delete(meta, "labels")
		}
	}

	delete(obj.Object, "status")
}

var perKindCleaners = map[string]func(*unstructured.Unstructured){
	"Service": func(obj *unstructured.Unstructured) {
		// cluster-populated networking fields
		spec, ok := obj.Object["spec"].(map[string]interface{})
		if !ok {
			return
		}
		for _, k := range []string{
			"clusterIP",
			"clusterIPs",
			"ipFamilies",
			"ipFamilyPolicy",
			"internalTrafficPolicy",
		} {
			delete(spec, k)
		}
	},
	"PersistentVolumeClaim": func(obj *unstructured.Unstructured) {
		spec, ok := obj.Object["spec"].(map[string]interface{})
		if !ok {
			return
		}
		delete(spec, "volumeName")
	},
	"ServiceAccount": func(obj *unstructured.Unstructured) {
		// `secrets` is auto-populated with the default token on older clusters
		delete(obj.Object, "secrets")
	},
	"Deployment":  clearPodTemplateTimestamp,
	"StatefulSet": clearPodTemplateTimestamp,
	"DaemonSet":   clearPodTemplateTimestamp,
	"Job":         clearPodTemplateTimestamp,
	"CronJob": func(obj *unstructured.Unstructured) {
		// CronJob nests another PodTemplate one level deeper.
		unstructured.RemoveNestedField(obj.Object, "spec", "jobTemplate", "spec", "template", "metadata", "creationTimestamp")
	},
}

func clearPodTemplateTimestamp(obj *unstructured.Unstructured) {
	unstructured.RemoveNestedField(obj.Object, "spec", "template", "metadata", "creationTimestamp")
}
