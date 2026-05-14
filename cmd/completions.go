package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/cache"
	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
)

// completeClusterAndApp provides positional arg completions:
// arg 0 = cluster, arg 1 = app.
func completeClusterAndApp(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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

// completeClusterAndHelmApp is like completeClusterAndApp but restricts the
// app list to those that actually carry at least one HelmRelease — used by
// commands that only make sense for chart-backed apps (e.g. `values`).
func completeClusterAndHelmApp(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	switch len(args) {
	case 0:
		return completeClusters(toComplete)
	case 1:
		return completeHelmApps(args[0], toComplete)
	default:
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// completeHelmApps lists apps that have at least one HelmRelease declared in
// their cluster directory. Apps without a HelmRelease are skipped silently.
func completeHelmApps(cluster, toComplete string) ([]string, cobra.ShellCompDirective) {
	cwd, cfg, err := loadCompletionConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	genOpts := generator.Options{App: "_", Cluster: cluster, Namespace: "_"}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	entries, err := os.ReadDir(filepath.Join(cwd, namespacesDir))
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	clusterRoot := filepath.Join(cwd, clusterDir)
	var apps []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, toComplete) {
			continue
		}
		appClusterDir := generator.ResolveAppClusterDir(clusterRoot, name, cfg.Paths.ClusterSubdirs)
		helmInfos, scanErr := checker.ScanAllHelm(appClusterDir, generator.AppFilter(name, cfg.Paths.ClusterSubdirs))
		if scanErr != nil || len(helmInfos) == 0 {
			continue
		}
		apps = append(apps, name)
	}

	return apps, cobra.ShellCompDirectiveNoFileComp
}

// completeApps lists app names by scanning the namespaces directory
// (always has per-app subdirectories regardless of cluster dir layout).
func completeApps(cluster, toComplete string) ([]string, cobra.ShellCompDirective) {
	cwd, cfg, err := loadCompletionConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	genOpts := generator.Options{App: "_", Cluster: cluster, Namespace: "_"}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	fullNamespacesDir := filepath.Join(cwd, namespacesDir)
	entries, err := os.ReadDir(fullNamespacesDir)
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

	ns, _ := cmd.Flags().GetString("namespace") //nolint:errcheck // flag is always registered
	if ns == "" {
		ns = app
	}

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: ns}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	appClusterDir := generator.ResolveAppClusterDir(filepath.Join(cwd, clusterDir), app, cfg.Paths.ClusterSubdirs)
	helmInfos, scanErr := checker.ScanAllHelm(appClusterDir, generator.AppFilter(app, cfg.Paths.ClusterSubdirs))
	if scanErr != nil || len(helmInfos) == 0 {
		return nil, cobra.ShellCompDirectiveError
	}

	// Complete versions from the first helm chart found
	info := helmInfos[0]
	var versions []string
	if info.RepoType == "oci" {
		taggedVersions, fetchErr := checker.FetchOCIVersions(info.RepoURL, info.ChartName)
		if fetchErr != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		for _, tv := range taggedVersions {
			versions = append(versions, tv.Tag)
		}
	} else {
		taggedVersions, fetchErr := checker.FetchHelmVersions(info.RepoURL, info.ChartName)
		if fetchErr != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		for _, tv := range taggedVersions {
			versions = append(versions, tv.Tag)
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

// completeHelmCharts provides smart completion for the --helm flag:
//   - Empty input or partial chart name: offers "chart:" prefixes for each
//     HelmRelease in the app's cluster directory.
//   - After "chart:": queries the repository for available versions and offers
//     "chart:version" entries.
func completeHelmCharts(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 2 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	cluster := args[0]
	app := args[1]

	cwd, cfg, err := loadCompletionConfig()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	ns, _ := cmd.Flags().GetString("namespace") //nolint:errcheck // flag is always registered
	if ns == "" {
		ns = app
	}

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: ns}
	clusterDir, err := generator.ResolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	appClusterDir := generator.ResolveAppClusterDir(filepath.Join(cwd, clusterDir), app, cfg.Paths.ClusterSubdirs)
	helmInfos, scanErr := checker.ScanAllHelm(appClusterDir, generator.AppFilter(app, cfg.Paths.ClusterSubdirs))
	if scanErr != nil || len(helmInfos) == 0 {
		return nil, cobra.ShellCompDirectiveError
	}

	// Phase 1: no ":" yet — offer "chart:" prefixes, one per HelmRelease.
	idx := strings.LastIndex(toComplete, ":")
	if idx < 0 {
		var completions []string
		for _, info := range helmInfos {
			prefix := info.ChartName + ":"
			if strings.HasPrefix(prefix, toComplete) {
				completions = append(completions, prefix)
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
	}

	// Phase 2: "chart:" present — find matching HelmInfo and fetch versions.
	chart := toComplete[:idx]
	var target *checker.HelmInfo
	for i := range helmInfos {
		if helmInfos[i].ChartName == chart {
			target = &helmInfos[i]
			break
		}
	}
	if target == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	var tagged []checker.TaggedVersion
	var fetchErr error
	if target.RepoType == "oci" {
		tagged, fetchErr = checker.FetchOCIVersions(target.RepoURL, target.ChartName)
	} else {
		tagged, fetchErr = checker.FetchHelmVersions(target.RepoURL, target.ChartName)
	}
	if fetchErr != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var completions []string
	for _, tv := range tagged {
		entry := chart + ":" + tv.Tag
		if strings.HasPrefix(entry, toComplete) {
			completions = append(completions, entry)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// completeImages provides smart completion for the --image flag:
//   - Empty input: offers "name=" prefixes for container selection
//   - After "name=" or "name=image:": queries the registry for available tags
//   - After "image:": queries the registry for available tags
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

	ns, _ := cmd.Flags().GetString("namespace") //nolint:errcheck // flag is always registered
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

	// Check if user has selected a container via "name=" prefix
	containerName, _ := splitImageArg(toComplete)

	// If user is typing a container name or hasn't started yet, offer container names
	if containerName == "" && !strings.Contains(toComplete, ":") && !strings.Contains(toComplete, "/") {
		var completions []string
		for _, img := range images {
			prefix := img.Container + "="
			if strings.HasPrefix(prefix, toComplete) {
				completions = append(completions, prefix)
			}
		}
		return completions, cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveNoSpace
	}

	// Find the matching container's image to determine the registry/repo
	var targetImage *checker.ImageInfo
	if containerName != "" {
		for i := range images {
			if images[i].Container == containerName {
				targetImage = &images[i]
				break
			}
		}
	} else if len(images) > 0 {
		// No container name specified — use the first (or only) container
		targetImage = &images[0]
	}

	if targetImage == nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Query registry for available tags
	tags, err := checker.FetchImageTags(*targetImage)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	imageBase := targetImage.Registry + "/" + targetImage.Repo
	// Use short form for Docker Hub library images
	if targetImage.Registry == "registry-1.docker.io" {
		if strings.HasPrefix(targetImage.Repo, "library/") {
			imageBase = strings.TrimPrefix(targetImage.Repo, "library/")
		} else {
			imageBase = targetImage.Repo
		}
	}

	var completions []string
	for _, tag := range tags {
		var entry string
		if containerName != "" {
			entry = containerName + "=" + imageBase + ":" + tag
		} else {
			entry = imageBase + ":" + tag
		}
		if strings.HasPrefix(entry, toComplete) {
			completions = append(completions, entry)
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// splitImageArg splits "name=rest" into (name, rest) or ("", original) if no "=".
func splitImageArg(s string) (string, string) {
	if idx := strings.Index(s, "="); idx >= 0 {
		return s[:idx], s[idx+1:]
	}
	return "", s
}

// loadCompletionConfig loads config from cwd for use in completion functions.
// It also installs the registry cache so completion hits the same cache as
// `flaxx check`. Cache setup failures are silent — completions must never
// error a tab-press.
func loadCompletionConfig() (string, config.Config, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", config.Config{}, err
	}
	cfg, _, err := config.LoadFromDir(cwd)
	if err != nil {
		return "", config.Config{}, err
	}

	ttl := time.Hour
	if cfg.Cache.TTL != "" {
		if d, parseErr := time.ParseDuration(cfg.Cache.TTL); parseErr == nil {
			ttl = d
		}
	}
	if c, cacheErr := cache.New(ttl, cfg.Cache.Enabled); cacheErr == nil {
		checker.SetCache(c)
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
