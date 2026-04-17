package checker

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// ImageInfo holds a container image reference extracted from a workload resource.
type ImageInfo struct {
	Container string // container name
	Image     string // full image reference (registry/repo:tag)
	Registry  string
	Repo      string
	Tag       string
}

// ImageCheckResult holds the result of checking a single image.
type ImageCheckResult struct {
	ImageInfo
	LatestVersion    string
	AvailableUpdates []string
}

// ScanImages reads YAML files in a directory and extracts container image
// references from Deployment, StatefulSet, and DaemonSet resources.
func ScanImages(dir string) ([]ImageInfo, error) {
	files, err := findYAMLFiles(dir)
	if err != nil {
		return nil, err
	}

	var images []ImageInfo
	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, err)
		}

		found, err := parseWorkloadImages(data)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", filePath, err)
		}
		images = append(images, found...)
	}

	return images, nil
}

// FetchImageTags queries the container registry for available tags of an image,
// returning original tags sorted by semver newest first.
//
// Results are cached via the package-level cache (see SetCache), sharing the
// OCI keyspace with FetchOCIVersions.
func FetchImageTags(info ImageInfo) ([]string, error) {
	tags, err := cachedFetchTags(info.Registry, info.Repo)
	if err != nil {
		return nil, fmt.Errorf("fetching tags for %s/%s: %w", info.Registry, info.Repo, err)
	}

	versions := ParseTaggedVersions(tags)

	result := make([]string, len(versions))
	for i, tv := range versions {
		result[i] = tv.Tag
	}
	return result, nil
}

// CheckImage queries the container registry for available tags and compares
// against the current tag. The filter mode controls which version channels are shown.
func CheckImage(info ImageInfo, mode FilterMode) (*ImageCheckResult, error) {
	tags, err := cachedFetchTags(info.Registry, info.Repo)
	if err != nil {
		return nil, fmt.Errorf("fetching tags for %s/%s: %w", info.Registry, info.Repo, err)
	}

	result := &ImageCheckResult{
		ImageInfo: info,
	}

	versions := ParseTaggedVersions(tags)

	if info.Tag != "" {
		current := ParseVersion(info.Tag)
		if current != nil {
			filtered := FilterTaggedVersions(versions, current, mode)
			if len(filtered) > 0 {
				result.LatestVersion = filtered[0].Tag
			}
			for _, tv := range filtered {
				if tv.Version.GreaterThan(current) {
					result.AvailableUpdates = append(result.AvailableUpdates, tv.Tag)
				}
			}
		}
	}

	return result, nil
}

// ParseImageRef splits a container image reference into registry, repository, and tag.
// Examples:
//
//	nginx:1.25             -> registry-1.docker.io, library/nginx, 1.25
//	org/app:v2             -> registry-1.docker.io, org/app, v2
//	ghcr.io/org/app:1.0    -> ghcr.io, org/app, 1.0
//	reg.io/org/app         -> reg.io, org/app, ""
func ParseImageRef(ref string) (registry, repo, tag string) {
	// Split off tag
	tag = ""
	// Handle digests (@sha256:...)
	if idx := strings.Index(ref, "@"); idx != -1 {
		ref = ref[:idx]
	}
	if idx := strings.LastIndex(ref, ":"); idx != -1 {
		possibleTag := ref[idx+1:]
		// Make sure this isn't a port number (part of registry)
		if !strings.Contains(possibleTag, "/") {
			tag = possibleTag
			ref = ref[:idx]
		}
	}

	// Determine if first component is a registry
	parts := strings.SplitN(ref, "/", 2)
	if len(parts) == 1 {
		// Just a name like "nginx"
		return "registry-1.docker.io", "library/" + parts[0], tag
	}

	first := parts[0]
	if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
		return first, parts[1], tag
	}

	// No dot/colon in first part -> Docker Hub org/repo
	return "registry-1.docker.io", ref, tag
}

// workload is a minimal struct for parsing Kubernetes workload resources.
type workload struct {
	Kind string `yaml:"kind"`
	Spec struct {
		Template struct {
			Spec struct {
				Containers []container `yaml:"containers"`
			} `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

type container struct {
	Name  string `yaml:"name"`
	Image string `yaml:"image"`
}

func parseWorkloadImages(data []byte) ([]ImageInfo, error) {
	var images []ImageInfo
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	for {
		var w workload
		err := decoder.Decode(&w)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch w.Kind {
		case "Deployment", "StatefulSet", "DaemonSet":
		default:
			continue
		}

		for _, c := range w.Spec.Template.Spec.Containers {
			if c.Image == "" {
				continue
			}
			registry, repo, tag := ParseImageRef(c.Image)
			images = append(images, ImageInfo{
				Container: c.Name,
				Image:     c.Image,
				Registry:  registry,
				Repo:      repo,
				Tag:       tag,
			})
		}
	}

	return images, nil
}
