package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
	"github.com/xx4h/flaxx/internal/updater"
)

var (
	updateHelmVersion string
	updateImage       string
	updateNamespace   string
	updateDryRun      bool
)

var updateCmd = &cobra.Command{
	Use:   "update <app> <cluster>",
	Short: "Update fields in an existing app's files",
	Long: `Update specific fields in an existing app's YAML files.

Examples:
  # Bump Helm chart version
  flaxx update myapp k8s --helm-version 2.0.0

  # Update container image in a Deployment
  flaxx update myapp k8s --image registry/myapp:v1.2.3

  # Update a specific container in a multi-container pod
  flaxx update myapp k8s --image sidecar=registry/sidecar:v2.0`,
	Args: cobra.ExactArgs(2),
	RunE: runUpdate,
}

func init() {
	updateCmd.Flags().StringVar(&updateHelmVersion, "helm-version", "", "update HelmRelease chart version")
	updateCmd.Flags().StringVar(&updateImage, "image", "", "update container image (format: image:tag or name=image:tag)")
	updateCmd.Flags().StringVarP(&updateNamespace, "namespace", "n", "", "override namespace (default: app name)")
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "print output without writing files")

	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	app := args[0]
	cluster := args[1]

	if updateHelmVersion == "" && updateImage == "" {
		return fmt.Errorf("at least one of --helm-version or --image is required")
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

	appClusterDir := filepath.Join(cwd, clusterDir, app)
	appNamespacesDir := filepath.Join(cwd, namespacesDir, app)

	var updatedFiles []string

	if updateHelmVersion != "" {
		file, err := updater.UpdateHelmVersion(appClusterDir, updateHelmVersion, updateDryRun)
		if err != nil {
			return fmt.Errorf("updating helm version: %w", err)
		}
		updatedFiles = append(updatedFiles, file)
	}

	if updateImage != "" {
		file, err := updater.UpdateImage(appNamespacesDir, updateImage, updateDryRun)
		if err != nil {
			return fmt.Errorf("updating image: %w", err)
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

func resolveUpdatePath(pattern string, opts generator.Options) (string, error) {
	// Reuse the template resolution from generator
	return generator.ResolvePath(pattern, opts)
}
