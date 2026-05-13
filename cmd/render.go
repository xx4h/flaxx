package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
	"github.com/xx4h/flaxx/internal/renderer"
)

var (
	renderNamespace   string
	renderHelm        string
	renderReleaseName string
	renderValues      []string
	renderSet         []string
	renderSetString   []string
	renderSkipCRDs    bool
	renderValuesOnly  bool
	renderKubeVersion string
	renderAPIVersions []string
)

var renderCmd = &cobra.Command{
	Use:   "render <cluster> <app>",
	Short: "Render the manifests a HelmRelease would produce",
	Long: `Render the Kubernetes manifests for an app's HelmRelease, using the
chart, version, and values declared in the Flux files.

The chart is pulled into Helm's local cache and rendered client-side
(equivalent to "helm template" but driven by the HelmRelease in your
repo). spec.values, spec.valuesFrom (resolved from sibling ConfigMap
or Secret YAML files in the namespaces directory), and any overrides
supplied via --values / --set / --set-string are layered in that order.

Examples:
  # Render the manifests for an app
  flaxx render k8s myapp

  # Override values at the CLI like helm template
  flaxx render k8s myapp -f override.yaml --set image.tag=v2

  # Pick one HelmRelease when an app has several
  flaxx render k8s monitoring --helm grafana

  # Inspect just the merged values that would be passed to the chart
  flaxx render k8s myapp --values-only`,
	Args:              cobra.ExactArgs(2),
	RunE:              runRender,
	ValidArgsFunction: completeClusterAndApp,
}

func init() {
	renderCmd.Flags().StringVarP(&renderNamespace, "namespace", "n", "", "override release namespace (default: HelmRelease namespace)")
	renderCmd.Flags().StringVar(&renderHelm, "helm", "", "render only the HelmRelease whose chart matches this name")
	renderCmd.Flags().StringVar(&renderReleaseName, "release-name", "", "override release name (default: HelmRelease metadata.name)")
	renderCmd.Flags().StringSliceVarP(&renderValues, "values", "f", nil, "values file(s) merged on top of the HelmRelease values (repeatable)")
	renderCmd.Flags().StringSliceVar(&renderSet, "set", nil, "set values on the command line (repeatable)")
	renderCmd.Flags().StringSliceVar(&renderSetString, "set-string", nil, "set string values on the command line (repeatable)")
	renderCmd.Flags().BoolVar(&renderSkipCRDs, "skip-crds", false, "do not include CRDs from the chart")
	renderCmd.Flags().BoolVar(&renderValuesOnly, "values-only", false, "print the merged values instead of rendered manifests")
	renderCmd.Flags().StringVar(&renderKubeVersion, "kube-version", "", "Kubernetes version reported to the chart (default: "+renderer.DefaultKubeVersion+")")
	renderCmd.Flags().StringSliceVar(&renderAPIVersions, "api-versions", nil, "extra API versions advertised to the chart (repeatable)")

	rootCmd.AddCommand(renderCmd)
}

func runRender(cmd *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := loadRenderConfig(cwd)
	if err != nil {
		return err
	}

	ns := renderNamespace
	if ns == "" {
		ns = app
	}

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: ns}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return err
	}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return err
	}

	appClusterDir := generator.ResolveAppClusterDir(filepath.Join(cwd, clusterDir), app, cfg.Paths.ClusterSubdirs)
	appNamespacesDir := filepath.Join(cwd, namespacesDir, app)

	appFilter := generator.AppFilter(app, cfg.Paths.ClusterSubdirs)
	helmInfos, err := checker.ScanAllHelm(appClusterDir, appFilter)
	if err != nil {
		return fmt.Errorf("scanning helm releases: %w", err)
	}
	if len(helmInfos) == 0 {
		return fmt.Errorf("no HelmRelease found for app %q in cluster %q", app, cluster)
	}

	selected, err := selectHelmRelease(helmInfos, renderHelm)
	if err != nil {
		return err
	}

	for i, info := range selected {
		info.App = app
		if i > 0 {
			fmt.Println("---")
		}
		if err := renderOne(cmd.Context(), info, appNamespacesDir); err != nil {
			return err
		}
	}
	return nil
}

func loadRenderConfig(cwd string) (config.Config, error) {
	if cfgFile != "" {
		cfg, err := config.Load(cfgFile)
		if err != nil {
			return config.Config{}, fmt.Errorf("loading config: %w", err)
		}
		return cfg, nil
	}
	cfg, _, err := config.LoadFromDir(cwd)
	if err != nil {
		return config.Config{}, fmt.Errorf("loading config: %w", err)
	}
	return cfg, nil
}

// selectHelmRelease narrows the discovered HelmReleases down to the ones the
// caller wants to render. With --helm <chart>, exactly one match is required.
// Without --helm, all HelmReleases are returned (the caller stitches them).
func selectHelmRelease(infos []checker.HelmInfo, chartFilter string) ([]checker.HelmInfo, error) {
	if chartFilter == "" {
		return infos, nil
	}
	var matches []checker.HelmInfo
	for _, info := range infos {
		if info.ChartName == chartFilter {
			matches = append(matches, info)
		}
	}
	if len(matches) == 0 {
		var available []string
		for _, info := range infos {
			available = append(available, info.ChartName)
		}
		return nil, fmt.Errorf("no HelmRelease with chart %q (available: %v)", chartFilter, available)
	}
	return matches, nil
}

func renderOne(ctx context.Context, info checker.HelmInfo, appNamespacesDir string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	opts := renderer.Options{
		Info:         info,
		Namespace:    renderNamespace,
		ReleaseName:  renderReleaseName,
		SearchDirs:   []string{appNamespacesDir},
		ValueFiles:   renderValues,
		SetValues:    renderSet,
		StringValues: renderSetString,
		IncludeCRDs:  !renderSkipCRDs,
		KubeVersion:  renderKubeVersion,
		APIVersions:  renderAPIVersions,
		Out:          os.Stderr,
	}

	res, err := renderer.Render(ctx, opts)
	if err != nil {
		return err
	}

	if renderValuesOnly {
		return printMergedValues(res.MergedValue)
	}

	fmt.Print(res.Manifest)
	return nil
}

func printMergedValues(values map[string]any) error {
	if len(values) == 0 {
		fmt.Println("{}")
		return nil
	}
	data, err := yaml.Marshal(values)
	if err != nil {
		return fmt.Errorf("marshaling merged values: %w", err)
	}
	fmt.Print(string(data))
	return nil
}
