package importer

import (
	"testing"

	"github.com/xx4h/flaxx/internal/generator"
)

func TestHelmTypeFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want generator.DeployType
	}{
		{"https://grafana.github.io/helm-charts", generator.TypeExtHelm},
		{"http://charts.example.com", generator.TypeExtHelm},
		{"oci://ghcr.io/example/charts", generator.TypeExtOCI},
		{"oci://quay.io/cilium-charts", generator.TypeExtOCI},
	}
	for _, tc := range tests {
		t.Run(tc.url, func(t *testing.T) {
			if got := helmTypeFromURL(tc.url); got != tc.want {
				t.Errorf("helmTypeFromURL(%q) = %v, want %v", tc.url, got, tc.want)
			}
		})
	}
}
