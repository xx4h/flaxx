package templates

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

type TemplateData struct {
	App       string
	Cluster   string
	Namespace string
	Interval  string
	Timeout   string
	Prune     string
	// Git-specific
	GitURL    string
	GitBranch string
	GitPath   string
	GitSecret string
	GitName   string
	// Helm-specific
	HelmURL     string
	HelmChart   string
	HelmVersion string
	HelmOCI     bool
	// HelmValues is a pre-rendered YAML fragment, already indented to sit
	// under `spec.values:` in the HelmRelease (4 spaces). Empty string
	// means "no user-supplied values" and the template emits `values: {}`.
	HelmValues string
}

func Render(name, tmpl string, data TemplateData) (string, error) {
	t, err := template.New(name).Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing template %s: %w", name, err)
	}
	return buf.String(), nil
}

const NamespaceTmpl = `apiVersion: v1
kind: Namespace
metadata:
  annotations:
    kustomize.toolkit.fluxcd.io/prune: disabled
  name: {{.Namespace}}
`

const NsKustomizationTmpl = `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- namespace.yaml
{{- range .}}
- {{.}}
{{- end}}
`

const FluxKustomizationSingleTmpl = `---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: {{.App}}
  namespace: flux-system
spec:
  interval: {{.Interval}}
  targetNamespace: {{.Namespace}}
  path: ./{{.NamespacesPath}}
  prune: {{.Prune}}
  sourceRef:
    kind: GitRepository
    name: flux-system
  timeout: {{.Timeout}}
`

const FluxKustomizationDualTmpl = `---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: {{.App}}
  namespace: flux-system
spec:
  interval: {{.Interval}}
  targetNamespace: {{.Namespace}}
  path: ./{{.NamespacesPath}}
  prune: {{.Prune}}
  sourceRef:
    kind: GitRepository
    name: flux-system
  timeout: {{.Timeout}}
---
apiVersion: kustomize.toolkit.fluxcd.io/v1
kind: Kustomization
metadata:
  name: {{.App}}-app
  namespace: {{.Namespace}}
spec:
  interval: {{.Interval}}
  targetNamespace: {{.Namespace}}
  path: {{.GitPath}}
  prune: {{.Prune}}
  sourceRef:
    kind: GitRepository
    name: {{.GitName}}
  timeout: {{.Timeout}}
`

const GitRepositoryTmpl = `---
apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: {{.GitName}}
  namespace: {{.Namespace}}
spec:
  interval: 1m0s
  ref:
    branch: {{.GitBranch}}
  secretRef:
    name: {{.GitSecret}}
  url: {{.GitURL}}
`

const HelmRepositoryTmpl = `---
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  labels:
    app.kubernetes.io/name: {{.App}}
  name: {{.App}}
  namespace: {{.Namespace}}
spec:
  interval: 0h10m0s
{{- if .HelmOCI}}
  type: oci
{{- end}}
  url: {{.HelmURL}}
`

const HelmReleaseTmpl = `---
apiVersion: helm.toolkit.fluxcd.io/v2beta1
kind: HelmRelease
metadata:
  name: {{.App}}
  namespace: {{.Namespace}}
spec:
  chart:
    spec:
      chart: {{.HelmChart}}
{{- if .HelmVersion}}
      version: '{{.HelmVersion}}'
{{- end}}
      sourceRef:
        kind: HelmRepository
        name: {{.App}}
        namespace: {{.Namespace}}
  interval: 0h10m0s
{{- if .HelmValues}}
  values:
{{.HelmValues}}
{{- else}}
  values: {}
{{- end}}
`

// KustomizationData holds data for the flux kustomization templates.
type KustomizationData struct {
	App            string
	Cluster        string
	Namespace      string
	Interval       string
	Timeout        string
	Prune          string
	NamespacesPath string
	GitPath        string
	GitName        string
}

// RenderNamespace renders the namespace.yaml template.
func RenderNamespace(data TemplateData) (string, error) {
	return Render("namespace", NamespaceTmpl, data)
}

// RenderNsKustomization renders the kustomization.yaml with extra resources.
func RenderNsKustomization(extraResources []string) (string, error) {
	t, err := template.New("ns-kustomization").Parse(NsKustomizationTmpl)
	if err != nil {
		return "", fmt.Errorf("parsing ns-kustomization template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, extraResources); err != nil {
		return "", fmt.Errorf("executing ns-kustomization template: %w", err)
	}
	return buf.String(), nil
}

// RenderFluxKustomization renders either single or dual kustomization.
func RenderFluxKustomization(data KustomizationData, dual bool) (string, error) {
	tmpl := FluxKustomizationSingleTmpl
	if dual {
		tmpl = FluxKustomizationDualTmpl
	}
	t, err := template.New("flux-kustomization").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parsing flux-kustomization template: %w", err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("executing flux-kustomization template: %w", err)
	}
	return buf.String(), nil
}

// RenderGitRepository renders the GitRepository source.
func RenderGitRepository(data TemplateData) (string, error) {
	return Render("git-repository", GitRepositoryTmpl, data)
}

// RenderHelmFile renders a combined HelmRepository + HelmRelease file.
func RenderHelmFile(data TemplateData) (string, error) {
	repo, err := Render("helm-repository", HelmRepositoryTmpl, data)
	if err != nil {
		return "", err
	}
	release, err := Render("helm-release", HelmReleaseTmpl, data)
	if err != nil {
		return "", err
	}
	return repo + release, nil
}

const DeploymentTmpl = `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{.App}}
  namespace: {{.Namespace}}
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: {{.App}}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{.App}}
    spec:
      containers:
      - name: {{.App}}
        image: nginx:latest
`

const StatefulSetTmpl = `---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: {{.App}}
  namespace: {{.Namespace}}
spec:
  replicas: 1
  serviceName: {{.App}}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{.App}}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{.App}}
    spec:
      containers:
      - name: {{.App}}
        image: nginx:latest
  volumeClaimTemplates: []
`

const DaemonSetTmpl = `---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: {{.App}}
  namespace: {{.Namespace}}
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: {{.App}}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{.App}}
    spec:
      containers:
      - name: {{.App}}
        image: nginx:latest
`

// RenderDeployment renders a minimal Deployment manifest.
func RenderDeployment(data TemplateData) (string, error) {
	return Render("deployment", DeploymentTmpl, data)
}

// RenderStatefulSet renders a minimal StatefulSet manifest.
func RenderStatefulSet(data TemplateData) (string, error) {
	return Render("statefulset", StatefulSetTmpl, data)
}

// RenderDaemonSet renders a minimal DaemonSet manifest.
func RenderDaemonSet(data TemplateData) (string, error) {
	return Render("daemonset", DaemonSetTmpl, data)
}

// RenderWorkload renders the workload manifest matching kind (case-insensitive).
// Accepts "deployment", "statefulset", "daemonset".
func RenderWorkload(kind string, data TemplateData) (string, error) {
	switch NormalizeWorkloadKind(kind) {
	case "Deployment":
		return RenderDeployment(data)
	case "StatefulSet":
		return RenderStatefulSet(data)
	case "DaemonSet":
		return RenderDaemonSet(data)
	default:
		return "", fmt.Errorf("unknown workload kind %q (want deployment|statefulset|daemonset)", kind)
	}
}

// NormalizeWorkloadKind canonicalizes user-supplied workload kind strings to
// "Deployment" / "StatefulSet" / "DaemonSet", or returns "" if unrecognized.
func NormalizeWorkloadKind(kind string) string {
	switch strings.ToLower(kind) {
	case "deployment":
		return "Deployment"
	case "statefulset":
		return "StatefulSet"
	case "daemonset":
		return "DaemonSet"
	}
	return ""
}
