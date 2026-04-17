package extractor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xx4h/flaxx/internal/checker"
	"github.com/xx4h/flaxx/internal/generator"
)

// detectCandidates runs all detectors and returns an ordered, deduplicated
// candidate list. Order: helm, images, ingress, git — matches the order a
// reader scanning a typical flaxx app would likely care about.
func detectCandidates(appClusterDir, appNamespacesDir string, opts ExtractOptions) []Candidate {
	var out []Candidate
	out = append(out, detectHelm(appClusterDir, opts)...)
	out = append(out, detectImages(appNamespacesDir, opts)...)
	out = append(out, detectIngress(appNamespacesDir, opts)...)
	out = append(out, detectGit(appClusterDir, opts)...)
	out = dedupCandidates(out)
	return templatizeDefaults(out, opts)
}

// templatizeDefaults replaces substrings matching App/Cluster/Namespace inside
// detected defaults with their template references so new instances pick up
// their own App name by default (e.g. "podinfo.example.com" → "{{.App}}.example.com").
func templatizeDefaults(in []Candidate, opts ExtractOptions) []Candidate {
	out := make([]Candidate, len(in))
	for i, c := range in {
		c.Default = substituteBuiltins(c.Default, opts.App, opts.Cluster, opts.Namespace)
		out[i] = c
	}
	return out
}

func substituteBuiltins(s, app, cluster, namespace string) string {
	subs := builtinSubstitutions(app, cluster, namespace)
	return applySubstitutions(s, subs)
}

func detectHelm(appClusterDir string, opts ExtractOptions) []Candidate {
	filter := generator.AppFilter(opts.App, opts.Cfg.Paths.ClusterSubdirs)
	infos, err := checker.ScanAllHelm(appClusterDir, filter)
	if err != nil || len(infos) == 0 {
		return nil
	}

	var out []Candidate
	for _, info := range infos {
		if info.ChartName != "" && info.ChartName != opts.App {
			out = append(out, Candidate{
				Name:        "chart_name",
				Value:       info.ChartName,
				Default:     info.ChartName,
				Description: "Helm chart name",
				origin:      "helm",
			})
		}
		if info.CurrentVersion != "" {
			out = append(out, Candidate{
				Name:        "chart_version",
				Value:       info.CurrentVersion,
				Default:     info.CurrentVersion,
				Description: "Helm chart version",
				origin:      "helm",
			})
		}
		if info.RepoURL != "" {
			out = append(out, Candidate{
				Name:        "helm_url",
				Value:       info.RepoURL,
				Default:     info.RepoURL,
				Description: "Helm repository URL",
				origin:      "helm",
			})
		}
	}
	return out
}

func detectImages(appNamespacesDir string, opts ExtractOptions) []Candidate {
	_ = opts
	images, err := checker.ScanImages(appNamespacesDir)
	if err != nil || len(images) == 0 {
		return nil
	}

	// Sanitize container names for variable keys (foo-bar -> foo_bar).
	var out []Candidate
	for _, img := range images {
		key := sanitizeKey(img.Container)
		if img.Repo != "" {
			out = append(out, Candidate{
				Name:        "image_" + key,
				Value:       buildImageBase(img),
				Default:     buildImageBase(img),
				Description: fmt.Sprintf("Image for container %q", img.Container),
				origin:      "image",
			})
		}
		if img.Tag != "" {
			out = append(out, Candidate{
				Name:        "tag_" + key,
				Value:       img.Tag,
				Default:     img.Tag,
				Description: fmt.Sprintf("Image tag for container %q", img.Container),
				origin:      "image",
			})
		}
	}
	return out
}

func buildImageBase(img checker.ImageInfo) string {
	// Reconstruct the image reference without the tag (what would go before
	// the colon). If the original image had no registry-qualifying dot, use
	// the short form.
	if strings.Contains(img.Image, "/") && !strings.HasPrefix(img.Repo, "library/") {
		parts := strings.SplitN(img.Image, ":", 2)
		return parts[0]
	}
	// Short form: Docker Hub library like "nginx:1.25" → base "nginx"
	parts := strings.SplitN(img.Image, ":", 2)
	return parts[0]
}

// detectIngress scans namespace-dir YAMLs for Ingress resources and returns
// host candidates. Uses a minimal schema to avoid coupling to the checker
// package's private types.
func detectIngress(appNamespacesDir string, opts ExtractOptions) []Candidate {
	_ = opts
	files, err := listYAMLFiles(appNamespacesDir)
	if err != nil {
		return nil
	}

	var hosts []string
	seen := make(map[string]bool)
	for _, fp := range files {
		data, readErr := os.ReadFile(fp)
		if readErr != nil {
			continue
		}
		parsed := parseIngressDocs(data)
		for _, h := range parsed {
			if h == "" || seen[h] {
				continue
			}
			seen[h] = true
			hosts = append(hosts, h)
		}
	}

	if len(hosts) == 0 {
		return nil
	}
	if len(hosts) == 1 {
		return []Candidate{{
			Name:        "ingress_host",
			Value:       hosts[0],
			Default:     hosts[0],
			Description: "Ingress hostname",
			origin:      "ingress",
		}}
	}
	var out []Candidate
	for i, h := range hosts {
		out = append(out, Candidate{
			Name:        fmt.Sprintf("ingress_host_%d", i+1),
			Value:       h,
			Default:     h,
			Description: fmt.Sprintf("Ingress hostname #%d", i+1),
			origin:      "ingress",
		})
	}
	return out
}

func detectGit(appClusterDir string, opts ExtractOptions) []Candidate {
	files, err := listYAMLFiles(appClusterDir)
	if err != nil {
		return nil
	}
	filter := generator.AppFilter(opts.App, opts.Cfg.Paths.ClusterSubdirs)

	var out []Candidate
	for _, fp := range files {
		if filter != "" {
			base := filepath.Base(fp)
			if !strings.HasPrefix(base, filter+"-") && !strings.HasPrefix(base, filter+".") {
				continue
			}
		}
		data, readErr := os.ReadFile(fp)
		if readErr != nil {
			continue
		}
		for _, r := range parseGitRepoDocs(data) {
			if r.URL != "" {
				out = append(out, Candidate{
					Name:        "git_url",
					Value:       r.URL,
					Default:     r.URL,
					Description: "Git repository URL",
					origin:      "git",
				})
			}
			if r.Branch != "" && r.Branch != "main" {
				out = append(out, Candidate{
					Name:        "git_branch",
					Value:       r.Branch,
					Default:     r.Branch,
					Description: "Git branch",
					origin:      "git",
				})
			}
		}
	}
	return out
}

// dedupCandidates drops later occurrences of the same (Name, Value) pair
// while preserving order.
func dedupCandidates(in []Candidate) []Candidate {
	seen := make(map[string]bool)
	var out []Candidate
	for _, c := range in {
		key := c.Name + "\x00" + c.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func sanitizeKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	result := b.String()
	if result == "" {
		return "main"
	}
	return result
}

type ingressDoc struct {
	Kind string `yaml:"kind"`
	Spec struct {
		Rules []struct {
			Host string `yaml:"host"`
		} `yaml:"rules"`
		TLS []struct {
			Hosts []string `yaml:"hosts"`
		} `yaml:"tls"`
	} `yaml:"spec"`
}

func parseIngressDocs(data []byte) []string {
	var hosts []string
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var d ingressDoc
		err := decoder.Decode(&d)
		if err == io.EOF {
			break
		}
		if err != nil {
			return hosts
		}
		if d.Kind != "Ingress" {
			continue
		}
		for _, r := range d.Spec.Rules {
			if r.Host != "" {
				hosts = append(hosts, r.Host)
			}
		}
		for _, t := range d.Spec.TLS {
			hosts = append(hosts, t.Hosts...)
		}
	}
	return hosts
}

type gitRepoDoc struct {
	Kind string `yaml:"kind"`
	Spec struct {
		URL string `yaml:"url"`
		Ref struct {
			Branch string `yaml:"branch"`
		} `yaml:"ref"`
	} `yaml:"spec"`
}

type gitRepoInfo struct {
	URL    string
	Branch string
}

func parseGitRepoDocs(data []byte) []gitRepoInfo {
	var out []gitRepoInfo
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var d gitRepoDoc
		err := decoder.Decode(&d)
		if err == io.EOF {
			break
		}
		if err != nil {
			return out
		}
		if d.Kind != "GitRepository" {
			continue
		}
		out = append(out, gitRepoInfo{URL: d.Spec.URL, Branch: d.Spec.Ref.Branch})
	}
	return out
}

func listYAMLFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			out = append(out, filepath.Join(dir, name))
		}
	}
	return out, nil
}
