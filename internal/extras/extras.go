package extras

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

const (
	// TargetCluster places rendered files in the cluster directory.
	TargetCluster = "cluster"
	// TargetNamespaces places rendered files in the namespaces directory (the
	// default when Meta.Target is empty).
	TargetNamespaces = "namespaces"
	// TargetSplit routes files based on which subdirectory ("cluster/" or
	// "namespaces/") they live in inside the template directory.
	TargetSplit = "split"
)

type Variable struct {
	Description string `yaml:"description"`
	Default     string `yaml:"default"`
}

type Meta struct {
	Name        string              `yaml:"name"`
	Description string              `yaml:"description"`
	Target      string              `yaml:"target"`
	Variables   map[string]Variable `yaml:"variables"`
}

// File is one rendered file within an Extra, with its resolved target.
type File struct {
	// RelPath is the file path relative to the Extra.Dir — e.g. "ingress.yaml"
	// or "cluster/myapp-helm.yml" for split extras.
	RelPath string
	// OutName is the filename to write at the target directory.
	OutName string
	// Target is either TargetCluster or TargetNamespaces.
	Target string
}

type Extra struct {
	Meta  Meta
	Dir   string
	Files []File
}

type ExtraData struct {
	App       string
	Cluster   string
	Namespace string
	Vars      map[string]string
}

// Discover finds all extras in the given templates directory.
func Discover(templatesDir string) ([]Extra, error) {
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading templates dir: %w", err)
	}

	var extras []Extra
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		extraDir := filepath.Join(templatesDir, entry.Name())
		metaPath := filepath.Join(extraDir, "_meta.yaml")

		data, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}

		var meta Meta
		if unmarshalErr := yaml.Unmarshal(data, &meta); unmarshalErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", metaPath, unmarshalErr)
		}

		files, err := listTemplateFiles(extraDir, meta.Target)
		if err != nil {
			return nil, err
		}

		extras = append(extras, Extra{
			Meta:  meta,
			Dir:   extraDir,
			Files: files,
		})
	}

	return extras, nil
}

// FindByName finds an extra by name from the discovered list.
func FindByName(extras []Extra, name string) *Extra {
	for i := range extras {
		if extras[i].Meta.Name == name {
			return &extras[i]
		}
	}
	return nil
}

// RenderFile renders a single extra template file with the provided data.
func RenderFile(filePath string, data ExtraData) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", filePath, err)
	}

	// Build template data map with built-in + custom variables
	tmplData := map[string]string{
		"App":       data.App,
		"Cluster":   data.Cluster,
		"Namespace": data.Namespace,
	}
	for k, v := range data.Vars {
		tmplData[k] = v
	}

	// First resolve defaults that reference other variables
	funcMap := template.FuncMap{}
	tmpl, err := template.New(filepath.Base(filePath)).Funcs(funcMap).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("parsing template %s: %w", filePath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, tmplData); err != nil {
		return "", fmt.Errorf("executing template %s: %w", filePath, err)
	}

	return buf.String(), nil
}

// ResolveVariables merges extra meta defaults with user overrides, resolving
// template expressions in defaults.
func ResolveVariables(meta Meta, data ExtraData) (map[string]string, error) {
	vars := make(map[string]string)

	// Start with defaults, resolving any template references
	for name, v := range meta.Variables {
		if override, ok := data.Vars[name]; ok {
			vars[name] = override
			continue
		}
		if v.Default == "" {
			vars[name] = ""
			continue
		}
		// Resolve default value which may contain template references
		resolved, err := resolveValue(v.Default, data)
		if err != nil {
			return nil, fmt.Errorf("resolving default for %s: %w", name, err)
		}
		vars[name] = resolved
	}

	return vars, nil
}

func resolveValue(value string, data ExtraData) (string, error) {
	if !strings.Contains(value, "{{") {
		return value, nil
	}

	tmplData := map[string]string{
		"App":       data.App,
		"Cluster":   data.Cluster,
		"Namespace": data.Namespace,
	}

	t, err := template.New("resolve").Parse(value)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, tmplData); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// DefaultTargetForFile returns the target for a file in an extra that does not
// use split layout.
func DefaultTargetForFile(meta Meta) string {
	if meta.Target == TargetCluster {
		return TargetCluster
	}
	return TargetNamespaces
}

// listTemplateFiles enumerates the rendered files in an extra directory.
// For split extras, it walks cluster/ and namespaces/ subdirectories and
// errors on stray files at the root. For flat extras, it lists files in the
// root directory only and skips _meta.yaml.
func listTemplateFiles(dir, target string) ([]File, error) {
	if target == TargetSplit {
		return listSplitFiles(dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading extra dir %s: %w", dir, err)
	}

	fileTarget := TargetNamespaces
	if target == TargetCluster {
		fileTarget = TargetCluster
	}

	var files []File
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "_meta.yaml" {
			continue
		}
		files = append(files, File{
			RelPath: entry.Name(),
			OutName: entry.Name(),
			Target:  fileTarget,
		})
	}
	return files, nil
}

func listSplitFiles(dir string) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading extra dir %s: %w", dir, err)
	}

	var files []File
	for _, entry := range entries {
		name := entry.Name()
		if name == "_meta.yaml" {
			continue
		}
		if !entry.IsDir() {
			return nil, fmt.Errorf("split extra %s: stray file %q at root (must live in cluster/ or namespaces/)", dir, name)
		}
		var subTarget string
		switch name {
		case "cluster":
			subTarget = TargetCluster
		case "namespaces":
			subTarget = TargetNamespaces
		default:
			return nil, fmt.Errorf("split extra %s: unexpected subdirectory %q (only cluster/ and namespaces/ are allowed)", dir, name)
		}
		subDir := filepath.Join(dir, name)
		subEntries, subErr := os.ReadDir(subDir)
		if subErr != nil {
			return nil, fmt.Errorf("reading %s: %w", subDir, subErr)
		}
		for _, sub := range subEntries {
			if sub.IsDir() {
				return nil, fmt.Errorf("split extra %s: nested directory %q not supported", subDir, sub.Name())
			}
			files = append(files, File{
				RelPath: filepath.Join(name, sub.Name()),
				OutName: sub.Name(),
				Target:  subTarget,
			})
		}
	}
	return files, nil
}
