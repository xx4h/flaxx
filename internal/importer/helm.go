package importer

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

// HelmReleaseInfo carries the subset of a decoded Helm release we need in
// order to re-express the app as a HelmRelease manifest.
type HelmReleaseInfo struct {
	Name         string
	Namespace    string
	Revision     int
	ChartName    string
	ChartVersion string
	AppVersion   string
	Sources      []string
	Home         string
	// Values are the user-supplied overrides (equivalent to `helm get values
	// <rel>` without --all). Chart defaults are deliberately excluded so the
	// generated HelmRelease stays small and keeps resolving defaults from
	// the chart itself at reconcile time.
	Values map[string]interface{}
}

// helmReleasePayload is the shape of the JSON that Helm stores inside its
// release secrets. We decode only the fields we care about.
type helmReleasePayload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   int    `json:"version"`
	Chart     struct {
		Metadata struct {
			Name       string   `json:"name"`
			Version    string   `json:"version"`
			AppVersion string   `json:"appVersion"`
			Sources    []string `json:"sources"`
			Home       string   `json:"home"`
		} `json:"metadata"`
	} `json:"chart"`
	// Config is Helm's name for the user-supplied values.
	Config map[string]interface{} `json:"config"`
}

// detectHelmRelease returns the highest-revision Helm release secret for
// the given app/namespace, or nil when no such secret exists.
func detectHelmRelease(ctx context.Context, rest *rest.Config, namespace, app string) (*HelmReleaseInfo, error) {
	cs, err := kubernetes.NewForConfig(rest)
	if err != nil {
		return nil, err
	}

	// Helm 3 secrets have type "helm.sh/release.v1" and are named
	// sh.helm.release.v1.<release>.v<N>.
	secrets, err := cs.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: "type=helm.sh/release.v1",
	})
	if err != nil {
		return nil, err
	}

	var latest *corev1.Secret
	var latestRev int
	prefix := "sh.helm.release.v1." + app + ".v"
	for i := range secrets.Items {
		s := &secrets.Items[i]
		if !strings.HasPrefix(s.Name, prefix) {
			continue
		}
		rev, parseErr := strconv.Atoi(strings.TrimPrefix(s.Name, prefix))
		if parseErr != nil {
			continue
		}
		if latest == nil || rev > latestRev {
			latest = s
			latestRev = rev
		}
	}

	if latest == nil {
		return nil, nil
	}

	payload, err := decodeHelmReleaseSecret(latest.Data["release"])
	if err != nil {
		return nil, fmt.Errorf("decoding release secret %s: %w", latest.Name, err)
	}

	return &HelmReleaseInfo{
		Name:         payload.Name,
		Namespace:    payload.Namespace,
		Revision:     payload.Version,
		ChartName:    payload.Chart.Metadata.Name,
		ChartVersion: payload.Chart.Metadata.Version,
		AppVersion:   payload.Chart.Metadata.AppVersion,
		Sources:      payload.Chart.Metadata.Sources,
		Home:         payload.Chart.Metadata.Home,
		Values:       payload.Config,
	}, nil
}

// decodeHelmReleaseSecret mirrors Helm 3's encodeRelease in reverse:
// the secret data is a base64 string wrapping a gzip stream of JSON.
func decodeHelmReleaseSecret(data []byte) (*helmReleasePayload, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty release data")
	}
	decoded, err := base64.StdEncoding.DecodeString(string(data))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(decoded))
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()
	raw, err := io.ReadAll(gz)
	if err != nil {
		return nil, fmt.Errorf("gunzip: %w", err)
	}
	var payload helmReleasePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}
	return &payload, nil
}

// resolveHelmRepoURL picks the best-available URL for the release's chart.
// Precedence: explicit --helm-url flag > local Helm repositories.yaml + index
// cache > error with an actionable message.
//
// Chart metadata sources/home are deliberately ignored here: they typically
// point at the project site (e.g. grafana.com) rather than a Helm chart
// repository URL, so blindly using them would produce a HelmRepository that
// cannot be resolved by Flux.
func resolveHelmRepoURL(release *HelmReleaseInfo, flag string) (string, error) {
	if flag != "" {
		return flag, nil
	}

	url, err := lookupRepoURLFromLocalHelm(release.ChartName, release.ChartVersion)
	if err == nil && url != "" {
		return url, nil
	}

	return "", fmt.Errorf("could not resolve Helm repository URL for chart %q version %q — pass --helm-url=<repo URL> or --non-helm to emit raw manifests instead",
		release.ChartName, release.ChartVersion)
}

// lookupRepoURLFromLocalHelm walks the user's local Helm config to find a
// repository whose cached index lists the requested chart+version. Returns
// empty string if nothing matches (which is not a hard error).
func lookupRepoURLFromLocalHelm(chartName, chartVersion string) (string, error) {
	repoConfig := helmRepositoriesFile()
	repoCache := helmRepositoryCache()
	if repoConfig == "" || repoCache == "" {
		return "", nil
	}

	data, err := os.ReadFile(repoConfig)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var cfg helmRepoConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return "", err
	}

	// Deterministic iteration.
	sort.Slice(cfg.Repositories, func(i, j int) bool {
		return cfg.Repositories[i].Name < cfg.Repositories[j].Name
	})

	for _, repo := range cfg.Repositories {
		indexPath := filepath.Join(repoCache, repo.Name+"-index.yaml")
		raw, err := os.ReadFile(indexPath)
		if err != nil {
			continue
		}
		var idx helmIndex
		if err := yaml.Unmarshal(raw, &idx); err != nil {
			continue
		}
		versions, ok := idx.Entries[chartName]
		if !ok {
			continue
		}
		for _, e := range versions {
			if e.Version == chartVersion {
				return repo.URL, nil
			}
		}
	}

	return "", nil
}

type helmRepoConfig struct {
	Repositories []struct {
		Name string `yaml:"name"`
		URL  string `yaml:"url"`
	} `yaml:"repositories"`
}

type helmIndex struct {
	Entries map[string][]struct {
		Version string `yaml:"version"`
	} `yaml:"entries"`
}

// helmRepositoriesFile returns the path to Helm's repositories.yaml,
// honoring HELM_REPOSITORY_CONFIG and XDG_CONFIG_HOME. Returns "" if the
// user's home directory cannot be resolved.
func helmRepositoriesFile() string {
	if p := os.Getenv("HELM_REPOSITORY_CONFIG"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "helm", "repositories.yaml")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "helm", "repositories.yaml")
}

// helmRepositoryCache returns the path to Helm's repo index cache directory.
func helmRepositoryCache() string {
	if p := os.Getenv("HELM_REPOSITORY_CACHE"); p != "" {
		return p
	}
	if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
		return filepath.Join(xdg, "helm", "repository")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "helm", "repository")
}
