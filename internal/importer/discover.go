package importer

import (
	"context"
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// discoverResources enumerates every user-facing namespaced resource in the
// given namespace and returns the kept objects plus a list of skip reasons
// (for optional display to the user).
func discoverResources(ctx context.Context, restCfg *rest.Config, namespace string, includeSecrets bool) ([]*unstructured.Unstructured, []string, error) {
	disc, err := discovery.NewDiscoveryClientForConfig(restCfg)
	if err != nil {
		return nil, nil, err
	}
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		return nil, nil, err
	}

	apiLists, err := disc.ServerPreferredNamespacedResources()
	if err != nil {
		// Partial results are common (e.g. aggregated API servers down);
		// carry on with whatever the server returned.
		if len(apiLists) == 0 {
			return nil, nil, err
		}
	}

	var (
		kept    []*unstructured.Unstructured
		skipped []string
	)

	// Deterministic iteration — both for reproducible output and so tests
	// can assert on skip ordering.
	sort.SliceStable(apiLists, func(i, j int) bool {
		return apiLists[i].GroupVersion < apiLists[j].GroupVersion
	})

	for _, list := range apiLists {
		gv, parseErr := schema.ParseGroupVersion(list.GroupVersion)
		if parseErr != nil {
			continue
		}
		for _, res := range list.APIResources {
			if !supportsList(res.Verbs) {
				continue
			}
			if strings.Contains(res.Name, "/") {
				// subresources like pods/status, deployments/scale
				continue
			}
			gvk := gv.WithKind(res.Kind)
			gvr := gv.WithResource(res.Name)

			if reason, skip := shouldSkipGVK(gvk, includeSecrets); skip {
				skipped = append(skipped, fmt.Sprintf("%s: %s", gvk, reason))
				continue
			}

			objList, err := dyn.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
			if err != nil {
				// Many clusters expose resources we lack RBAC for; skip
				// rather than abort so a best-effort import still works.
				skipped = append(skipped, fmt.Sprintf("%s: list failed (%v)", gvk, err))
				continue
			}

			for i := range objList.Items {
				obj := &objList.Items[i]
				if reason, skip := shouldSkipObject(obj, gvk, includeSecrets); skip {
					skipped = append(skipped, fmt.Sprintf("%s/%s: %s", gvk, obj.GetName(), reason))
					continue
				}
				// Stamp the GVK onto the object — dynamic.List sometimes
				// elides it, and our YAML marshal depends on it.
				obj.GetObjectKind().SetGroupVersionKind(gvk)
				kept = append(kept, obj)
			}
		}
	}

	// Deterministic ordering for reproducible output.
	sort.SliceStable(kept, func(i, j int) bool {
		ki := kept[i].GetKind() + "/" + kept[i].GetName()
		kj := kept[j].GetKind() + "/" + kept[j].GetName()
		return ki < kj
	})

	return kept, skipped, nil
}

// supportsList returns true when the resource exposes the "list" verb.
func supportsList(verbs metav1.Verbs) bool {
	for _, v := range verbs {
		if v == "list" {
			return true
		}
	}
	return false
}
