package cmd

import (
	"reflect"
	"testing"
)

func TestCopilotChildEnv_RemovesAuthenticationTokens(t *testing.T) {
	parent := []string{
		"PATH=/usr/bin",
		"GITHUB_TOKEN=data-token",
		"GH_TOKEN=gh-token",
		"COPILOT_GITHUB_TOKEN=copilot-token",
		"COPILOT_CLI_PATH=/opt/copilot",
	}
	want := []string{"PATH=/usr/bin", "COPILOT_CLI_PATH=/opt/copilot"}

	got := copilotChildEnv(parent)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("copilotChildEnv() = %q, want %q", got, want)
	}
	if parent[1] != "GITHUB_TOKEN=data-token" {
		t.Fatal("copilotChildEnv mutated the parent environment")
	}
}
