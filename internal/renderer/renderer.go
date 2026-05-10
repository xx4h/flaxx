// Package renderer renders the Kubernetes manifests that a Flux HelmRelease
// would produce, by pulling its chart locally and running the Helm template
// engine client-side.
package renderer

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/cli/values"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/registry"
	"sigs.k8s.io/yaml"

	"github.com/xx4h/flaxx/internal/checker"
)

// Options drives a single Render call.
type Options struct {
	// Info is the HelmRelease to render — chart name, version, repository,
	// inline values, and valuesFrom references all come from here.
	Info checker.HelmInfo

	// Namespace overrides Info.Namespace when non-empty.
	Namespace string

	// ReleaseName overrides Info.Name when non-empty. Defaults to Info.Name
	// (the HelmRelease metadata.name) which matches how Flux installs it.
	ReleaseName string

	// SearchDirs are filesystem directories scanned for sibling ConfigMap /
	// Secret resources referenced by spec.valuesFrom. Order does not matter.
	SearchDirs []string

	// ValueFiles, SetValues, StringValues are forwarded to Helm's standard
	// --values / --set / --set-string flags and layered on top of the values
	// derived from the HelmRelease itself.
	ValueFiles   []string
	SetValues    []string
	StringValues []string

	// IncludeCRDs controls whether the rendered output includes the chart's
	// crds/ directory. Callers set this explicitly; the default zero value
	// (false) skips CRDs.
	IncludeCRDs bool

	// KubeVersion overrides the Kubernetes version reported to the chart's
	// kubeVersion constraint check (e.g. "v1.30.0"). Empty falls back to
	// renderer.DefaultKubeVersion.
	KubeVersion string

	// APIVersions appends to the set of API versions advertised to the chart
	// (e.g. "monitoring.coreos.com/v1"). Charts that gate templates on
	// .Capabilities.APIVersions.Has need this.
	APIVersions []string

	// Out receives non-fatal warnings (e.g. missing optional valuesFrom
	// references). nil discards them.
	Out io.Writer
}

// Result carries the rendered manifests plus the values that produced them,
// so callers can show both.
type Result struct {
	Manifest    string
	MergedValue map[string]any
}

// DefaultKubeVersion is reported to charts when Options.KubeVersion is empty.
// Helm's own default is several years stale, so we ship a more recent value
// to keep modern charts that gate on kubeVersion from refusing to render.
const DefaultKubeVersion = "v1.30.0"

// Render pulls the chart referenced by opts.Info and renders its templates
// using the merged values derived from spec.values, spec.valuesFrom, and
// CLI-supplied overrides. The returned Result.Manifest is ready to print.
func Render(ctx context.Context, opts Options) (*Result, error) {
	if opts.Info.ChartName == "" {
		return nil, fmt.Errorf("HelmRelease %q has no chart name", opts.Info.Name)
	}
	if opts.Info.RepoURL == "" {
		return nil, fmt.Errorf("HelmRelease %q has no resolved HelmRepository URL", opts.Info.Name)
	}

	out := opts.Out
	if out == nil {
		out = io.Discard
	}

	settings := cli.New()

	mergedValues, err := buildValues(opts, settings, out)
	if err != nil {
		return nil, err
	}

	actionConfig := new(action.Configuration)
	if isOCI(opts.Info) {
		regClient, regErr := registry.NewClient(
			registry.ClientOptDebug(false),
			registry.ClientOptWriter(io.Discard),
			registry.ClientOptCredentialsFile(settings.RegistryConfig),
		)
		if regErr != nil {
			return nil, fmt.Errorf("creating registry client: %w", regErr)
		}
		actionConfig.RegistryClient = regClient
	}

	kubeVersion := opts.KubeVersion
	if kubeVersion == "" {
		kubeVersion = DefaultKubeVersion
	}
	parsedKV, err := chartutil.ParseKubeVersion(kubeVersion)
	if err != nil {
		return nil, fmt.Errorf("parsing kube version %q: %w", kubeVersion, err)
	}

	inst := action.NewInstall(actionConfig)
	inst.DryRun = true
	inst.Replace = true
	inst.ClientOnly = true
	inst.IncludeCRDs = opts.IncludeCRDs
	inst.DisableHooks = true
	inst.Namespace = resolveNamespace(opts)
	inst.ReleaseName = resolveReleaseName(opts)
	inst.Version = opts.Info.CurrentVersion
	inst.KubeVersion = parsedKV
	inst.APIVersions = append(inst.APIVersions, opts.APIVersions...)

	chartRef := opts.Info.ChartName
	if isOCI(opts.Info) {
		chartRef = strings.TrimSuffix(opts.Info.RepoURL, "/") + "/" + opts.Info.ChartName
	} else {
		inst.RepoURL = opts.Info.RepoURL
	}

	chartPath, err := inst.LocateChart(chartRef, settings)
	if err != nil {
		return nil, fmt.Errorf("locating chart %s: %w", chartRef, err)
	}

	ch, err := loader.Load(chartPath)
	if err != nil {
		return nil, fmt.Errorf("loading chart %s: %w", chartPath, err)
	}

	rel, err := inst.RunWithContext(ctx, ch, mergedValues)
	if err != nil {
		return nil, fmt.Errorf("rendering chart: %w", err)
	}

	return &Result{
		Manifest:    rel.Manifest,
		MergedValue: mergedValues,
	}, nil
}

func buildValues(opts Options, settings *cli.EnvSettings, out io.Writer) (map[string]any, error) {
	merged := map[string]any{}

	// Flux applies valuesFrom in order, with later entries overriding earlier
	// ones, then merges spec.values on top. We mirror that.
	for _, ref := range opts.Info.ValuesFrom {
		data, err := resolveValuesFrom(ref, opts.SearchDirs)
		if err != nil {
			if ref.Optional {
				fmt.Fprintf(out, "warning: skipping optional valuesFrom %s/%s: %v\n", ref.Kind, ref.Name, err)
				continue
			}
			return nil, fmt.Errorf("resolving valuesFrom %s/%s: %w", ref.Kind, ref.Name, err)
		}
		if data != nil {
			merged = mergeMaps(merged, data)
		}
	}

	if opts.Info.Values != nil {
		merged = mergeMaps(merged, opts.Info.Values)
	}

	cliOpts := values.Options{
		ValueFiles:   opts.ValueFiles,
		Values:       opts.SetValues,
		StringValues: opts.StringValues,
	}
	cliVals, err := cliOpts.MergeValues(getter.All(settings))
	if err != nil {
		return nil, err
	}
	if len(cliVals) > 0 {
		merged = mergeMaps(merged, cliVals)
	}

	return merged, nil
}

// resolveValuesFrom locates a sibling ConfigMap / Secret in the repo and
// returns the parsed YAML map for the referenced key. The default key is
// "values.yaml", matching Flux.
func resolveValuesFrom(ref checker.ValuesFromRef, searchDirs []string) (map[string]any, error) {
	key := ref.ValuesKey
	if key == "" {
		key = "values.yaml"
	}

	raw, err := findResourceData(ref.Kind, ref.Name, key, searchDirs)
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, fmt.Errorf("parsing values from %s/%s key %s: %w", ref.Kind, ref.Name, key, err)
	}
	return parsed, nil
}

// findResourceData scans searchDirs for a YAML file containing a resource of
// the given kind/name and returns the value at data[key] (or stringData[key]
// for Secrets). Empty string + nil error means "not found".
func findResourceData(kind, name, key string, searchDirs []string) (string, error) {
	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			n := e.Name()
			if !strings.HasSuffix(n, ".yaml") && !strings.HasSuffix(n, ".yml") {
				continue
			}
			path := filepath.Join(dir, n)
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				continue
			}

			docs := splitYAML(data)
			for _, doc := range docs {
				var meta struct {
					Kind     string `yaml:"kind"`
					Metadata struct {
						Name string `yaml:"name"`
					} `yaml:"metadata"`
					Data       map[string]string `yaml:"data"`
					StringData map[string]string `yaml:"stringData"`
				}
				if err := yaml.Unmarshal(doc, &meta); err != nil {
					continue
				}
				if meta.Kind != kind || meta.Metadata.Name != name {
					continue
				}
				if v, ok := meta.StringData[key]; ok {
					return v, nil
				}
				if v, ok := meta.Data[key]; ok {
					return v, nil
				}
				return "", fmt.Errorf("%s %q has no key %q", kind, name, key)
			}
		}
	}
	return "", fmt.Errorf("%s %q not found in %v", kind, name, searchDirs)
}

// splitYAML breaks a multi-document YAML stream into individual documents.
func splitYAML(data []byte) [][]byte {
	parts := strings.Split(string(data), "\n---")
	out := make([][]byte, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimPrefix(p, "---")
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, []byte(p))
	}
	return out
}

// mergeMaps deep-merges b into a; b wins on conflicts. This matches Helm's
// own coalesce semantics for layered values.
func mergeMaps(a, b map[string]any) map[string]any {
	out := make(map[string]any, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if vmap, ok := v.(map[string]any); ok {
			if existing, ok := out[k].(map[string]any); ok {
				out[k] = mergeMaps(existing, vmap)
				continue
			}
		}
		out[k] = v
	}
	return out
}

func isOCI(info checker.HelmInfo) bool {
	if info.RepoType == "oci" {
		return true
	}
	return strings.HasPrefix(info.RepoURL, "oci://")
}

func resolveNamespace(opts Options) string {
	if opts.Namespace != "" {
		return opts.Namespace
	}
	if opts.Info.Namespace != "" {
		return opts.Info.Namespace
	}
	return "default"
}

func resolveReleaseName(opts Options) string {
	if opts.ReleaseName != "" {
		return opts.ReleaseName
	}
	if opts.Info.Name != "" {
		return opts.Info.Name
	}
	return opts.Info.ChartName
}
