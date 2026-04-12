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

type Extra struct {
	Meta  Meta
	Dir   string
	Files []string
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
		if err := yaml.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", metaPath, err)
		}

		files, err := listTemplateFiles(extraDir)
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

func listTemplateFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading extra dir %s: %w", dir, err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "_meta.yaml" {
			continue
		}
		files = append(files, entry.Name())
	}
	return files, nil
}
