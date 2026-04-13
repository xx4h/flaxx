package cmd

import (
	"strings"
	"testing"
)

func TestVersionCommand(t *testing.T) {
	output, err := executeCommand("version")
	if err != nil {
		t.Fatalf("version command failed: %v", err)
	}

	for _, field := range []string{"Version:", "Commit:", "Date:"} {
		if !strings.Contains(output, field) {
			t.Errorf("output missing %s line", field)
		}
	}
}
