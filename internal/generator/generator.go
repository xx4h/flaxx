package generator

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/extras"
	"github.com/xx4h/flaxx/internal/templates"
)

type DeployType string

const (
	TypeCore     DeployType = "core"
	TypeCoreHelm DeployType = "core-helm"
	TypeExtGit   DeployType = "ext-git"
	TypeExtHelm  DeployType = "ext-helm"
	TypeExtOCI   DeployType = "ext-oci"
)

type Options struct {
	App       string
	Cluster   string
	Namespace string
	Type      DeployType

	// Git options
	GitURL    string
	GitBranch string
	GitPath   string
	GitSecret string

	// Helm options
	HelmURL     string
	HelmChart   string
	HelmVersion string

	// Extras
	Extras []string
	Sets   map[string]string

	DryRun bool
}

type Result struct {
	Files []string
}

func Run(cfg config.Config, opts Options, repoRoot string) (*Result, error) {
	if opts.Namespace == "" {
		opts.Namespace = opts.App
	}
	if opts.HelmChart == "" {
		opts.HelmChart = opts.App
	}

	clusterDir, err := resolvePath(cfg.Paths.ClusterDir, opts)
	if err != nil {
		return nil, fmt.Errorf("resolving cluster dir: %w", err)
	}
	namespacesDir, err := resolvePath(cfg.Paths.NamespacesDir, opts)
	if err != nil {
		return nil, fmt.Errorf("resolving namespaces dir: %w", err)
	}

	appClusterDir := filepath.Join(repoRoot, clusterDir, opts.App)
	appNamespacesDir := filepath.Join(repoRoot, namespacesDir, opts.App)

	if !opts.DryRun {
		if err := checkNotExists(appClusterDir); err != nil {
			return nil, err
		}
		if err := checkNotExists(appNamespacesDir); err != nil {
			return nil, err
		}
	}

	// Resolve file names from config
	kustomizationName, err := resolveName(cfg.Naming.Kustomization, opts)
	if err != nil {
		return nil, err
	}
	helmName, err := resolveName(cfg.Naming.Helm, opts)
	if err != nil {
		return nil, err
	}
	gitName, err := resolveName(cfg.Naming.Git, opts)
	if err != nil {
		return nil, err
	}

	// Build template data
	pruneStr := "false"
	if cfg.Defaults.Prune {
		pruneStr = "true"
	}

	tmplData := templates.TemplateData{
		App:         opts.App,
		Cluster:     opts.Cluster,
		Namespace:   opts.Namespace,
		Interval:    cfg.Defaults.Interval,
		Timeout:     cfg.Defaults.Timeout,
		Prune:       pruneStr,
		GitURL:      opts.GitURL,
		GitBranch:   opts.GitBranch,
		GitPath:     opts.GitPath,
		GitSecret:   opts.GitSecret,
		GitName:     deriveGitName(opts.GitURL),
		HelmURL:     opts.HelmURL,
		HelmChart:   opts.HelmChart,
		HelmVersion: opts.HelmVersion,
		HelmOCI:     opts.Type == TypeExtOCI,
	}

	ksData := templates.KustomizationData{
		App:            opts.App,
		Cluster:        opts.Cluster,
		Namespace:      opts.Namespace,
		Interval:       cfg.Defaults.Interval,
		Timeout:        cfg.Defaults.Timeout,
		Prune:          pruneStr,
		NamespacesPath: filepath.Join(namespacesDir, opts.App),
		GitPath:        opts.GitPath,
		GitName:        deriveGitName(opts.GitURL),
	}

	var result Result

	// Discover and process extras
	var extraFiles []string
	if len(opts.Extras) > 0 {
		templatesDir := filepath.Join(repoRoot, cfg.TemplatesDir)
		discovered, err := extras.Discover(templatesDir)
		if err != nil {
			return nil, fmt.Errorf("discovering extras: %w", err)
		}

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

			for _, fileName := range extra.Files {
				filePath := filepath.Join(extra.Dir, fileName)
				content, err := extras.RenderFile(filePath, extraData)
				if err != nil {
					return nil, fmt.Errorf("rendering extra %q file %s: %w", extraName, fileName, err)
				}

				target := appNamespacesDir
				if extra.Meta.Target == "cluster" {
					target = appClusterDir
				}

				outPath := filepath.Join(target, fileName)
				if err := writeFile(outPath, content, opts.DryRun, &result); err != nil {
					return nil, err
				}
				if extra.Meta.Target != "cluster" {
					extraFiles = append(extraFiles, fileName)
				}
			}
		}
	}

	// Generate namespace.yaml
	nsContent, err := templates.RenderNamespace(tmplData)
	if err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(appNamespacesDir, cfg.Naming.Namespace), nsContent, opts.DryRun, &result); err != nil {
		return nil, err
	}

	// Generate kustomization.yaml (namespace-level)
	nsKsContent, err := templates.RenderNsKustomization(extraFiles)
	if err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(appNamespacesDir, cfg.Naming.NsKustomization), nsKsContent, opts.DryRun, &result); err != nil {
		return nil, err
	}

	// Generate flux kustomization
	isDual := opts.Type == TypeExtGit
	ksContent, err := templates.RenderFluxKustomization(ksData, isDual)
	if err != nil {
		return nil, err
	}
	if err := writeFile(filepath.Join(appClusterDir, kustomizationName), ksContent, opts.DryRun, &result); err != nil {
		return nil, err
	}

	// Generate type-specific files
	switch opts.Type {
	case TypeCoreHelm, TypeExtHelm, TypeExtOCI:
		helmContent, err := templates.RenderHelmFile(tmplData)
		if err != nil {
			return nil, err
		}
		if err := writeFile(filepath.Join(appClusterDir, helmName), helmContent, opts.DryRun, &result); err != nil {
			return nil, err
		}
	case TypeExtGit:
		gitContent, err := templates.RenderGitRepository(tmplData)
		if err != nil {
			return nil, err
		}
		if err := writeFile(filepath.Join(appClusterDir, gitName), gitContent, opts.DryRun, &result); err != nil {
			return nil, err
		}
	}

	return &result, nil
}

func writeFile(path, content string, dryRun bool, result *Result) error {
	rel, err := filepath.Rel(".", path)
	if err != nil {
		rel = path
	}
	result.Files = append(result.Files, rel)

	if dryRun {
		fmt.Printf("--- %s ---\n%s\n", rel, content)
		return nil
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

func checkNotExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("directory already exists: %s", path)
	}
	return nil
}

func resolvePath(pattern string, opts Options) (string, error) {
	return resolveTemplate(pattern, opts)
}

func resolveName(pattern string, opts Options) (string, error) {
	return resolveTemplate(pattern, opts)
}

func resolveTemplate(pattern string, opts Options) (string, error) {
	if !strings.Contains(pattern, "{{") {
		return pattern, nil
	}
	data := map[string]string{
		"App":       opts.App,
		"Cluster":   opts.Cluster,
		"Namespace": opts.Namespace,
	}
	t, err := template.New("resolve").Parse(pattern)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func deriveGitName(url string) string {
	if url == "" {
		return ""
	}
	base := filepath.Base(url)
	return strings.TrimSuffix(base, ".git")
}
