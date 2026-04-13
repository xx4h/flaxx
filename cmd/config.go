package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/xx4h/flaxx/internal/config"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage flaxx configuration",
	Long:  "Show current configuration or generate a .flaxx.yaml from the detected repository structure.",
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current effective configuration as YAML",
	Args:  cobra.NoArgs,
	RunE:  runConfigShow,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate .flaxx.yaml from the detected repository structure",
	Long: `Scan the current directory for a flux repository structure and generate
a .flaxx.yaml configuration file that matches it.

Detects cluster directories, namespace directories, and whether the
layout uses flat files or per-app subdirectories.`,
	Args: cobra.NoArgs,
	RunE: runConfigInit,
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configInitCmd)
	rootCmd.AddCommand(configCmd)
}

// detectedStructure holds the results of scanning a directory for flux repo patterns.
type detectedStructure struct {
	clusterDir    string
	namespacesDir string
	subdirs       bool
	clusterNames  []string
}

func runConfigShow(_ *cobra.Command, _ []string) error {
	cfg, _, err := detectConfig()
	if err != nil {
		return err
	}

	data, marshalErr := yaml.Marshal(cfg)
	if marshalErr != nil {
		return fmt.Errorf("marshaling config: %w", marshalErr)
	}

	fmt.Print(string(data))
	return nil
}

func runConfigInit(_ *cobra.Command, _ []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	cfgPath := filepath.Join(cwd, ".flaxx.yaml")
	if _, statErr := os.Stat(cfgPath); statErr == nil {
		return fmt.Errorf(".flaxx.yaml already exists; remove it first to regenerate")
	}

	cfg, detected, err := detectConfig()
	if err != nil {
		return err
	}

	fmt.Println("Detected structure:")
	fmt.Printf("  Cluster dir:    %s\n", detected.clusterDir)
	fmt.Printf("  Namespaces dir: %s\n", detected.namespacesDir)
	if detected.subdirs {
		fmt.Println("  Layout:         subdirs")
	} else {
		fmt.Println("  Layout:         flat")
	}
	fmt.Printf("  Clusters:       %s\n", strings.Join(detected.clusterNames, ", "))
	fmt.Println()

	data, marshalErr := yaml.Marshal(cfg)
	if marshalErr != nil {
		return fmt.Errorf("marshaling config: %w", marshalErr)
	}

	if writeErr := os.WriteFile(cfgPath, data, 0o644); writeErr != nil { //nolint:gosec // config file needs to be readable
		return fmt.Errorf("writing .flaxx.yaml: %w", writeErr)
	}

	fmt.Println("Created .flaxx.yaml")
	return nil
}

// detectConfig scans the directory structure and returns a config that matches it.
func detectConfig() (config.Config, *detectedStructure, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("getting working directory: %w", err)
	}

	detected, err := detectStructure(cwd)
	if err != nil {
		return config.Config{}, nil, fmt.Errorf("detecting structure: %w", err)
	}
	if detected == nil {
		return config.Config{}, nil, fmt.Errorf("no flux repository structure detected")
	}

	cfg := config.DefaultConfig()
	cfg.Paths.ClusterDir = detected.clusterDir
	cfg.Paths.NamespacesDir = detected.namespacesDir
	cfg.Paths.ClusterSubdirs = detected.subdirs

	return cfg, detected, nil
}

// detectStructure walks the directory tree to find flux-like patterns and
// infers the cluster/namespaces directory templates.
func detectStructure(cwd string) (*detectedStructure, error) {
	// Strategy: look for directories containing *-kustomization.yaml files
	// (cluster dirs) or namespace.yaml files (namespace dirs), then infer
	// the path templates.

	// First, try common patterns
	patterns := []struct {
		clusterDir    string
		namespacesDir string
	}{
		// Flux official: clusters/<cluster>/apps + apps/<app>
		{"clusters/*/apps", "apps"},
		// more flat variant: clusters/<cluster>/apps + apps/<app>
		{"clusters/*", "apps"},
		// flaxx default: clusters/<cluster> + clusters/<cluster>-namespaces
		{"clusters/*", "clusters/*-namespaces"},
		// Nested under a prefix
		{"flux/clusters/*/apps", "flux/apps"},
		{"flux/clusters/*", "flux/clusters/*-namespaces"},
	}

	for _, p := range patterns {
		result := tryPattern(cwd, p.clusterDir, p.namespacesDir)
		if result != nil {
			return result, nil
		}
	}

	// Fallback: scan for any directory containing *-kustomization.yaml
	return scanForStructure(cwd)
}

// tryPattern checks if a given glob pattern matches cluster and namespace directories.
func tryPattern(cwd, clusterGlob, nsGlob string) *detectedStructure {
	clusterMatches, err := filepath.Glob(filepath.Join(cwd, clusterGlob))
	if err != nil || len(clusterMatches) == 0 {
		return nil
	}

	// Verify at least one match has kustomization files or app-like content
	var validClusters []string
	var isSubdirs bool

	for _, match := range clusterMatches {
		entries, readErr := os.ReadDir(match)
		if readErr != nil {
			continue
		}

		hasFluxContent := false
		hasAppSubdirs := false
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), "-kustomization.yaml") {
				hasFluxContent = true
			}
			if e.IsDir() && e.Name() != "flux-system" {
				// Check if it contains kustomization files (subdir layout)
				subEntries, subErr := os.ReadDir(filepath.Join(match, e.Name()))
				if subErr == nil {
					for _, se := range subEntries {
						if strings.HasSuffix(se.Name(), "-kustomization.yaml") {
							hasAppSubdirs = true
							hasFluxContent = true
						}
					}
				}
			}
		}

		if hasFluxContent {
			// Extract cluster name from path
			rel, relErr := filepath.Rel(cwd, match)
			if relErr != nil {
				continue
			}
			parts := strings.Split(rel, string(filepath.Separator))
			clusterName := parts[len(parts)-1]
			// For patterns like "clusters/*/apps", the cluster name is one level up
			if clusterName == "apps" && len(parts) > 1 {
				clusterName = parts[len(parts)-2]
			}
			validClusters = append(validClusters, clusterName)
			if hasAppSubdirs {
				isSubdirs = true
			}
		}
	}

	if len(validClusters) == 0 {
		return nil
	}

	// Verify namespace dir exists
	nsMatches, err := filepath.Glob(filepath.Join(cwd, nsGlob))
	if err != nil || len(nsMatches) == 0 {
		// For patterns where nsGlob has *, check with cluster name substituted
		if !strings.Contains(nsGlob, "*") {
			nsPath := filepath.Join(cwd, nsGlob)
			if _, statErr := os.Stat(nsPath); statErr != nil {
				return nil
			}
		} else {
			return nil
		}
	}

	// Convert glob patterns to template patterns
	clusterTemplate := globToTemplate(clusterGlob)
	nsTemplate := globToTemplate(nsGlob)

	return &detectedStructure{
		clusterDir:    clusterTemplate,
		namespacesDir: nsTemplate,
		subdirs:       isSubdirs,
		clusterNames:  validClusters,
	}
}

// globToTemplate converts a glob pattern to a flaxx template pattern.
// "clusters/*" -> "clusters/{{.Cluster}}"
// "clusters/*-namespaces" -> "clusters/{{.Cluster}}-namespaces"
// "clusters/*/apps" -> "clusters/{{.Cluster}}/apps"
// "apps" -> "apps" (no wildcard)
func globToTemplate(glob string) string {
	parts := strings.Split(glob, "/")
	for i, p := range parts {
		if p == "*" {
			parts[i] = "{{.Cluster}}"
		} else if strings.Contains(p, "*") {
			parts[i] = strings.Replace(p, "*", "{{.Cluster}}", 1)
		}
	}
	return strings.Join(parts, "/")
}

// scanForStructure is a fallback that walks the directory tree looking for
// flux-like files and infers the structure from what it finds.
func scanForStructure(cwd string) (*detectedStructure, error) {
	var clusterDirs []string

	walkErr := filepath.WalkDir(cwd, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}
		if d.IsDir() {
			// Don't recurse too deep
			rel, relErr := filepath.Rel(cwd, path)
			if relErr != nil {
				return nil
			}
			if strings.Count(rel, string(filepath.Separator)) > 4 {
				return filepath.SkipDir
			}
			// Skip hidden dirs and common non-flux dirs
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		if strings.HasSuffix(d.Name(), "-kustomization.yaml") {
			dir := filepath.Dir(path)
			rel, relErr := filepath.Rel(cwd, dir)
			if relErr == nil {
				clusterDirs = append(clusterDirs, rel)
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if len(clusterDirs) == 0 {
		return nil, nil
	}

	// Deduplicate
	seen := make(map[string]bool)
	var unique []string
	for _, d := range clusterDirs {
		if !seen[d] {
			seen[d] = true
			unique = append(unique, d)
		}
	}

	// Use the first found directory to infer the pattern
	// This is best-effort for unusual structures
	first := unique[0]
	return &detectedStructure{
		clusterDir:    first,
		namespacesDir: first, // best guess
		clusterNames:  []string{"(detected from " + first + ")"},
	}, nil
}
