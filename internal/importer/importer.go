// Package importer adopts a running app from a Kubernetes cluster into the
// flaxx repository. It is the first place in flaxx that talks to a live
// Kubernetes API — everything else operates purely on local YAML.
//
// The two shapes of import:
//
//   - Helm-managed: a single HelmRelease + HelmRepository pair, rendered via
//     the same code paths as `flaxx generate --type ext-helm`.
//   - Raw: every user-facing namespaced resource, sanitized and written as
//     plain YAML, wrapped in a core-type Flux Kustomization.
package importer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
)

// Options configures a single import run.
type Options struct {
	App       string
	Cluster   string
	Namespace string // defaults to App when empty

	Kubeconfig string // optional: overrides default kubeconfig loading
	Context    string // optional: kubeconfig context override

	IncludeSecrets bool   // write Secret manifests (plain base64) when set
	ForceNonHelm   bool   // skip Helm detection and always emit raw manifests
	HelmURLFlag    string // fallback URL when Helm detection cannot resolve it

	Force  bool
	DryRun bool

	Cfg      config.Config
	RepoRoot string
}

// Result reports what the import produced.
type Result struct {
	Mode      string // "helm" or "raw"
	Files     []string
	HelmChart string
	HelmURL   string
	Skipped   []string // kinds skipped with a short reason
}

// Run executes the import pipeline.
func Run(opts Options) (*Result, error) {
	if err := validate(&opts); err != nil {
		return nil, err
	}

	paths, err := resolvePaths(opts)
	if err != nil {
		return nil, err
	}

	if !opts.DryRun {
		if prepErr := prepareTarget(paths, opts); prepErr != nil {
			return nil, prepErr
		}
	}

	rest, err := restConfig(opts.Kubeconfig, opts.Context)
	if err != nil {
		return nil, fmt.Errorf("loading kubeconfig: %w", err)
	}

	ctx := context.Background()

	// Helm detection runs first unless the user forced raw-manifest output.
	if !opts.ForceNonHelm {
		release, detectErr := detectHelmRelease(ctx, rest, opts.Namespace, opts.App)
		if detectErr != nil {
			return nil, fmt.Errorf("detecting Helm release: %w", detectErr)
		}
		if release != nil {
			return runHelm(opts, release)
		}
	}

	return runRaw(ctx, opts, paths, rest)
}

func validate(opts *Options) error {
	if opts.App == "" || opts.Cluster == "" {
		return fmt.Errorf("app and cluster are required")
	}
	if opts.Namespace == "" {
		opts.Namespace = opts.App
	}
	if opts.RepoRoot == "" {
		return fmt.Errorf("repo root is required")
	}
	return nil
}

type resolvedPaths struct {
	appClusterDir    string
	appNamespacesDir string
}

func resolvePaths(opts Options) (resolvedPaths, error) {
	genOpts := generator.Options{App: opts.App, Cluster: opts.Cluster, Namespace: opts.Namespace}

	clusterDir, err := generator.ResolvePath(opts.Cfg.Paths.ClusterDir, genOpts)
	if err != nil {
		return resolvedPaths{}, fmt.Errorf("resolving cluster dir: %w", err)
	}
	namespacesDir, err := generator.ResolvePath(opts.Cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return resolvedPaths{}, fmt.Errorf("resolving namespaces dir: %w", err)
	}
	fullClusterDir := filepath.Join(opts.RepoRoot, clusterDir)
	return resolvedPaths{
		appClusterDir:    generator.ResolveAppClusterDir(fullClusterDir, opts.App, opts.Cfg.Paths.ClusterSubdirs),
		appNamespacesDir: filepath.Join(opts.RepoRoot, namespacesDir, opts.App),
	}, nil
}

// prepareTarget honors --force by removing an existing app directory, or
// refuses to proceed if one already exists and --force was not passed.
func prepareTarget(paths resolvedPaths, opts Options) error {
	if opts.Force {
		if _, err := os.Stat(paths.appNamespacesDir); err == nil {
			if rmErr := os.RemoveAll(paths.appNamespacesDir); rmErr != nil {
				return fmt.Errorf("removing existing namespace dir: %w", rmErr)
			}
		}
		if opts.Cfg.Paths.ClusterSubdirs {
			if _, err := os.Stat(paths.appClusterDir); err == nil {
				if rmErr := os.RemoveAll(paths.appClusterDir); rmErr != nil {
					return fmt.Errorf("removing existing cluster dir: %w", rmErr)
				}
			}
		}
		return nil
	}
	if err := generator.CheckNotExists(paths.appNamespacesDir); err != nil {
		return err
	}
	if opts.Cfg.Paths.ClusterSubdirs {
		if err := generator.CheckNotExists(paths.appClusterDir); err != nil {
			return err
		}
	}
	return nil
}

// runHelm emits a HelmRelease + HelmRepository by delegating to the generator,
// mirroring `flaxx generate --type ext-helm` (or `--type ext-oci` when the
// source is an OCI registry).
func runHelm(opts Options, release *HelmReleaseInfo) (*Result, error) {
	repoURL, err := resolveHelmRepoURL(release, opts.HelmURLFlag)
	if err != nil {
		return nil, err
	}

	genResult, err := generator.Run(opts.Cfg, generator.Options{
		App:         opts.App,
		Cluster:     opts.Cluster,
		Namespace:   opts.Namespace,
		Type:        helmTypeFromURL(repoURL),
		HelmURL:     repoURL,
		HelmChart:   release.ChartName,
		HelmVersion: release.ChartVersion,
		HelmValues:  release.Values,
		DryRun:      opts.DryRun,
		// Raw side doesn't skip; Helm scaffolding goes into fresh dirs.
	}, opts.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("scaffolding Helm release: %w", err)
	}

	return &Result{
		Mode:      "helm",
		Files:     genResult.Files,
		HelmChart: release.ChartName,
		HelmURL:   repoURL,
	}, nil
}

// helmTypeFromURL picks the right generator DeployType based on the
// repository URL scheme. OCI registries need `type: oci` on the generated
// HelmRepository, which the generator only emits when Type == TypeExtOCI.
func helmTypeFromURL(url string) generator.DeployType {
	if strings.HasPrefix(url, "oci://") {
		return generator.TypeExtOCI
	}
	return generator.TypeExtHelm
}

// runRaw discovers every namespaced resource, filters and sanitizes it,
// writes the manifest files, then delegates the Flux scaffolding to the
// generator with SkipExistsCheck=true so it doesn't re-check a dir the
// importer just created.
func runRaw(ctx context.Context, opts Options, paths resolvedPaths, rest restConfigT) (*Result, error) {
	listed, skipped, err := discoverResources(ctx, rest, opts.Namespace, opts.IncludeSecrets)
	if err != nil {
		return nil, fmt.Errorf("discovering resources: %w", err)
	}

	sanitized := make(map[string]string)
	for _, obj := range listed {
		sanitize(obj)
		content, marshalErr := marshalUnstructured(obj)
		if marshalErr != nil {
			return nil, fmt.Errorf("marshalling %s/%s: %w", obj.GetKind(), obj.GetName(), marshalErr)
		}
		fileName := manifestFilename(obj.GetKind(), obj.GetName())
		sanitized[fileName] = content
	}

	// Write sanitized manifests into the app namespace dir so the generator
	// can reference them from the in-namespace kustomization.yaml.
	if !opts.DryRun {
		if mkErr := os.MkdirAll(paths.appNamespacesDir, 0o755); mkErr != nil {
			return nil, fmt.Errorf("creating %s: %w", paths.appNamespacesDir, mkErr)
		}
	}

	fileNames := make([]string, 0, len(sanitized))
	for name := range sanitized {
		fileNames = append(fileNames, name)
	}
	sort.Strings(fileNames)

	var writtenFiles []string
	for _, name := range fileNames {
		full := filepath.Join(paths.appNamespacesDir, name)
		if opts.DryRun {
			fmt.Printf("--- %s ---\n%s\n", full, sanitized[name])
			writtenFiles = append(writtenFiles, full)
			continue
		}
		if wrErr := os.WriteFile(full, []byte(sanitized[name]), 0o644); wrErr != nil { //nolint:gosec // manifest files need to be readable
			return nil, fmt.Errorf("writing %s: %w", full, wrErr)
		}
		writtenFiles = append(writtenFiles, full)
	}

	genResult, err := generator.Run(opts.Cfg, generator.Options{
		App:                      opts.App,
		Cluster:                  opts.Cluster,
		Namespace:                opts.Namespace,
		Type:                     generator.TypeCore,
		PreWrittenNamespaceFiles: fileNames,
		SkipExistsCheck:          true,
		DryRun:                   opts.DryRun,
	}, opts.RepoRoot)
	if err != nil {
		return nil, fmt.Errorf("scaffolding core Kustomization: %w", err)
	}

	combined := append([]string{}, writtenFiles...)
	combined = append(combined, genResult.Files...)

	return &Result{
		Mode:    "raw",
		Files:   combined,
		Skipped: skipped,
	}, nil
}

// manifestFilename produces a stable, filesystem-safe name for a manifest.
// Example: ("Deployment", "grafana-web") → "deployment-grafana-web.yaml".
func manifestFilename(kind, name string) string {
	k := strings.ToLower(kind)
	// ConfigMap → configmap; ServiceAccount → serviceaccount; etc.
	return fmt.Sprintf("%s-%s.yaml", k, name)
}
