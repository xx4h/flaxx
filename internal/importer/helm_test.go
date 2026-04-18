package importer

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// encodeHelmRelease is the inverse of decodeHelmReleaseSecret, used to build
// realistic test fixtures.
func encodeHelmRelease(t *testing.T, payload helmReleasePayload) []byte {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var buf bytes.Buffer
	gw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := gw.Write(raw); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return []byte(base64.StdEncoding.EncodeToString(buf.Bytes()))
}

func TestDecodeHelmReleaseSecret(t *testing.T) {
	original := helmReleasePayload{
		Name:      "grafana",
		Namespace: "monitoring",
		Version:   3,
		Config: map[string]interface{}{
			"replicaCount": float64(3),
			"image": map[string]interface{}{
				"tag": "v11.2.0",
			},
		},
	}
	original.Chart.Metadata.Name = "grafana"
	original.Chart.Metadata.Version = "8.5.7"
	original.Chart.Metadata.AppVersion = "11.2.0"

	encoded := encodeHelmRelease(t, original)

	got, err := decodeHelmReleaseSecret(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Name != original.Name {
		t.Errorf("Name = %q, want %q", got.Name, original.Name)
	}
	if got.Version != original.Version {
		t.Errorf("Version = %d, want %d", got.Version, original.Version)
	}
	if got.Chart.Metadata.Version != "8.5.7" {
		t.Errorf("chart version = %q, want 8.5.7", got.Chart.Metadata.Version)
	}
	if got.Config["replicaCount"] != float64(3) {
		t.Errorf("Config.replicaCount = %v, want 3", got.Config["replicaCount"])
	}
	imgMap, ok := got.Config["image"].(map[string]interface{})
	if !ok {
		t.Fatalf("Config.image is not a map: %T", got.Config["image"])
	}
	if imgMap["tag"] != "v11.2.0" {
		t.Errorf("Config.image.tag = %v, want v11.2.0", imgMap["tag"])
	}
}

func TestDecodeHelmReleaseSecret_Empty(t *testing.T) {
	if _, err := decodeHelmReleaseSecret(nil); err == nil {
		t.Error("expected error for empty data")
	}
}

func TestDecodeHelmReleaseSecret_BadBase64(t *testing.T) {
	if _, err := decodeHelmReleaseSecret([]byte("!!!not base64!!!")); err == nil {
		t.Error("expected error for invalid base64")
	}
}

func TestResolveHelmRepoURL_FlagTakesPrecedence(t *testing.T) {
	rel := &HelmReleaseInfo{ChartName: "grafana", ChartVersion: "8.5.7"}
	url, err := resolveHelmRepoURL(rel, "https://explicit.example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://explicit.example.com" {
		t.Errorf("url = %q, want https://explicit.example.com", url)
	}
}

func TestResolveHelmRepoURL_LocalLookup(t *testing.T) {
	tmp := t.TempDir()
	configDir := filepath.Join(tmp, "config")
	cacheDir := filepath.Join(tmp, "cache")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatal(err)
	}

	repositoriesYAML := `apiVersion: ""
generated: "0001-01-01T00:00:00Z"
repositories:
  - name: grafana
    url: https://grafana.github.io/helm-charts
`
	if err := os.WriteFile(filepath.Join(configDir, "repositories.yaml"), []byte(repositoriesYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	// A minimal Helm index listing one chart+version.
	indexYAML := `apiVersion: v1
entries:
  grafana:
    - name: grafana
      version: 8.5.7
    - name: grafana
      version: 8.5.6
`
	if err := os.WriteFile(filepath.Join(cacheDir, "grafana-index.yaml"), []byte(indexYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(configDir, "repositories.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", cacheDir)

	rel := &HelmReleaseInfo{ChartName: "grafana", ChartVersion: "8.5.7"}
	url, err := resolveHelmRepoURL(rel, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url != "https://grafana.github.io/helm-charts" {
		t.Errorf("url = %q, want https://grafana.github.io/helm-charts", url)
	}
}

func TestResolveHelmRepoURL_NoMatchErrors(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HELM_REPOSITORY_CONFIG", filepath.Join(tmp, "nonexistent.yaml"))
	t.Setenv("HELM_REPOSITORY_CACHE", tmp)

	rel := &HelmReleaseInfo{ChartName: "does-not-exist", ChartVersion: "0.0.0"}
	if _, err := resolveHelmRepoURL(rel, ""); err == nil {
		t.Error("expected error when nothing matches")
	}
}
