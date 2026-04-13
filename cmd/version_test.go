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

	if !strings.Contains(output, "Version:") {
		t.Error("output missing Version: line")
	}
	if !strings.Contains(output, "Commit:") {
		t.Error("output missing Commit: line")
	}
	if !strings.Contains(output, "Date:") {
		t.Error("output missing Date: line")
	}
	if !strings.Contains(output, "dev") {
		t.Error("output should contain default 'dev' value")
	}
}
