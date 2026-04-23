package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
	"github.com/xx4h/flaxx/internal/updater"
)

var (
	updateHelmVersion string
	updateHelm        []string
	updateImage       string
	updateNamespace   string
	updateDryRun      bool
)

var updateCmd = &cobra.Command{
	Use:   "update <cluster> <app>",
	Short: "Update fields in an existing app's files",
	Long: `Update specific fields in an existing app's YAML files.

Examples:
  # Update a specific helm chart version
  flaxx update k8s myapp --helm grafana:8.0.0

  # Update multiple helm charts at once
  flaxx update k8s myapp --helm grafana:8.0.0 --helm loki:3.0.0

  # Update container image in a Deployment
  flaxx update k8s myapp --image registry/myapp:v1.2.3

  # Update a specific container in a multi-container pod
  flaxx update k8s myapp --image sidecar=registry/sidecar:v2.0

Deprecated flags:
  --helm-version    Use --helm chart:version instead`,
	Args:              cobra.ExactArgs(2),
	RunE:              runUpdate,
	ValidArgsFunction: completeClusterAndApp,
}

func init() {
	updateCmd.Flags().StringSliceVar(&updateHelm, "helm", nil, "update helm chart version (format: chart:version, repeatable)")
	updateCmd.Flags().StringVar(&updateHelmVersion, "helm-version", "", "update HelmRelease chart version (deprecated: use --helm)")
	updateCmd.Flags().StringVar(&updateImage, "image", "", "update container image (format: image:tag or name=image:tag)")
	updateCmd.Flags().StringVarP(&updateNamespace, "namespace", "n", "", "override namespace (default: app name)")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "print output without writing files")

	updateCmd.MarkFlagsMutuallyExclusive("helm", "helm-version")

	_ = updateCmd.RegisterFlagCompletionFunc("helm", completeHelmCharts)
	_ = updateCmd.RegisterFlagCompletionFunc("helm-version", completeHelmVersions)
	_ = updateCmd.RegisterFlagCompletionFunc("image", completeImages)

	rootCmd.AddCommand(updateCmd)
}

func runUpdate(_ *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]

	if len(updateHelm) == 0 && updateHelmVersion == "" && updateImage == "" {
		return fmt.Errorf("at least one of --helm, --helm-version, or --image is required")
	}

	ns := updateNamespace
	if ns == "" {
		ns = app
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	var cfg config.Config
	if cfgFile != "" {
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	} else {
		cfg, _, err = config.LoadFromDir(cwd)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: ns}
	clusterDir, err := resolveUpdatePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return err
	}
	namespacesDir, err := resolveUpdatePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return err
	}

	appClusterDir := generator.ResolveAppClusterDir(filepath.Join(cwd, clusterDir), app, cfg.Paths.ClusterSubdirs)
	appNamespacesDir := filepath.Join(cwd, namespacesDir, app)

	var updatedFiles []string

	if len(updateHelm) > 0 {
		helmUpdates, parseErr := parseHelmFlags(updateHelm)
		if parseErr != nil {
			return parseErr
		}
		files, updateErr := updater.UpdateHelmCharts(appClusterDir, helmUpdates, updateDryRun)
		if updateErr != nil {
			return fmt.Errorf("updating helm charts: %w", updateErr)
		}
		updatedFiles = append(updatedFiles, files...)
	}

	if updateHelmVersion != "" {
		fmt.Fprintln(os.Stderr, "Warning: --helm-version is deprecated, use --helm chart:version instead")
		files, updateErr := updater.UpdateHelmCharts(appClusterDir, map[string]string{"": updateHelmVersion}, updateDryRun)
		if updateErr != nil {
			return fmt.Errorf("updating helm version: %w", updateErr)
		}
		updatedFiles = append(updatedFiles, files...)
	}

	if updateImage != "" {
		file, updateErr := updater.UpdateImage(appNamespacesDir, updateImage, updateDryRun)
		if updateErr != nil {
			return fmt.Errorf("updating image: %w", updateErr)
		}
		updatedFiles = append(updatedFiles, file)
	}

	if !updateDryRun {
		fmt.Println("Updated files:")
		for _, f := range updatedFiles {
			fmt.Printf("  %s\n", f)
		}
	}

	return nil
}

// parseHelmFlags parses --helm chart:version flags into a map.
func parseHelmFlags(flags []string) (map[string]string, error) {
	updates := make(map[string]string, len(flags))
	for _, f := range flags {
		idx := strings.LastIndex(f, ":")
		if idx < 1 {
			return nil, fmt.Errorf("invalid --helm value %q: expected chart:version", f)
		}
		chart := f[:idx]
		version := f[idx+1:]
		if version == "" {
			return nil, fmt.Errorf("invalid --helm value %q: version is empty", f)
		}
		updates[chart] = version
	}
	return updates, nil
}

func resolveUpdatePath(pattern string, opts generator.Options) (string, error) {
	return generator.ResolvePath(pattern, opts)
}
