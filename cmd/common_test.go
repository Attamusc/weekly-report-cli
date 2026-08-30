package cmd

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestWriteCopilotFallback_SanitizesAndProvidesAction(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		stage      copilotFailureStage
		want       string
		wantAction string
	}{
		{name: "missing CLI", err: errors.New("exec: copilot: executable file not found in $PATH token-secret"), stage: copilotStageStartup, want: "missing executable", wantAction: "Install GitHub Copilot CLI"},
		{name: "authorization", err: errors.New("401 unauthorized token-secret"), stage: copilotStageInference, want: "authorization", wantAction: "Check Copilot Requests permission"},
		{name: "model", err: errors.New("model claude unavailable token-secret"), stage: copilotStageInference, want: "model availability", wantAction: "Check that the configured Copilot model"},
		{name: "startup", err: errors.New("process crashed token-secret"), stage: copilotStageStartup, want: "startup", wantAction: "Run copilot --version"},
		{name: "inference", err: errors.New("provider payload token-secret"), stage: copilotStageInference, want: "inference", wantAction: "Retry the command"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			var stdout bytes.Buffer
			writeCopilotFallback(&stderr, classifyCopilotFailure(tc.err, tc.stage))

			got := stderr.String()
			if !strings.Contains(got, "using non-AI fallback content") || !strings.Contains(got, tc.want) || !strings.Contains(got, tc.wantAction) {
				t.Fatalf("diagnostic = %q", got)
			}
			if strings.Contains(got, "token-secret") || strings.Contains(got, tc.err.Error()) {
				t.Fatalf("diagnostic leaked raw error: %q", got)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout was modified: %q", stdout.String())
			}
		})
	}
}

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
