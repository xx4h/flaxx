package templates

import (
	"bytes"
	"fmt"
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
  values: {}
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
