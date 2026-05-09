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
	showNamespace   string
	showHelm        string
	showReleaseName string
	showValues      []string
	showSet         []string
	showSetString   []string
	showSkipCRDs    bool
	showValuesOnly  bool
	showKubeVersion string
	showAPIVersions []string
)

var showCmd = &cobra.Command{
	Use:   "show <cluster> <app>",
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
  flaxx show k8s myapp

  # Override values at the CLI like helm template
  flaxx show k8s myapp -f override.yaml --set image.tag=v2

  # Pick one HelmRelease when an app has several
  flaxx show k8s monitoring --helm grafana

  # Inspect just the merged values that would be passed to the chart
  flaxx show k8s myapp --values-only`,
	Args:              cobra.ExactArgs(2),
	RunE:              runShow,
	ValidArgsFunction: completeClusterAndApp,
}

func init() {
	showCmd.Flags().StringVarP(&showNamespace, "namespace", "n", "", "override release namespace (default: HelmRelease namespace)")
	showCmd.Flags().StringVar(&showHelm, "helm", "", "render only the HelmRelease whose chart matches this name")
	showCmd.Flags().StringVar(&showReleaseName, "release-name", "", "override release name (default: HelmRelease metadata.name)")
	showCmd.Flags().StringSliceVarP(&showValues, "values", "f", nil, "values file(s) merged on top of the HelmRelease values (repeatable)")
	showCmd.Flags().StringSliceVar(&showSet, "set", nil, "set values on the command line (repeatable)")
	showCmd.Flags().StringSliceVar(&showSetString, "set-string", nil, "set string values on the command line (repeatable)")
	showCmd.Flags().BoolVar(&showSkipCRDs, "skip-crds", false, "do not include CRDs from the chart")
	showCmd.Flags().BoolVar(&showValuesOnly, "values-only", false, "print the merged values instead of rendered manifests")
	showCmd.Flags().StringVar(&showKubeVersion, "kube-version", "", "Kubernetes version reported to the chart (default: "+renderer.DefaultKubeVersion+")")
	showCmd.Flags().StringSliceVar(&showAPIVersions, "api-versions", nil, "extra API versions advertised to the chart (repeatable)")

	rootCmd.AddCommand(showCmd)
}

func runShow(cmd *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfg, err := loadShowConfig(cwd)
	if err != nil {
		return err
	}

	ns := showNamespace
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

	selected, err := selectHelmRelease(helmInfos, showHelm)
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

func loadShowConfig(cwd string) (config.Config, error) {
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
		Namespace:    showNamespace,
		ReleaseName:  showReleaseName,
		SearchDirs:   []string{appNamespacesDir},
		ValueFiles:   showValues,
		SetValues:    showSet,
		StringValues: showSetString,
		IncludeCRDs:  !showSkipCRDs,
		KubeVersion:  showKubeVersion,
		APIVersions:  showAPIVersions,
		Out:          os.Stderr,
	}

	res, err := renderer.Render(ctx, opts)
	if err != nil {
		return err
	}

	if showValuesOnly {
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
