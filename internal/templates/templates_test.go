package templates

import (
	"strings"
	"testing"
)

func TestRenderNamespace(t *testing.T) {
	data := TemplateData{
		App:       "myapp",
		Cluster:   "k8s",
		Namespace: "myapp",
	}

	result, err := RenderNamespace(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "name: myapp") {
		t.Error("expected namespace name myapp")
	}
	if !strings.Contains(result, "kustomize.toolkit.fluxcd.io/prune: disabled") {
		t.Error("expected prune disabled annotation")
	}
}

func TestRenderNsKustomization(t *testing.T) {
	result, err := RenderNsKustomization([]string{"serviceaccount.yaml", "vso-config.yaml"})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "- namespace.yaml") {
		t.Error("expected namespace.yaml in resources")
	}
	if !strings.Contains(result, "- serviceaccount.yaml") {
		t.Error("expected serviceaccount.yaml in resources")
	}
	if !strings.Contains(result, "- vso-config.yaml") {
		t.Error("expected vso-config.yaml in resources")
	}
}

func TestRenderNsKustomizationNoExtras(t *testing.T) {
	result, err := RenderNsKustomization(nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "- namespace.yaml") {
		t.Error("expected namespace.yaml")
	}
	if strings.Contains(result, "serviceaccount") {
		t.Error("should not contain extras")
	}
}

func TestRenderFluxKustomizationSingle(t *testing.T) {
	data := KustomizationData{
		App:            "myapp",
		Namespace:      "myapp",
		Interval:       "2m",
		Timeout:        "1m",
		Prune:          "false",
		NamespacesPath: "clusters/k8s-namespaces/myapp",
	}

	result, err := RenderFluxKustomization(data, false)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "name: myapp") {
		t.Error("expected app name")
	}
	if !strings.Contains(result, "name: flux-system") {
		t.Error("expected flux-system source ref")
	}
	if strings.Contains(result, "myapp-app") {
		t.Error("single kustomization should not have dual entry")
	}
}

func TestRenderFluxKustomizationDual(t *testing.T) {
	data := KustomizationData{
		App:            "myapp",
		Namespace:      "myapp",
		Interval:       "2m",
		Timeout:        "1m",
		Prune:          "false",
		NamespacesPath: "clusters/k8s-namespaces/myapp",
		GitPath:        "./deploy/production",
		GitName:        "myapp-server",
	}

	result, err := RenderFluxKustomization(data, true)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "name: myapp-app") {
		t.Error("expected dual kustomization with -app suffix")
	}
	if !strings.Contains(result, "name: myapp-server") {
		t.Error("expected git repo source ref")
	}
}

func TestRenderGitRepository(t *testing.T) {
	data := TemplateData{
		Namespace: "myapp",
		GitName:   "myapp-server",
		GitBranch: "main",
		GitSecret: "git-repo-secret",
		GitURL:    "https://git.example.com/org/myapp-server.git",
	}

	result, err := RenderGitRepository(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "name: myapp-server") {
		t.Error("expected git repo name")
	}
	if !strings.Contains(result, "branch: main") {
		t.Error("expected branch")
	}
	if !strings.Contains(result, "url: https://git.example.com/org/myapp-server.git") {
		t.Error("expected url")
	}
}

func TestRenderHelmFile(t *testing.T) {
	data := TemplateData{
		App:         "myapp",
		Namespace:   "myapp",
		HelmURL:     "https://charts.example.com",
		HelmChart:   "myapp",
		HelmVersion: "1.2.3",
		HelmOCI:     false,
	}

	result, err := RenderHelmFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "kind: HelmRepository") {
		t.Error("expected HelmRepository")
	}
	if !strings.Contains(result, "kind: HelmRelease") {
		t.Error("expected HelmRelease")
	}
	if !strings.Contains(result, "version: '1.2.3'") {
		t.Error("expected version")
	}
	if strings.Contains(result, "type: oci") {
		t.Error("should not have OCI type")
	}
}

func TestRenderHelmFileOCI(t *testing.T) {
	data := TemplateData{
		App:       "myapp",
		Namespace: "myapp",
		HelmURL:   "oci://ghcr.io/example/charts",
		HelmChart: "myapp",
		HelmOCI:   true,
	}

	result, err := RenderHelmFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(result, "type: oci") {
		t.Error("expected OCI type")
	}
	if strings.Contains(result, "version:") {
		t.Error("should not have version when not set")
	}
}
