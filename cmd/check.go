package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
)

var (
	checkNamespace string
	checkAll       bool
)

var checkCmd = &cobra.Command{
	Use:   "check <cluster> [<app>]",
	Short: "Check for newer Helm chart and container image versions",
	Long: `Check for available updates to Helm charts and container images used by an app.

Reads HelmRelease/HelmRepository from the cluster directory and
Deployment/StatefulSet/DaemonSet from the namespaces directory,
queries upstream repositories, and reports available newer versions.

Supports both standard Helm repositories and OCI registries.

Examples:
  # Check for newer versions
  flaxx check k8s myapp

  # Check all apps in a cluster
  flaxx check k8s --all

  # Check with namespace override
  flaxx check k8s myapp -n custom-ns`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runCheck,
}

func init() {
	checkCmd.Flags().StringVarP(&checkNamespace, "namespace", "n", "", "override namespace (default: app name)")
	checkCmd.Flags().BoolVarP(&checkAll, "all", "a", false, "check all apps in the cluster")

	rootCmd.AddCommand(checkCmd)
}

func runCheck(cmd *cobra.Command, args []string) error {
	var app, cluster string

	cluster = args[0]

	if checkAll {
		if len(args) > 1 {
			return fmt.Errorf("--all does not take an app argument")
		}
	} else {
		if len(args) < 2 {
			return fmt.Errorf("app argument is required (or use --all)")
		}
		app = args[1]
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

	if checkAll {
		return runCheckAll(cfg, cluster, cwd)
	}

	return runCheckApp(cfg, app, cluster, cwd)
}

func runCheckApp(cfg config.Config, app, cluster, cwd string) error {
	ns := checkNamespace
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

	appClusterDir := filepath.Join(cwd, clusterDir, app)
	appNamespacesDir := filepath.Join(cwd, namespacesDir, app)

	printed := false

	// Check Helm chart versions
	info, err := checker.ScanApp(appClusterDir)
	if err == nil {
		info.App = app
		result, err := checker.CheckHelm(info)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: checking helm versions: %v\n", err)
		} else {
			printCheckResult(result)
			printed = true
		}
	}

	// Check container image versions
	images, err := checker.ScanImages(appNamespacesDir)
	if err == nil && len(images) > 0 {
		var imgErrors []string
		for _, img := range images {
			result, err := checker.CheckImage(img)
			if err != nil {
				imgErrors = append(imgErrors, fmt.Sprintf("%s: %v", img.Image, err))
				continue
			}
			if printed {
				fmt.Println()
			}
			printed = true
			printImageResult(result)
		}
		if len(imgErrors) > 0 {
			fmt.Println()
			fmt.Println("Image check errors:")
			for _, e := range imgErrors {
				fmt.Printf("  %s\n", e)
			}
		}
	}

	if !printed {
		return fmt.Errorf("no Helm charts or container images found for %s", app)
	}

	return nil
}

func runCheckAll(cfg config.Config, cluster, cwd string) error {
	genOpts := generator.Options{App: "_", Cluster: cluster, Namespace: "_"}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return err
	}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return err
	}

	fullClusterDir := filepath.Join(cwd, clusterDir)
	fullNamespacesDir := filepath.Join(cwd, namespacesDir)

	// Collect all app names from both directories
	appNames := make(map[string]bool)
	if entries, err := os.ReadDir(fullClusterDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				appNames[e.Name()] = true
			}
		}
	}
	if entries, err := os.ReadDir(fullNamespacesDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				appNames[e.Name()] = true
			}
		}
	}

	if len(appNames) == 0 {
		fmt.Println("No apps found.")
		return nil
	}

	var checkErrors []string
	first := true

	for app := range appNames {
		appClusterDir := filepath.Join(fullClusterDir, app)
		appNsDir := filepath.Join(fullNamespacesDir, app)
		appPrinted := false

		// Check Helm
		info, err := checker.ScanApp(appClusterDir)
		if err == nil {
			info.App = app
			result, err := checker.CheckHelm(info)
			if err != nil {
				checkErrors = append(checkErrors, fmt.Sprintf("%s (helm): %v", app, err))
			} else {
				if !first {
					fmt.Println()
				}
				first = false
				appPrinted = true
				printCheckResult(result)
			}
		}

		// Check images
		images, err := checker.ScanImages(appNsDir)
		if err == nil {
			for _, img := range images {
				result, err := checker.CheckImage(img)
				if err != nil {
					checkErrors = append(checkErrors, fmt.Sprintf("%s (image %s): %v", app, img.Image, err))
					continue
				}
				if !first || appPrinted {
					fmt.Println()
				}
				first = false
				appPrinted = true
				printImageResult(result)
			}
		}
	}

	if len(checkErrors) > 0 {
		fmt.Println()
		fmt.Println("Errors:")
		for _, e := range checkErrors {
			fmt.Printf("  %s\n", e)
		}
	}

	return nil
}

func printImageResult(r *checker.ImageCheckResult) {
	fmt.Printf("Image:      %s\n", r.Image)
	fmt.Printf("Container:  %s\n", r.Container)
	fmt.Printf("Registry:   %s\n", r.Registry)

	if r.Tag != "" {
		fmt.Printf("Current:    %s\n", r.Tag)
	} else {
		fmt.Printf("Current:    (no tag)\n")
	}

	if r.LatestVersion != "" {
		fmt.Printf("Latest:     %s\n", r.LatestVersion)
	}

	if len(r.AvailableUpdates) == 0 {
		fmt.Println("\nUp to date.")
	} else {
		fmt.Printf("\nAvailable updates (%d):\n", len(r.AvailableUpdates))
		limit := 10
		if len(r.AvailableUpdates) < limit {
			limit = len(r.AvailableUpdates)
		}
		for _, v := range r.AvailableUpdates[:limit] {
			fmt.Printf("  %s\n", v)
		}
		if len(r.AvailableUpdates) > limit {
			fmt.Printf("  ... and %d more\n", len(r.AvailableUpdates)-limit)
		}
	}
}

func printCheckResult(r *checker.CheckResult) {
	fmt.Printf("Chart:      %s\n", r.ChartName)
	fmt.Printf("Repository: %s\n", r.RepoURL)

	if r.CurrentVersion != "" {
		fmt.Printf("Current:    %s\n", r.CurrentVersion)
	} else {
		fmt.Printf("Current:    (not pinned)\n")
	}

	fmt.Printf("Latest:     %s\n", r.LatestVersion)

	if len(r.AvailableUpdates) == 0 {
		fmt.Println("\nUp to date.")
	} else {
		fmt.Printf("\nAvailable updates (%d):\n", len(r.AvailableUpdates))
		limit := 10
		if len(r.AvailableUpdates) < limit {
			limit = len(r.AvailableUpdates)
		}
		for _, v := range r.AvailableUpdates[:limit] {
			fmt.Printf("  %s\n", v)
		}
		if len(r.AvailableUpdates) > limit {
			fmt.Printf("  ... and %d more\n", len(r.AvailableUpdates)-limit)
		}
	}
}
