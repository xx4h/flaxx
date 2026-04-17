package extractor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/extras"
	"github.com/xx4h/flaxx/internal/generator"
)

// ExtractOptions configures a single extraction run.
type ExtractOptions struct {
	App            string
	Cluster        string
	Namespace      string // defaults to App if empty
	TemplateName   string
	Description    string
	IncludeCluster bool
	Interactive    bool
	Force          bool
	DryRun         bool

	Cfg      config.Config
	RepoRoot string

	// Stdout receives dry-run output and progress messages. Defaults to os.Stdout.
	Stdout io.Writer
	// Prompter is used when Interactive is true.
	Prompter Prompter
}

// ExtractResult reports what was produced.
type ExtractResult struct {
	TemplateDir string
	Files       []string
	Variables   map[string]extras.Variable
	Target      string
}

// Candidate is a single detected literal value proposed as a template variable.
type Candidate struct {
	Name        string // variable name (e.g. "chart_version")
	Value       string // literal to replace in file content
	Default     string // default value for _meta.yaml; may contain template refs
	Description string
	// origin identifies which detector produced the candidate (for ordering
	// ambiguous duplicates predictably).
	origin string
}

// Extract runs the extraction pipeline and writes the resulting template.
func Extract(opts ExtractOptions) (*ExtractResult, error) {
	if err := validateOptions(&opts); err != nil {
		return nil, err
	}

	paths, err := resolvePaths(opts)
	if err != nil {
		return nil, err
	}

	if _, statErr := os.Stat(paths.appNamespacesDir); statErr != nil {
		return nil, fmt.Errorf("app namespace directory not found: %s", paths.appNamespacesDir)
	}

	if _, statErr := os.Stat(paths.templateDir); statErr == nil && !opts.Force && !opts.DryRun {
		return nil, fmt.Errorf("template %q already exists at %s (use --force to overwrite)", opts.TemplateName, paths.templateDir)
	}

	nsFiles, clusterFiles, err := collectAppFiles(paths, opts)
	if err != nil {
		return nil, err
	}

	candidates := detectCandidates(paths.appClusterDir, paths.appNamespacesDir, opts)
	if opts.Interactive {
		candidates, err = filterInteractive(candidates, opts.Prompter)
		if err != nil {
			return nil, err
		}
	}

	variables, substitutions, err := buildVariablesAndSubs(candidates, opts)
	if err != nil {
		return nil, err
	}

	rendered := renderCollectedFiles(nsFiles, clusterFiles, substitutions)

	target := extras.TargetNamespaces
	if opts.IncludeCluster {
		target = extras.TargetSplit
	}

	metaYAML, err := buildMetaYAML(opts.TemplateName, opts.Description, target, variables)
	if err != nil {
		return nil, err
	}

	result := &ExtractResult{
		TemplateDir: paths.templateDir,
		Variables:   variables,
		Target:      target,
	}

	if err := writeOutput(paths.templateDir, metaYAML, rendered, opts, result); err != nil {
		return nil, err
	}

	return result, nil
}

func validateOptions(opts *ExtractOptions) error {
	if opts.App == "" || opts.Cluster == "" {
		return fmt.Errorf("app and cluster are required")
	}
	if opts.TemplateName == "" {
		return fmt.Errorf("template name is required")
	}
	if opts.Namespace == "" {
		opts.Namespace = opts.App
	}
	if opts.Stdout == nil {
		opts.Stdout = os.Stdout
	}
	return nil
}

type resolvedPaths struct {
	appClusterDir    string
	appNamespacesDir string
	templateDir      string
}

func resolvePaths(opts ExtractOptions) (resolvedPaths, error) {
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
		templateDir:      filepath.Join(opts.RepoRoot, opts.Cfg.TemplatesDir, opts.TemplateName),
	}, nil
}

func collectAppFiles(paths resolvedPaths, opts ExtractOptions) (map[string]string, map[string]string, error) {
	nsFiles, err := collectFiles(paths.appNamespacesDir, "", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("collecting namespace files: %w", err)
	}

	var clusterFiles map[string]string
	if opts.IncludeCluster {
		clusterFiles, err = collectClusterFiles(paths.appClusterDir, opts.App, opts.Cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("collecting cluster files: %w", err)
		}
	}
	return nsFiles, clusterFiles, nil
}

func buildVariablesAndSubs(candidates []Candidate, opts ExtractOptions) (map[string]extras.Variable, []substitution, error) {
	variables := make(map[string]extras.Variable)
	substitutions := builtinSubstitutions(opts.App, opts.Cluster, opts.Namespace)

	sort.SliceStable(candidates, func(i, j int) bool {
		return len(candidates[i].Value) > len(candidates[j].Value)
	})

	seen := make(map[string]string)
	for _, c := range candidates {
		if c.Value == "" {
			continue
		}
		if existing, ok := seen[c.Name]; ok && existing != c.Value {
			return nil, nil, fmt.Errorf("variable name %q would be used for both %q and %q — rename one with --interactive", c.Name, existing, c.Value)
		}
		seen[c.Name] = c.Value
		if _, ok := variables[c.Name]; !ok {
			variables[c.Name] = extras.Variable{
				Description: c.Description,
				Default:     c.Default,
			}
		}
		substitutions = append(substitutions, substitution{
			Literal: c.Value,
			Expr:    fmt.Sprintf("{{.%s}}", c.Name),
		})
	}
	return variables, substitutions, nil
}

func renderCollectedFiles(nsFiles, clusterFiles map[string]string, subs []substitution) map[string]string {
	rendered := make(map[string]string, len(nsFiles)+len(clusterFiles))
	for name, content := range nsFiles {
		rendered[filepath.Join("__ns__", name)] = applySubstitutions(content, subs)
	}
	for name, content := range clusterFiles {
		rendered[filepath.Join("__cluster__", name)] = applySubstitutions(content, subs)
	}
	return rendered
}

// collectFiles reads every regular top-level file in dir, returning a map of
// filename → content. exclude is a set of filenames to skip.
func collectFiles(dir, prefixFilter string, exclude map[string]bool) (map[string]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := make(map[string]string)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if exclude != nil && exclude[name] {
			continue
		}
		if prefixFilter != "" && !strings.HasPrefix(name, prefixFilter+"-") && !strings.HasPrefix(name, prefixFilter+".") {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, name))
		if readErr != nil {
			return nil, readErr
		}
		out[name] = string(data)
	}
	return out, nil
}

// collectClusterFiles gathers the cluster-dir files that belong to the app.
// In flat layout it filters by app-name prefix; in subdirs layout it reads
// every file inside the per-app subdirectory.
func collectClusterFiles(appClusterDir, app string, cfg config.Config) (map[string]string, error) {
	filter := generator.AppFilter(app, cfg.Paths.ClusterSubdirs)
	return collectFiles(appClusterDir, filter, map[string]bool{"kustomization.yaml": true})
}

type substitution struct {
	Literal string
	Expr    string
	// IsRegex is true when Literal is a regex and Expr is the replacement.
	IsRegex bool
}

// builtinSubstitutions returns word-boundary regex substitutions for
// App/Cluster/Namespace. Built-ins go first in the list so raw values from
// detectors (which are often equal to the app name, e.g. chart_name) don't
// get replaced before the built-in regex runs.
func builtinSubstitutions(app, cluster, namespace string) []substitution {
	var subs []substitution
	// Order: namespace before app (they're often equal) so that when namespace
	// == app, the more specific replacement runs first. For identical values,
	// having two regexes with the same pattern but different expressions would
	// yield the first one — pick {{.Namespace}} for namespace contexts and
	// {{.App}} for others. In practice mechanically replacing them both with
	// the same expression is fine because RenderFile resolves both to the same
	// value. We prefer {{.App}} for the common case (namespace is usually the
	// app).
	uniq := make(map[string]bool)
	add := func(value, expr string) {
		if value == "" || uniq[value] {
			return
		}
		uniq[value] = true
		subs = append(subs, substitution{
			Literal: `\b` + regexp.QuoteMeta(value) + `\b`,
			Expr:    expr,
			IsRegex: true,
		})
	}
	// Prefer .App when namespace == app so generated templates read naturally.
	if namespace != app {
		add(namespace, "{{.Namespace}}")
	}
	add(app, "{{.App}}")
	add(cluster, "{{.Cluster}}")
	return subs
}

func applySubstitutions(content string, subs []substitution) string {
	result := content
	// Built-in (regex) subs first; detector (literal) subs second. Within
	// each group, preserve the provided order.
	var regexSubs, literalSubs []substitution
	for _, s := range subs {
		if s.IsRegex {
			regexSubs = append(regexSubs, s)
		} else {
			literalSubs = append(literalSubs, s)
		}
	}
	// Literal substitutions come before built-ins: a chart version like
	// "1.2.3" or an image tag should become {{.chart_version}}, not get
	// mangled by the App regex first. But literal tags like the app name
	// itself are already excluded from literalSubs via builtinSubstitutions.
	for _, s := range literalSubs {
		result = strings.ReplaceAll(result, s.Literal, s.Expr)
	}
	for _, s := range regexSubs {
		re := regexp.MustCompile(s.Literal)
		result = re.ReplaceAllString(result, s.Expr)
	}
	return result
}

// buildMetaYAML renders _meta.yaml with a stable key order (variables sorted
// alphabetically) so the file is reproducible.
func buildMetaYAML(name, description, target string, variables map[string]extras.Variable) (string, error) {
	var varsNode yaml.Node
	varsNode.Kind = yaml.MappingNode

	names := make([]string, 0, len(variables))
	for n := range variables {
		names = append(names, n)
	}
	sort.Strings(names)

	for _, n := range names {
		v := variables[n]
		keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: n}
		valNode := &yaml.Node{Kind: yaml.MappingNode}
		valNode.Content = []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "description"},
			{Kind: yaml.ScalarNode, Value: v.Description},
			{Kind: yaml.ScalarNode, Value: "default"},
			{Kind: yaml.ScalarNode, Value: v.Default},
		}
		varsNode.Content = append(varsNode.Content, keyNode, valNode)
	}

	root := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "name"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: name},
		&yaml.Node{Kind: yaml.ScalarNode, Value: "description"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: description},
		&yaml.Node{Kind: yaml.ScalarNode, Value: "target"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: target},
	)
	if len(variables) > 0 {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "variables"},
			&varsNode,
		)
	}

	doc := &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	if err := enc.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func writeOutput(templateDir, metaYAML string, rendered map[string]string, opts ExtractOptions, result *ExtractResult) error {
	// Remove existing dir when --force (but only for real runs).
	if opts.Force && !opts.DryRun {
		if _, statErr := os.Stat(templateDir); statErr == nil {
			if rmErr := os.RemoveAll(templateDir); rmErr != nil {
				return fmt.Errorf("removing existing template dir: %w", rmErr)
			}
		}
	}

	writeFile := func(relOut, content string) error {
		full := filepath.Join(templateDir, relOut)
		result.Files = append(result.Files, full)
		if opts.DryRun {
			fmt.Fprintf(opts.Stdout, "--- %s ---\n%s\n", full, content)
			return nil
		}
		if mkErr := os.MkdirAll(filepath.Dir(full), 0o755); mkErr != nil {
			return fmt.Errorf("creating %s: %w", filepath.Dir(full), mkErr)
		}
		//nolint:gosec // template files need to be readable by users
		if wrErr := os.WriteFile(full, []byte(content), 0o644); wrErr != nil {
			return fmt.Errorf("writing %s: %w", full, wrErr)
		}
		return nil
	}

	if err := writeFile("_meta.yaml", metaYAML); err != nil {
		return err
	}

	// Sorted iteration for reproducible Files ordering.
	paths := make([]string, 0, len(rendered))
	for p := range rendered {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		content := rendered[p]
		var outRel string
		switch {
		case strings.HasPrefix(p, "__ns__"+string(filepath.Separator)):
			name := strings.TrimPrefix(p, "__ns__"+string(filepath.Separator))
			if opts.IncludeCluster {
				outRel = filepath.Join("namespaces", name)
			} else {
				outRel = name
			}
		case strings.HasPrefix(p, "__cluster__"+string(filepath.Separator)):
			name := strings.TrimPrefix(p, "__cluster__"+string(filepath.Separator))
			outRel = filepath.Join("cluster", name)
		default:
			outRel = p
		}
		if err := writeFile(outRel, content); err != nil {
			return err
		}
	}

	return nil
}

func filterInteractive(candidates []Candidate, prompter Prompter) ([]Candidate, error) {
	if prompter == nil {
		return nil, fmt.Errorf("interactive mode requires a prompter")
	}
	var kept []Candidate
	for _, c := range candidates {
		choice, err := prompter.Ask(c)
		if err != nil {
			return nil, err
		}
		if !choice.Keep {
			continue
		}
		c.Name = choice.Name
		kept = append(kept, c)
	}
	return kept, nil
}
