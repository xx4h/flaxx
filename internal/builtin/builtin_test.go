package builtin

import (
	"testing"
)

func TestAllReturnsTemplates(t *testing.T) {
	templates := All()
	if len(templates) < 3 {
		t.Errorf("expected at least 3 built-in templates, got %d", len(templates))
	}

	names := make(map[string]bool)
	for _, tmpl := range templates {
		if tmpl.Name == "" {
			t.Error("template has empty name")
		}
		if tmpl.Description == "" {
			t.Errorf("template %q has empty description", tmpl.Name)
		}
		if _, ok := tmpl.Files["_meta.yaml"]; !ok {
			t.Errorf("template %q missing _meta.yaml", tmpl.Name)
		}
		if len(tmpl.Files) < 2 {
			t.Errorf("template %q should have at least _meta.yaml + one resource file", tmpl.Name)
		}
		names[tmpl.Name] = true
	}

	for _, expected := range []string{"vso", "ingress", "multus"} {
		if !names[expected] {
			t.Errorf("missing expected template %q", expected)
		}
	}
}

func TestFindByName(t *testing.T) {
	tmpl := FindByName("vso")
	if tmpl == nil {
		t.Fatal("expected to find vso template")
	}
	if tmpl.Name != "vso" {
		t.Errorf("expected name vso, got %s", tmpl.Name)
	}

	missing := FindByName("nonexistent")
	if missing != nil {
		t.Error("expected nil for nonexistent template")
	}
}
