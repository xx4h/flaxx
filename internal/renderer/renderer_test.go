package renderer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/xx4h/flaxx/internal/checker"
)

func TestMergeMaps_DeepMerge(t *testing.T) {
	a := map[string]any{
		"image": map[string]any{
			"repository": "nginx",
			"tag":        "1.0",
		},
		"replicas": 1,
	}
	b := map[string]any{
		"image": map[string]any{
			"tag":        "2.0",
			"pullPolicy": "Always",
		},
		"service": map[string]any{
			"type": "ClusterIP",
		},
	}
	got := mergeMaps(a, b)

	want := map[string]any{
		"image": map[string]any{
			"repository": "nginx",
			"tag":        "2.0",
			"pullPolicy": "Always",
		},
		"replicas": 1,
		"service": map[string]any{
			"type": "ClusterIP",
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mergeMaps mismatch:\n got: %#v\nwant: %#v", got, want)
	}
}

func TestMergeMaps_ScalarOverwritesMap(t *testing.T) {
	a := map[string]any{"x": map[string]any{"y": 1}}
	b := map[string]any{"x": "scalar"}
	got := mergeMaps(a, b)
	if got["x"] != "scalar" {
		t.Errorf("expected scalar to overwrite map, got %#v", got["x"])
	}
}

func TestResolveValuesFrom_ConfigMap(t *testing.T) {
	dir := t.TempDir()
	cm := `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-values
data:
  values.yaml: |
    replicas: 3
    image:
      tag: v2
`
	if err := os.WriteFile(filepath.Join(dir, "cm.yaml"), []byte(cm), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveValuesFrom(checker.ValuesFromRef{
		Kind: "ConfigMap",
		Name: "my-values",
	}, []string{dir})
	if err != nil {
		t.Fatalf("resolveValuesFrom: %v", err)
	}
	if got["replicas"] != float64(3) && got["replicas"] != 3 {
		t.Errorf("replicas = %#v, want 3", got["replicas"])
	}
	img, ok := got["image"].(map[string]any)
	if !ok {
		t.Fatalf("image = %#v, want map", got["image"])
	}
	if img["tag"] != "v2" {
		t.Errorf("image.tag = %#v, want v2", img["tag"])
	}
}

func TestResolveValuesFrom_CustomKey(t *testing.T) {
	dir := t.TempDir()
	cm := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cfg
data:
  custom.yaml: "foo: bar"
`
	if err := os.WriteFile(filepath.Join(dir, "cfg.yaml"), []byte(cm), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveValuesFrom(checker.ValuesFromRef{
		Kind:      "ConfigMap",
		Name:      "cfg",
		ValuesKey: "custom.yaml",
	}, []string{dir})
	if err != nil {
		t.Fatalf("resolveValuesFrom: %v", err)
	}
	if got["foo"] != "bar" {
		t.Errorf("foo = %#v, want bar", got["foo"])
	}
}

func TestResolveValuesFrom_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := resolveValuesFrom(checker.ValuesFromRef{
		Kind: "ConfigMap",
		Name: "missing",
	}, []string{dir})
	if err == nil {
		t.Fatal("expected error for missing ConfigMap, got nil")
	}
}

func TestResolveValuesFrom_SecretStringData(t *testing.T) {
	dir := t.TempDir()
	sec := `apiVersion: v1
kind: Secret
metadata:
  name: my-secret
stringData:
  values.yaml: |
    secret: hunter2
`
	if err := os.WriteFile(filepath.Join(dir, "sec.yaml"), []byte(sec), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := resolveValuesFrom(checker.ValuesFromRef{
		Kind: "Secret",
		Name: "my-secret",
	}, []string{dir})
	if err != nil {
		t.Fatalf("resolveValuesFrom: %v", err)
	}
	if got["secret"] != "hunter2" {
		t.Errorf("secret = %#v, want hunter2", got["secret"])
	}
}

func TestIsOCI(t *testing.T) {
	cases := []struct {
		name string
		info checker.HelmInfo
		want bool
	}{
		{"explicit type", checker.HelmInfo{RepoType: "oci", RepoURL: "https://x"}, true},
		{"oci scheme", checker.HelmInfo{RepoURL: "oci://reg.example.com/charts"}, true},
		{"https", checker.HelmInfo{RepoURL: "https://charts.example.com"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isOCI(c.info); got != c.want {
				t.Errorf("isOCI = %v, want %v", got, c.want)
			}
		})
	}
}
