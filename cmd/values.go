package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/generator"
	"github.com/xx4h/flaxx/internal/renderer"
)

var (
	valuesHelm    string
	valuesVersion string
)

var valuesCmd = &cobra.Command{
	Use:   "values <cluster> <app>",
	Short: "Print the default values.yaml of an app's HelmRelease chart",
	Long: `Pull the chart referenced by an app's HelmRelease and print its
default values.yaml — the full set of options the chart maintainer
documents as overridable. Useful for discovering what knobs a chart
exposes before adding overrides to spec.values.

The chart version comes from spec.chart.spec.version in the HelmRelease,
or the latest available version if the HelmRelease is unpinned. Override
with --version to inspect a different release.

Examples:
  # Show the chart values for an app at the version pinned in the repo
  flaxx values k8s myapp

  # Inspect a different version without touching the repo
  flaxx values k8s myapp --version 2.0.0

  # Pick one HelmRelease when an app has several
  flaxx values k8s monitoring --helm grafana`,
	Args:              cobra.ExactArgs(2),
	RunE:              runValues,
	ValidArgsFunction: completeClusterAndHelmApp,
}

func init() {
	valuesCmd.Flags().StringVar(&valuesHelm, "helm", "", "show values for the HelmRelease whose chart matches this name")
	valuesCmd.Flags().StringVar(&valuesVersion, "version", "", "override the chart version (default: HelmRelease version, or latest if unpinned)")

	rootCmd.AddCommand(valuesCmd)
}

func runValues(_ *cobra.Command, args []string) error {
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

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: app}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return err
	}

	appClusterDir := generator.ResolveAppClusterDir(filepath.Join(cwd, clusterDir), app, cfg.Paths.ClusterSubdirs)
	appFilter := generator.AppFilter(app, cfg.Paths.ClusterSubdirs)
	helmInfos, err := checker.ScanAllHelm(appClusterDir, appFilter)
	if err != nil {
		return fmt.Errorf("scanning helm releases: %w", err)
	}
	if len(helmInfos) == 0 {
		return fmt.Errorf("no HelmRelease found for app %q in cluster %q", app, cluster)
	}

	selected, err := selectHelmRelease(helmInfos, valuesHelm)
	if err != nil {
		return err
	}

	for i, info := range selected {
		info.App = app
		if valuesVersion != "" {
			info.CurrentVersion = valuesVersion
		}
		if i > 0 {
			fmt.Println("---")
		}
		raw, err := renderer.Values(info)
		if err != nil {
			return err
		}
		if raw == "" {
			fmt.Fprintf(os.Stderr, "warning: chart %s has no values.yaml\n", info.ChartName)
			continue
		}
		fmt.Print(raw)
	}
	return nil
}
