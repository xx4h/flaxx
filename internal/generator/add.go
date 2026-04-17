package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/extras"
)

type AddOptions struct {
	App       string
	Cluster   string
	Namespace string
	Extras    []string
	Sets      map[string]string
	DryRun    bool
}

func RunAdd(cfg config.Config, opts AddOptions, repoRoot string) (*Result, error) {
	if opts.Namespace == "" {
		opts.Namespace = opts.App
	}

	genOpts := Options{App: opts.App, Cluster: opts.Cluster, Namespace: opts.Namespace}

	clusterDir, err := resolvePath(cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return nil, fmt.Errorf("resolving cluster dir: %w", err)
	}
	namespacesDir, err := resolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return nil, fmt.Errorf("resolving namespaces dir: %w", err)
	}

	appClusterDir := ResolveAppClusterDir(filepath.Join(repoRoot, clusterDir), opts.App, cfg.Paths.ClusterSubdirs)
	appNamespacesDir := filepath.Join(repoRoot, namespacesDir, opts.App)

	// Verify the app already exists (check namespaces dir — always has per-app subdirs)
	if !opts.DryRun {
		if _, statErr := os.Stat(appNamespacesDir); os.IsNotExist(statErr) {
			return nil, fmt.Errorf("app directory does not exist: %s", appNamespacesDir)
		}
	}

	if len(opts.Extras) == 0 {
		return nil, fmt.Errorf("at least one --extra is required")
	}

	templatesDir := filepath.Join(repoRoot, cfg.TemplatesDir)
	discovered, err := extras.Discover(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("discovering extras: %w", err)
	}

	var result Result
	var newNsFiles []string

	for _, extraName := range opts.Extras {
		extra := extras.FindByName(discovered, extraName)
		if extra == nil {
			return nil, fmt.Errorf("extra %q not found in %s", extraName, templatesDir)
		}

		extraData := extras.ExtraData{
			App:       opts.App,
			Cluster:   opts.Cluster,
			Namespace: opts.Namespace,
			Vars:      opts.Sets,
		}

		vars, err := extras.ResolveVariables(extra.Meta, extraData)
		if err != nil {
			return nil, fmt.Errorf("resolving variables for extra %q: %w", extraName, err)
		}
		extraData.Vars = vars

		for _, file := range extra.Files {
			filePath := filepath.Join(extra.Dir, file.RelPath)
			content, err := extras.RenderFile(filePath, extraData)
			if err != nil {
				return nil, fmt.Errorf("rendering extra %q file %s: %w", extraName, file.RelPath, err)
			}

			target := appNamespacesDir
			if file.Target == extras.TargetCluster {
				target = appClusterDir
			}

			outPath := filepath.Join(target, file.OutName)
			if err := writeFile(outPath, content, opts.DryRun, &result); err != nil {
				return nil, err
			}
			if file.Target != extras.TargetCluster {
				newNsFiles = append(newNsFiles, file.OutName)
			}
		}
	}

	// Update kustomization.yaml to include new resource files
	if len(newNsFiles) > 0 {
		ksPath := filepath.Join(appNamespacesDir, cfg.Naming.NsKustomization)
		if err := addToKustomization(ksPath, newNsFiles, opts.DryRun, &result); err != nil {
			return nil, err
		}
	}

	return &result, nil
}

func addToKustomization(ksPath string, newFiles []string, dryRun bool, result *Result) error {
	content, err := os.ReadFile(ksPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", ksPath, err)
	}

	existing := string(content)

	// Find which files are not yet listed
	var toAdd []string
	for _, f := range newFiles {
		if !strings.Contains(existing, "- "+f) {
			toAdd = append(toAdd, f)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	// Append new resources at the end
	var builder strings.Builder
	builder.WriteString(strings.TrimRight(existing, "\n"))
	builder.WriteString("\n")
	for _, f := range toAdd {
		builder.WriteString("- ")
		builder.WriteString(f)
		builder.WriteString("\n")
	}

	updated := builder.String()

	rel, err := filepath.Rel(".", ksPath)
	if err != nil {
		rel = ksPath
	}

	if dryRun {
		fmt.Printf("--- %s (updated) ---\n%s\n", rel, updated)
		result.Files = append(result.Files, rel+" (updated)")
		return nil
	}

	if err := os.WriteFile(ksPath, []byte(updated), 0o644); err != nil { //nolint:gosec // kustomization files need to be readable
		return fmt.Errorf("writing %s: %w", ksPath, err)
	}
	result.Files = append(result.Files, rel+" (updated)")

	return nil
}
