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

	// TargetCluster is the target value for extras that go into the cluster directory.
	TargetCluster = "cluster"
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

	fullClusterDir := filepath.Join(repoRoot, clusterDir)
	appClusterDir := ResolveAppClusterDir(fullClusterDir, opts.App, cfg.Paths.ClusterSubdirs)
	appNamespacesDir := filepath.Join(repoRoot, namespacesDir, opts.App)

	if !opts.DryRun {
		if existsErr := checkNotExists(appNamespacesDir); existsErr != nil {
			return nil, existsErr
		}
		// With subdirs, check that the app directory doesn't exist
		// With flat layout, check that the kustomization file doesn't exist
		if cfg.Paths.ClusterSubdirs {
			if existsErr := checkNotExists(appClusterDir); existsErr != nil {
				return nil, existsErr
			}
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

	// Process extras
	extraFiles, extraErr := processExtras(cfg, opts, repoRoot, appClusterDir, appNamespacesDir, &result)
	if extraErr != nil {
		return nil, extraErr
	}

	// Generate namespace files
	if renderErr := renderNamespaceFiles(cfg, tmplData, extraFiles, appNamespacesDir, opts.DryRun, &result); renderErr != nil {
		return nil, renderErr
	}

	// Generate flux kustomization
	if renderErr := renderFluxKustomization(ksData, opts.Type, kustomizationName, appClusterDir, opts.DryRun, &result); renderErr != nil {
		return nil, renderErr
	}

	// Generate type-specific files
	if renderErr := renderTypeFiles(opts.Type, tmplData, helmName, gitName, appClusterDir, opts.DryRun, &result); renderErr != nil {
		return nil, renderErr
	}

	// Auto-update parent kustomization.yaml in the cluster directory
	if !opts.DryRun {
		var clusterFiles []string
		clusterFiles = append(clusterFiles, clusterResourcePath(kustomizationName, opts.App, cfg.Paths.ClusterSubdirs))
		switch opts.Type {
		case TypeCoreHelm, TypeExtHelm, TypeExtOCI:
			clusterFiles = append(clusterFiles, clusterResourcePath(helmName, opts.App, cfg.Paths.ClusterSubdirs))
		case TypeExtGit:
			clusterFiles = append(clusterFiles, clusterResourcePath(gitName, opts.App, cfg.Paths.ClusterSubdirs))
		}
		if updateErr := updateClusterKustomization(fullClusterDir, clusterFiles); updateErr != nil {
			return nil, fmt.Errorf("updating cluster kustomization: %w", updateErr)
		}
	}

	return &result, nil
}

// clusterResourcePath returns the path to reference in the cluster kustomization.yaml.
// With subdirs: "myapp/myapp-kustomization.yaml". Flat: "myapp-kustomization.yaml".
func clusterResourcePath(fileName, app string, subdirs bool) string {
	if subdirs {
		return filepath.Join(app, fileName)
	}
	return fileName
}

// updateClusterKustomization adds resources to the cluster directory's kustomization.yaml,
// creating it if it doesn't exist.
func updateClusterKustomization(clusterDir string, resources []string) error {
	ksPath := filepath.Join(clusterDir, "kustomization.yaml")

	existing := make(map[string]bool)
	var lines []string

	data, err := os.ReadFile(ksPath)
	if err == nil {
		// Parse existing resources
		inResources := false
		for _, line := range strings.Split(string(data), "\n") {
			lines = append(lines, line)
			trimmed := strings.TrimSpace(line)
			if trimmed == "resources:" {
				inResources = true
				continue
			}
			if inResources {
				if strings.HasPrefix(trimmed, "- ") {
					existing[strings.TrimPrefix(trimmed, "- ")] = true
				} else if trimmed != "" {
					inResources = false
				}
			}
		}
	}

	// Find new resources to add
	var toAdd []string
	for _, r := range resources {
		if !existing[r] {
			toAdd = append(toAdd, r)
		}
	}

	if len(toAdd) == 0 {
		return nil
	}

	if len(lines) == 0 {
		// Create new kustomization.yaml
		lines = append(lines, "apiVersion: kustomize.config.k8s.io/v1beta1")
		lines = append(lines, "kind: Kustomization")
		lines = append(lines, "")
		lines = append(lines, "resources:")
	}

	for _, r := range toAdd {
		lines = append(lines, "- "+r)
	}

	// Ensure trailing newline
	content := strings.Join(lines, "\n")
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}

	if mkdirErr := os.MkdirAll(clusterDir, 0o755); mkdirErr != nil {
		return fmt.Errorf("creating directory %s: %w", clusterDir, mkdirErr)
	}

	return os.WriteFile(ksPath, []byte(content), 0o644) //nolint:gosec // kustomization files need to be readable
}

func processExtras(cfg config.Config, opts Options, repoRoot, appClusterDir, appNamespacesDir string, result *Result) ([]string, error) {
	if len(opts.Extras) == 0 {
		return nil, nil
	}

	templatesDir := filepath.Join(repoRoot, cfg.TemplatesDir)
	discovered, err := extras.Discover(templatesDir)
	if err != nil {
		return nil, fmt.Errorf("discovering extras: %w", err)
	}

	var extraFiles []string
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
			if extra.Meta.Target == TargetCluster {
				target = appClusterDir
			}

			outPath := filepath.Join(target, fileName)
			if err := writeFile(outPath, content, opts.DryRun, result); err != nil {
				return nil, err
			}
			if extra.Meta.Target != TargetCluster {
				extraFiles = append(extraFiles, fileName)
			}
		}
	}

	return extraFiles, nil
}

func renderNamespaceFiles(cfg config.Config, tmplData templates.TemplateData, extraFiles []string, appNamespacesDir string, dryRun bool, result *Result) error {
	nsContent, err := templates.RenderNamespace(tmplData)
	if err != nil {
		return err
	}
	if writeErr := writeFile(filepath.Join(appNamespacesDir, cfg.Naming.Namespace), nsContent, dryRun, result); writeErr != nil {
		return writeErr
	}

	nsKsContent, ksErr := templates.RenderNsKustomization(extraFiles)
	if ksErr != nil {
		return ksErr
	}
	return writeFile(filepath.Join(appNamespacesDir, cfg.Naming.NsKustomization), nsKsContent, dryRun, result)
}

func renderFluxKustomization(ksData templates.KustomizationData, deployType DeployType, kustomizationName, appClusterDir string, dryRun bool, result *Result) error {
	isDual := deployType == TypeExtGit
	ksContent, err := templates.RenderFluxKustomization(ksData, isDual)
	if err != nil {
		return err
	}
	return writeFile(filepath.Join(appClusterDir, kustomizationName), ksContent, dryRun, result)
}

func renderTypeFiles(deployType DeployType, tmplData templates.TemplateData, helmName, gitName, appClusterDir string, dryRun bool, result *Result) error {
	switch deployType {
	case TypeCoreHelm, TypeExtHelm, TypeExtOCI:
		helmContent, err := templates.RenderHelmFile(tmplData)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(appClusterDir, helmName), helmContent, dryRun, result)
	case TypeExtGit:
		gitContent, err := templates.RenderGitRepository(tmplData)
		if err != nil {
			return err
		}
		return writeFile(filepath.Join(appClusterDir, gitName), gitContent, dryRun, result)
	}
	return nil
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

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // config files need to be readable
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

// ResolvePath is the exported version for use by other commands.
func ResolvePath(pattern string, opts Options) (string, error) {
	return resolveTemplate(pattern, opts)
}

// ResolveAppClusterDir returns the directory where cluster-level files for an app
// are stored. With subdirs=true, files go into a per-app subdirectory. With
// subdirs=false (flat layout), files go directly into the cluster directory.
func ResolveAppClusterDir(base, app string, subdirs bool) string {
	if subdirs {
		return filepath.Join(base, app)
	}
	return base
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
