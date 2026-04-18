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

	// Regression lock: when HelmValues is empty we must keep the existing
	// `values: {}` placeholder so unrelated `generate` callers see
	// byte-identical output.
	if !strings.Contains(result, "values: {}") {
		t.Errorf("expected `values: {}` for empty HelmValues, got:\n%s", result)
	}
	if strings.Contains(result, "values:\n") && !strings.Contains(result, "values: {}") {
		t.Error("HelmValues empty should not emit a multi-line values block")
	}
}

func TestRenderHelmFileWithValues(t *testing.T) {
	data := TemplateData{
		App:         "myapp",
		Namespace:   "myapp",
		HelmURL:     "https://charts.example.com",
		HelmChart:   "myapp",
		HelmVersion: "1.2.3",
		HelmValues:  "    replicaCount: 3\n    image:\n      tag: v2",
	}

	result, err := RenderHelmFile(data)
	if err != nil {
		t.Fatal(err)
	}

	if strings.Contains(result, "values: {}") {
		t.Errorf("should not fall back to `values: {}` when HelmValues is non-empty, got:\n%s", result)
	}
	for _, want := range []string{
		"  values:\n",
		"    replicaCount: 3",
		"    image:",
		"      tag: v2",
	} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in:\n%s", want, result)
		}
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

func TestRenderDeployment(t *testing.T) {
	data := TemplateData{App: "myapp", Namespace: "myapp"}
	result, err := RenderDeployment(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"apiVersion: apps/v1", "kind: Deployment", "name: myapp", "replicas: 1"} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in:\n%s", want, result)
		}
	}
	for _, absent := range []string{"serviceName:", "volumeClaimTemplates"} {
		if strings.Contains(result, absent) {
			t.Errorf("Deployment should not contain %q", absent)
		}
	}
}

func TestRenderStatefulSet(t *testing.T) {
	data := TemplateData{App: "myapp", Namespace: "myapp"}
	result, err := RenderStatefulSet(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"kind: StatefulSet", "replicas: 1", "serviceName: myapp", "volumeClaimTemplates"} {
		if !strings.Contains(result, want) {
			t.Errorf("missing %q in:\n%s", want, result)
		}
	}
}

func TestRenderDaemonSet(t *testing.T) {
	data := TemplateData{App: "myapp", Namespace: "myapp"}
	result, err := RenderDaemonSet(data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "kind: DaemonSet") {
		t.Errorf("missing kind: DaemonSet in:\n%s", result)
	}
	if strings.Contains(result, "replicas:") {
		t.Error("DaemonSet should not contain replicas")
	}
	if strings.Contains(result, "serviceName:") {
		t.Error("DaemonSet should not contain serviceName")
	}
}

func TestRenderWorkloadDispatcher(t *testing.T) {
	cases := []struct {
		in       string
		wantKind string
	}{
		{"deployment", "kind: Deployment"},
		{"Deployment", "kind: Deployment"},
		{"STATEFULSET", "kind: StatefulSet"},
		{"DaemonSet", "kind: DaemonSet"},
	}
	for _, tc := range cases {
		got, err := RenderWorkload(tc.in, TemplateData{App: "x", Namespace: "x"})
		if err != nil {
			t.Fatalf("RenderWorkload(%q): %v", tc.in, err)
		}
		if !strings.Contains(got, tc.wantKind) {
			t.Errorf("RenderWorkload(%q) missing %q", tc.in, tc.wantKind)
		}
	}

	if _, err := RenderWorkload("job", TemplateData{}); err == nil {
		t.Error("expected error for unsupported kind")
	}
}

func TestNormalizeWorkloadKind(t *testing.T) {
	cases := map[string]string{
		"deployment":  "Deployment",
		"Deployment":  "Deployment",
		"DEPLOYMENT":  "Deployment",
		"statefulset": "StatefulSet",
		"StatefulSet": "StatefulSet",
		"daemonset":   "DaemonSet",
		"DaemonSet":   "DaemonSet",
		"job":         "",
		"":            "",
	}
	for in, want := range cases {
		if got := NormalizeWorkloadKind(in); got != want {
			t.Errorf("NormalizeWorkloadKind(%q) = %q, want %q", in, got, want)
		}
	}
}
