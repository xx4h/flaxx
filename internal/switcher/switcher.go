// Package switcher migrates existing Deployment/StatefulSet/DaemonSet YAMLs
// between workload kinds, adding or removing kind-specific fields.
package switcher

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xx4h/flaxx/internal/templates"
	"github.com/xx4h/flaxx/internal/yamlutil"
)

type Options struct {
	App         string
	Namespace   string
	TargetKind  string
	ServiceName string
	DryRun      bool
}

type Result struct {
	UpdatedFiles []string
	FromKind     string
	ToKind       string
	RenamedFrom  string
	RenamedTo    string
	Notices      []string
}

// Switch converts the single Deployment/StatefulSet/DaemonSet manifest found
// under dir to the target kind, applying the smart transition matrix.
// workloadMatch is a located workload doc within a parsed file.
type workloadMatch struct {
	filePath string
	docs     []*yaml.Node
	doc      *yaml.Node
	kind     string
}

// Switch converts the single Deployment/StatefulSet/DaemonSet manifest found
// under dir to the target kind, applying the smart transition matrix.
func Switch(dir string, opts Options) (*Result, error) {
	target := templates.NormalizeWorkloadKind(opts.TargetKind)
	if target == "" {
		return nil, fmt.Errorf("invalid target kind %q (want deployment|statefulset|daemonset)", opts.TargetKind)
	}

	m, err := findWorkload(dir)
	if err != nil {
		return nil, err
	}

	result := &Result{FromKind: m.kind, ToKind: target}
	if m.kind == target {
		result.Notices = append(result.Notices, fmt.Sprintf("workload already of kind %s; nothing to do", target))
		return result, nil
	}

	specNode := yamlutil.GetMapValue(m.doc, "spec")
	if specNode == nil {
		return nil, fmt.Errorf("workload in %s has no spec", m.filePath)
	}

	yamlutil.SetOrAddScalar(m.doc, "kind", target)
	yamlutil.SetOrAddScalar(m.doc, "apiVersion", "apps/v1")

	serviceName, derived := resolveServiceName(m.doc, opts)
	applyTransition(specNode, target, serviceName)
	if target == "StatefulSet" && derived {
		result.Notices = append(result.Notices, fmt.Sprintf("serviceName defaulted to %q; pass --service-name to override", serviceName))
	}

	writePath, oldBase := computeWritePath(m, opts, target, result)

	if !opts.DryRun && writePath != m.filePath {
		if _, statErr := os.Stat(writePath); statErr == nil {
			return nil, fmt.Errorf("cannot rename %s → %s: target file already exists", oldBase, filepath.Base(writePath))
		}
	}

	writtenRel, err := yamlutil.WriteDocuments(writePath, m.docs, opts.DryRun)
	if err != nil {
		return nil, err
	}
	result.UpdatedFiles = append(result.UpdatedFiles, writtenRel)

	if !opts.DryRun && writePath != m.filePath {
		if removeErr := os.Remove(m.filePath); removeErr != nil {
			return nil, fmt.Errorf("removing old file %s: %w", m.filePath, removeErr)
		}
		if ksErr := updateKustomization(dir, oldBase, filepath.Base(writePath)); ksErr != nil {
			return nil, fmt.Errorf("updating kustomization.yaml: %w", ksErr)
		}
	}

	return result, nil
}

func findWorkload(dir string) (*workloadMatch, error) {
	files, err := yamlutil.FindYAMLFiles(dir)
	if err != nil {
		return nil, err
	}
	var matches []*workloadMatch
	for _, filePath := range files {
		data, readErr := os.ReadFile(filePath)
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, readErr)
		}
		docs, parseErr := yamlutil.SplitYAMLDocuments(data)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing %s: %w", filePath, parseErr)
		}
		for _, doc := range docs {
			kind := yamlutil.GetScalarValue(doc, "kind")
			if kind == "Deployment" || kind == "StatefulSet" || kind == "DaemonSet" {
				matches = append(matches, &workloadMatch{filePath: filePath, docs: docs, doc: doc, kind: kind})
			}
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no Deployment/StatefulSet/DaemonSet found in %s", dir)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple workloads found in %s; switch supports a single workload per app", dir)
	}
	return matches[0], nil
}

// resolveServiceName returns (name, derived). derived is true when the value
// was inferred (vs. supplied explicitly by the caller).
func resolveServiceName(doc *yaml.Node, opts Options) (string, bool) {
	if opts.ServiceName != "" {
		return opts.ServiceName, false
	}
	name := ""
	if metaNode := yamlutil.GetMapValue(doc, "metadata"); metaNode != nil {
		name = yamlutil.GetScalarValue(metaNode, "name")
	}
	if name == "" {
		name = opts.App
	}
	return name, true
}

// computeWritePath returns the target path and the old basename, plus records
// rename info on result when the filename follows the <app>-<kind>.yaml convention.
func computeWritePath(m *workloadMatch, opts Options, target string, result *Result) (string, string) {
	oldBase := filepath.Base(m.filePath)
	app := opts.App
	if app == "" {
		if metaNode := yamlutil.GetMapValue(m.doc, "metadata"); metaNode != nil {
			app = yamlutil.GetScalarValue(metaNode, "name")
		}
	}
	if app == "" {
		return m.filePath, oldBase
	}
	oldConventional := app + "-" + strings.ToLower(m.kind) + ".yaml"
	if oldBase != oldConventional {
		return m.filePath, oldBase
	}
	newBase := app + "-" + strings.ToLower(target) + ".yaml"
	result.RenamedFrom = oldBase
	result.RenamedTo = newBase
	return filepath.Join(filepath.Dir(m.filePath), newBase), oldBase
}

// applyTransition mutates the spec map to match the target kind.
func applyTransition(spec *yaml.Node, target, serviceName string) {
	switch target {
	case "Deployment":
		ensureReplicas(spec, "1")
		yamlutil.RemoveMapEntry(spec, "serviceName")
		yamlutil.RemoveMapEntry(spec, "volumeClaimTemplates")
		yamlutil.RenameMapKey(spec, "updateStrategy", "strategy")
	case "StatefulSet":
		ensureReplicas(spec, "1")
		ensureServiceName(spec, serviceName)
		ensureEmptyVolumeClaimTemplates(spec)
		yamlutil.RenameMapKey(spec, "strategy", "updateStrategy")
	case "DaemonSet":
		yamlutil.RemoveMapEntry(spec, "replicas")
		yamlutil.RemoveMapEntry(spec, "serviceName")
		yamlutil.RemoveMapEntry(spec, "volumeClaimTemplates")
		yamlutil.RenameMapKey(spec, "strategy", "updateStrategy")
	}
}

func ensureReplicas(spec *yaml.Node, def string) {
	if yamlutil.GetMapValue(spec, "replicas") == nil {
		yamlutil.AddMapEntry(spec, "replicas", def)
		// Tag the scalar as int so it serializes without quotes.
		if v := yamlutil.GetMapValue(spec, "replicas"); v != nil {
			v.Tag = "!!int"
		}
	}
}

func ensureServiceName(spec *yaml.Node, name string) {
	if yamlutil.GetMapValue(spec, "serviceName") == nil {
		yamlutil.AddMapEntry(spec, "serviceName", name)
	}
}

func ensureEmptyVolumeClaimTemplates(spec *yaml.Node) {
	if yamlutil.GetMapValue(spec, "volumeClaimTemplates") == nil {
		yamlutil.AddMapSequence(spec, "volumeClaimTemplates")
	}
}

// updateKustomization rewrites the sibling kustomization.yaml to replace an
// old resource filename with the new one. No-op if the file doesn't exist or
// doesn't reference oldName.
func updateKustomization(dir, oldName, newName string) error {
	ksPath := filepath.Join(dir, "kustomization.yaml")
	data, err := os.ReadFile(ksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	lines := strings.Split(string(data), "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "- "+oldName {
			lines[i] = strings.Replace(line, oldName, newName, 1)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	content := strings.Join(lines, "\n")
	return os.WriteFile(ksPath, []byte(content), 0o644) //nolint:gosec // kustomization files need to be readable
}
