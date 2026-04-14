package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
	"github.com/xx4h/flaxx/internal/output"
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
	cfgDisplay := "(none, using defaults)"
	if cfgPath != "" {
		rel, relErr := filepath.Rel(cwd, cfgPath)
		if relErr != nil {
			rel = cfgPath
		}
		cfgDisplay = rel
	}

	layoutDisplay := "flat"
	if cfg.Paths.ClusterSubdirs {
		layoutDisplay = "subdirs"
	}

	const kw = 16
	configLines := []string{
		output.KeyValue("Config file:", cfgDisplay, kw),
		output.KeyValue("Cluster dir:", cfg.Paths.ClusterDir, kw),
		output.KeyValue("Namespaces dir:", cfg.Paths.NamespacesDir, kw),
		output.KeyValue("Layout:", layoutDisplay, kw),
		output.KeyValue("Templates dir:", cfg.TemplatesDir, kw),
	}

	fmt.Println(output.Title.Render("Configuration"))
	fmt.Println(output.SectionBox.Render(strings.Join(configLines, "\n")))
	fmt.Println()

	// Discover clusters
	clusters, discoverErr := discoverClusters(cfg, cwd)
	if discoverErr != nil {
		fmt.Println(output.Warning.Render(fmt.Sprintf("Unable to scan clusters: %v", discoverErr)))
		return nil
	}

	if len(clusters) == 0 {
		fmt.Println(output.Dim.Render("No clusters found."))
		return nil
	}

	for _, cluster := range clusters {
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

var (
	appNameStyle = lipgloss.NewStyle().Bold(true).Foreground(output.ColorGreen)
	tagStyle     = lipgloss.NewStyle().Foreground(output.ColorDim)
	fileStyle    = lipgloss.NewStyle().Foreground(output.ColorCyan)
	helmStyle    = lipgloss.NewStyle().Foreground(output.ColorYellow)
	imageStyle   = lipgloss.NewStyle().Foreground(output.ColorMagenta)
)

func inspectCluster(cfg config.Config, cluster, cwd string) {
	genOpts := generator.Options{App: "_", Cluster: cluster, Namespace: "_"}

	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		fmt.Println(output.Error.Render(fmt.Sprintf("Error resolving cluster dir: %v", err)))
		return
	}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		fmt.Println(output.Error.Render(fmt.Sprintf("Error resolving namespaces dir: %v", err)))
		return
	}

	fullClusterDir := filepath.Join(cwd, clusterDir)
	fullNamespacesDir := filepath.Join(cwd, namespacesDir)

	detectedLayout := detectLayout(fullClusterDir)
	apps := discoverApps(fullNamespacesDir)

	const kw = 10
	clusterLines := []string{
		output.KeyValue("Paths:", clusterDir+", "+namespacesDir, kw),
		output.KeyValue("Layout:", detectedLayout, kw),
		output.KeyValue("Apps:", fmt.Sprintf("%d", len(apps)), kw),
	}

	fmt.Println(output.Title.Render(fmt.Sprintf("Cluster: %s", cluster)))
	fmt.Println(output.SectionBox.Render(strings.Join(clusterLines, "\n")))

	if len(apps) == 0 {
		fmt.Println()
		return
	}

	for _, app := range apps {
		fmt.Println()
		inspectApp(cfg, app, cluster, fullClusterDir, fullNamespacesDir)
	}
	fmt.Println()
}

func detectLayout(clusterDir string) string {
	entries, err := os.ReadDir(clusterDir)
	if err != nil {
		return "unknown"
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
		return "mixed"
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

	// Collect cluster files
	kustomizationName, _ := generator.ResolvePath(cfg.Naming.Kustomization, genOpts)
	helmName, _ := generator.ResolvePath(cfg.Naming.Helm, genOpts)
	gitName, _ := generator.ResolvePath(cfg.Naming.Git, genOpts)

	var clusterFiles []string
	for _, name := range []string{kustomizationName, helmName, gitName} {
		path := filepath.Join(appClusterDir, name)
		if _, statErr := os.Stat(path); statErr == nil {
			clusterFiles = append(clusterFiles, name)
		}
	}

	// Collect namespace files
	var nsFiles []string
	if nsEntries, readErr := os.ReadDir(appNamespacesDir); readErr == nil {
		for _, e := range nsEntries {
			if !e.IsDir() {
				nsFiles = append(nsFiles, e.Name())
			}
		}
	}

	// Build app info lines
	var lines []string

	if len(clusterFiles) > 0 {
		lines = append(lines, tagStyle.Render("cluster:"))
		for _, f := range clusterFiles {
			lines = append(lines, "  "+fileStyle.Render(f))
		}
	}
	if len(nsFiles) > 0 {
		lines = append(lines, tagStyle.Render("namespace:"))
		for _, f := range nsFiles {
			lines = append(lines, "  "+fileStyle.Render(f))
		}
	}

	// Helm info
	appFilter := generator.AppFilter(app, cfg.Paths.ClusterSubdirs)
	helmInfos, scanErr := checker.ScanAllHelm(appClusterDir, appFilter)
	if scanErr == nil && len(helmInfos) > 0 {
		for _, info := range helmInfos {
			parts := []string{info.ChartName}
			if info.CurrentVersion != "" {
				parts = append(parts, info.CurrentVersion)
			}
			if info.RepoURL != "" {
				parts = append(parts, output.Dim.Render("("+info.RepoURL+")"))
			}
			lines = append(lines, tagStyle.Render("helm: ")+helmStyle.Render(strings.Join(parts, " ")))
		}
	}

	// Image info
	images, imgErr := checker.ScanImages(appNamespacesDir)
	if imgErr == nil && len(images) > 0 {
		for _, img := range images {
			lines = append(lines, tagStyle.Render("image: ")+imageStyle.Render(img.Container+" ")+output.Value.Render(img.Image))
		}
	}

	if len(lines) == 0 {
		lines = append(lines, output.Dim.Render("(empty)"))
	}

	content := output.Indent.Render(strings.Join(lines, "\n"))
	fmt.Println(appNameStyle.Render("  "+app) + "\n" + content)
}
