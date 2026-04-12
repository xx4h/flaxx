package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
)

// completeClusterAndApp provides positional arg completions:
// arg 0 = cluster, arg 1 = app.
func completeClusterAndApp(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeClusters(toComplete)
	case 1:
		return completeApps(args[0], toComplete)
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeClusters lists available cluster names by scanning the cluster directory pattern.
func completeClusters(toComplete string) ([]string, cobra.ShellCompDirective) {
	cwd, cfg, err := loadCompletionConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	clusterParent, err := clusterParentDir(cfg.Paths.ClusterDir, cwd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	entries, err := os.ReadDir(clusterParent)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	suffix := clusterDirSuffix(cfg.Paths.ClusterDir)
	nsSuffix := clusterDirSuffix(cfg.Paths.NamespacesDir)

	var clusters []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		// Skip namespaces directories
		if nsSuffix != "" && strings.HasSuffix(name, nsSuffix) {
			continue
		}
		// Strip suffix if present to get the cluster name
		cluster := name
		if suffix != "" {
			cluster = strings.TrimSuffix(name, suffix)
		}
		if strings.HasPrefix(cluster, toComplete) {
			clusters = append(clusters, cluster)
		}
	}

	return clusters, cobra.ShellCompDirectiveNoFileComp
}

// completeApps lists app names within a cluster's directory.
func completeApps(cluster, toComplete string) ([]string, cobra.ShellCompDirective) {
	cwd, cfg, err := loadCompletionConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	genOpts := generator.Options{App: "_", Cluster: cluster, Namespace: "_"}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	fullClusterDir := filepath.Join(cwd, clusterDir)
	entries, err := os.ReadDir(fullClusterDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var apps []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), toComplete) {
			apps = append(apps, e.Name())
		}
	}

	return apps, cobra.ShellCompDirectiveNoFileComp
}

// completeHelmVersions fetches available Helm chart versions for the given app.
func completeHelmVersions(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cluster := args[0]
	app := args[1]

	cwd, cfg, err := loadCompletionConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	ns, _ := cmd.Flags().GetString("namespace")
	if ns == "" {
		ns = app
	}

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: ns}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	appClusterDir := filepath.Join(cwd, clusterDir, app)
	info, err := checker.ScanApp(appClusterDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var versions []string
	if info.RepoType == "oci" {
		semVersions, err := checker.FetchOCIVersions(info.RepoURL, info.ChartName)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		for _, v := range semVersions {
			versions = append(versions, v.Original())
		}
	} else {
		semVersions, err := checker.FetchHelmVersions(info.RepoURL, info.ChartName)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		for _, v := range semVersions {
			versions = append(versions, v.Original())
		}
	}

	var filtered []string
	for _, v := range versions {
		if strings.HasPrefix(v, toComplete) {
			filtered = append(filtered, v)
		}
	}

	return filtered, cobra.ShellCompDirectiveNoFileComp
}

// completeImages lists container names from workload resources as "name=" prefixes,
// or full current image references.
func completeImages(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cluster := args[0]
	app := args[1]

	cwd, cfg, err := loadCompletionConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	ns, _ := cmd.Flags().GetString("namespace")
	if ns == "" {
		ns = app
	}

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: ns}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	appNsDir := filepath.Join(cwd, namespacesDir, app)
	images, err := checker.ScanImages(appNsDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var completions []string
	for _, img := range images {
		// Offer "name=currentimage" for multi-container selection
		entry := img.Container + "=" + img.Image
		if strings.HasPrefix(entry, toComplete) {
			completions = append(completions, entry)
		}
		// Also offer just the current image ref
		if strings.HasPrefix(img.Image, toComplete) {
			completions = append(completions, img.Image)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// loadCompletionConfig loads config from cwd for use in completion functions.
func loadCompletionConfig() (string, config.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", config.Config{}, err
	}
	cfg, _, err := config.LoadFromDir(cwd)
	if err != nil {
		return "", config.Config{}, err
	}
	return cwd, cfg, nil
}

// clusterParentDir extracts the parent directory from a cluster dir pattern.
// e.g. "clusters/{{.Cluster}}" → "<cwd>/clusters"
func clusterParentDir(pattern, cwd string) (string, error) {
	idx := strings.Index(pattern, "{{")
	if idx < 0 {
		return filepath.Join(cwd, pattern), nil
	}
	parent := pattern[:idx]
	parent = strings.TrimRight(parent, "/")
	return filepath.Join(cwd, parent), nil
}

// clusterDirSuffix extracts the suffix after {{.Cluster}} in a path pattern.
// e.g. "clusters/{{.Cluster}}-namespaces" → "-namespaces"
func clusterDirSuffix(pattern string) string {
	marker := "{{.Cluster}}"
	idx := strings.Index(pattern, marker)
	if idx < 0 {
		return ""
	}
	rest := pattern[idx+len(marker):]
	if slashIdx := strings.Index(rest, "/"); slashIdx >= 0 {
		rest = rest[:slashIdx]
	}
	return rest
}
