package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Analyze the current directory's flux repository structure",
	Long: `Inspect the current directory and report what flaxx detects:
configuration, clusters, apps, layout type (flat or subdirs),
and per-app files.`,
	Args: cobra.NoArgs,
	RunE: runInspect,
}

func init() {
	rootCmd.AddCommand(inspectCmd)
}

func runInspect(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	var cfg config.Config
	var cfgPath string
	if cfgFile != "" {
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		cfgPath = cfgFile
	} else {
		cfg, cfgPath, err = config.LoadFromDir(cwd)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	// Config section
	fmt.Println("Configuration:")
	if cfgPath != "" {
		rel, relErr := filepath.Rel(cwd, cfgPath)
		if relErr != nil {
			rel = cfgPath
		}
		fmt.Printf("  Config file:     %s\n", rel)
	} else {
		fmt.Println("  Config file:     (none, using defaults)")
	}
	fmt.Printf("  Cluster dir:     %s\n", cfg.Paths.ClusterDir)
	fmt.Printf("  Namespaces dir:  %s\n", cfg.Paths.NamespacesDir)
	if cfg.Paths.ClusterSubdirs {
		fmt.Println("  Layout:          subdirs (per-app subdirectories in cluster dir)")
	} else {
		fmt.Println("  Layout:          flat (files directly in cluster dir)")
	}
	fmt.Printf("  Templates dir:   %s\n", cfg.TemplatesDir)
	fmt.Println()

	// Discover clusters
	clusters, discoverErr := discoverClusters(cfg, cwd)
	if discoverErr != nil {
		fmt.Printf("Clusters: (unable to scan: %v)\n", discoverErr)
		return nil
	}

	if len(clusters) == 0 {
		fmt.Println("Clusters: (none found)")
		return nil
	}

	fmt.Printf("Clusters: %d found\n", len(clusters))
	for _, cluster := range clusters {
		fmt.Printf("\n  [%s]\n", cluster)
		inspectCluster(cfg, cluster, cwd)
	}

	return nil
}

func discoverClusters(cfg config.Config, cwd string) ([]string, error) {
	clusterParent, err := clusterParentDir(cfg.Paths.ClusterDir, cwd)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(clusterParent)
	if err != nil {
		return nil, err
	}

	suffix := clusterDirSuffix(cfg.Paths.ClusterDir)
	nsSuffix := clusterDirSuffix(cfg.Paths.NamespacesDir)

	var clusters []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if nsSuffix != "" && strings.HasSuffix(name, nsSuffix) {
			continue
		}
		cluster := name
		if suffix != "" {
			cluster = strings.TrimSuffix(name, suffix)
		}
		clusters = append(clusters, cluster)
	}

	return clusters, nil
}

func inspectCluster(cfg config.Config, cluster, cwd string) {
	genOpts := generator.Options{App: "_", Cluster: cluster, Namespace: "_"}

	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		fmt.Printf("    (error resolving cluster dir: %v)\n", err)
		return
	}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		fmt.Printf("    (error resolving namespaces dir: %v)\n", err)
		return
	}

	fullClusterDir := filepath.Join(cwd, clusterDir)
	fullNamespacesDir := filepath.Join(cwd, namespacesDir)

	fmt.Printf("    Cluster dir:    %s\n", clusterDir)
	fmt.Printf("    Namespaces dir: %s\n", namespacesDir)

	// Detect actual layout by examining what's on disk
	detectedLayout := detectLayout(fullClusterDir)
	fmt.Printf("    Detected layout: %s\n", detectedLayout)

	// Discover apps from namespaces dir
	apps := discoverApps(fullNamespacesDir)
	if len(apps) == 0 {
		fmt.Println("    Apps: (none found)")
		return
	}

	fmt.Printf("    Apps: %d\n", len(apps))
	for _, app := range apps {
		inspectApp(cfg, app, cluster, fullClusterDir, fullNamespacesDir)
	}
}

func detectLayout(clusterDir string) string {
	entries, err := os.ReadDir(clusterDir)
	if err != nil {
		return "unknown (directory not readable)"
	}

	hasSubdirs := false
	hasFlatKustomizations := false

	for _, e := range entries {
		if e.IsDir() && e.Name() != "flux-system" {
			hasSubdirs = true
		}
		if !e.IsDir() && strings.HasSuffix(e.Name(), "-kustomization.yaml") {
			hasFlatKustomizations = true
		}
	}

	switch {
	case hasFlatKustomizations && !hasSubdirs:
		return "flat"
	case hasSubdirs && !hasFlatKustomizations:
		return "subdirs"
	case hasFlatKustomizations && hasSubdirs:
		return "mixed (both flat files and subdirectories)"
	default:
		return "empty"
	}
}

func discoverApps(namespacesDir string) []string {
	entries, err := os.ReadDir(namespacesDir)
	if err != nil {
		return nil
	}

	var apps []string
	for _, e := range entries {
		if e.IsDir() {
			apps = append(apps, e.Name())
		}
	}
	return apps
}

func inspectApp(cfg config.Config, app, cluster, clusterDir, namespacesDir string) {
	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: app}
	appClusterDir := generator.ResolveAppClusterDir(clusterDir, app, cfg.Paths.ClusterSubdirs)
	appNamespacesDir := filepath.Join(namespacesDir, app)

	fmt.Printf("\n    [%s/%s]\n", cluster, app)

	// Check cluster dir files
	kustomizationName, _ := generator.ResolvePath(cfg.Naming.Kustomization, genOpts)
	helmName, _ := generator.ResolvePath(cfg.Naming.Helm, genOpts)
	gitName, _ := generator.ResolvePath(cfg.Naming.Git, genOpts)

	clusterFiles := []struct {
		name string
		kind string
	}{
		{kustomizationName, "Flux Kustomization"},
		{helmName, "Helm"},
		{gitName, "Git"},
	}

	fmt.Println("      Cluster files:")
	foundCluster := false
	for _, f := range clusterFiles {
		path := filepath.Join(appClusterDir, f.name)
		if _, statErr := os.Stat(path); statErr == nil {
			fmt.Printf("        %s (%s)\n", f.name, f.kind)
			foundCluster = true
		}
	}
	if !foundCluster {
		fmt.Println("        (none)")
	}

	// Check namespace dir files
	fmt.Println("      Namespace files:")
	nsEntries, err := os.ReadDir(appNamespacesDir)
	if err != nil {
		fmt.Println("        (directory not found)")
		return
	}
	for _, e := range nsEntries {
		if !e.IsDir() {
			fmt.Printf("        %s\n", e.Name())
		}
	}

	// Show helm info if available
	appFilter := generator.AppFilter(app, cfg.Paths.ClusterSubdirs)
	helmInfos, scanErr := checker.ScanAllHelm(appClusterDir, appFilter)
	if scanErr == nil && len(helmInfos) > 0 {
		fmt.Println("      Helm:")
		for _, info := range helmInfos {
			fmt.Printf("        Chart:   %s\n", info.ChartName)
			if info.CurrentVersion != "" {
				fmt.Printf("        Version: %s\n", info.CurrentVersion)
			}
			if info.RepoURL != "" {
				fmt.Printf("        Repo:    %s\n", info.RepoURL)
			}
			if info.RepoType != "" {
				fmt.Printf("        Type:    %s\n", info.RepoType)
			}
		}
	}

	// Show image info if available
	images, imgErr := checker.ScanImages(appNamespacesDir)
	if imgErr == nil && len(images) > 0 {
		fmt.Println("      Images:")
		for _, img := range images {
			fmt.Printf("        %s: %s\n", img.Container, img.Image)
		}
	}
}
