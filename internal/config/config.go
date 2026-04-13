package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Defaults struct {
	Interval string `yaml:"interval"`
	Timeout  string `yaml:"timeout"`
	Prune    bool   `yaml:"prune"`
}

type Paths struct {
	ClusterDir     string `yaml:"cluster_dir"`
	NamespacesDir  string `yaml:"namespaces_dir"`
	ClusterSubdirs bool   `yaml:"cluster_subdirs"`
}

type Naming struct {
	Kustomization   string `yaml:"kustomization"`
	Helm            string `yaml:"helm"`
	Git             string `yaml:"git"`
	Namespace       string `yaml:"namespace"`
	NsKustomization string `yaml:"ns_kustomization"`
}

type Config struct {
	Defaults     Defaults `yaml:"defaults"`
	Paths        Paths    `yaml:"paths"`
	Naming       Naming   `yaml:"naming"`
	TemplatesDir string   `yaml:"templates_dir"`
}

func DefaultConfig() Config {
	return Config{
		Defaults: Defaults{
			Interval: "2m",
			Timeout:  "1m",
			Prune:    false,
		},
		Paths: Paths{
			ClusterDir:    "clusters/{{.Cluster}}",
			NamespacesDir: "clusters/{{.Cluster}}-namespaces",
		},
		Naming: Naming{
			Kustomization:   "{{.App}}-kustomization.yaml",
			Helm:            "{{.App}}-helm.yml",
			Git:             "{{.App}}-git.yml",
			Namespace:       "namespace.yaml",
			NsKustomization: "kustomization.yaml",
		},
		TemplatesDir: ".flaxx/templates",
	}
}

func Load(path string) (Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func FindConfigFile(startDir string) string {
	dir := startDir
	for {
		candidate := filepath.Join(dir, ".flaxx.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func LoadFromDir(startDir string) (Config, string, error) {
	path := FindConfigFile(startDir)
	if path == "" {
		return DefaultConfig(), "", nil
	}
	cfg, err := Load(path)
	return cfg, path, err
}
