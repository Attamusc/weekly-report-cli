package config

import (
	"errors"
	"testing"
)

func TestFromEnvAndFlags_RequiresGitHubToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	_, err := FromEnvAndFlags(ConfigInput{})
	if err == nil {
		t.Fatal("expected error for missing GITHUB_TOKEN")
	}
}

func TestFromEnvAndFlags_DefaultValues(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	cfg, err := FromEnvAndFlags(ConfigInput{
		SinceDays:   7,
		Concurrency: 4,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Models.BaseURL != "https://models.github.ai" {
		t.Errorf("got BaseURL=%q, want default", cfg.Models.BaseURL)
	}
	if cfg.Models.Model != "gpt-5-mini" {
		t.Errorf("got Model=%q, want default", cfg.Models.Model)
	}
	if cfg.Models.Timeout.Seconds() != 120 {
		t.Errorf("got Timeout=%v, want 120s", cfg.Models.Timeout)
	}
	if cfg.SinceDays != 7 {
		t.Errorf("got SinceDays=%d, want 7", cfg.SinceDays)
	}
	if cfg.Concurrency != 4 {
		t.Errorf("got Concurrency=%d, want 4", cfg.Concurrency)
	}
}

func TestFromEnvAndFlags_EnvVarOverrides(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("GITHUB_MODELS_MODEL", "gpt-4o")
	t.Setenv("GITHUB_MODELS_BASE_URL", "https://custom.example.com")
	cfg, err := FromEnvAndFlags(ConfigInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Models.Model != "gpt-4o" {
		t.Errorf("got Model=%q, want gpt-4o", cfg.Models.Model)
	}
	if cfg.Models.BaseURL != "https://custom.example.com" {
		t.Errorf("got BaseURL=%q, want custom URL", cfg.Models.BaseURL)
	}
}

func TestFromEnvAndFlags_NoNotesInversion(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	cfg, _ := FromEnvAndFlags(ConfigInput{NoNotes: true})
	if cfg.Notes {
		t.Error("Notes should be false when NoNotes=true")
	}
	cfg2, _ := FromEnvAndFlags(ConfigInput{NoNotes: false})
	if !cfg2.Notes {
		t.Error("Notes should be true when NoNotes=false")
	}
}

func TestFromEnvAndFlags_QuietOverridesVerbose(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	cfg, _ := FromEnvAndFlags(ConfigInput{Verbose: true, Quiet: true})
	if cfg.Verbose {
		t.Error("Verbose should be false when Quiet is true")
	}
	if !cfg.Quiet {
		t.Error("Quiet should be true")
	}
}

func TestFromEnvAndFlags_DisableSummary(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("DISABLE_SUMMARY", "1")
	cfg, _ := FromEnvAndFlags(ConfigInput{})
	if cfg.Models.Enabled {
		t.Error("Models.Enabled should be false when DISABLE_SUMMARY is set")
	}
	if cfg.Models.Sentiment {
		t.Error("Sentiment should be false when AI is disabled")
	}
}

func TestFromEnvAndFlags_SentimentDisabled(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	cfg, _ := FromEnvAndFlags(ConfigInput{NoSentiment: true})
	if cfg.Models.Sentiment {
		t.Error("Sentiment should be false when NoSentiment=true")
	}
}

func TestFromEnvAndFlags_AITimeout(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("AI_TIMEOUT", "30")
	cfg, err := FromEnvAndFlags(ConfigInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Models.Timeout.Seconds() != 30 {
		t.Errorf("got Timeout=%v, want 30s", cfg.Models.Timeout)
	}
}

func TestFromEnvAndFlags_AITimeout_Invalid(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	t.Setenv("AI_TIMEOUT", "not-a-number")
	_, err := FromEnvAndFlags(ConfigInput{})
	if err == nil {
		t.Error("expected error for invalid AI_TIMEOUT")
	}
}

func TestFromEnvAndFlags_ProjectConfig(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "test-token")
	cfg, err := FromEnvAndFlags(ConfigInput{
		ProjectURL:         "org:my-org/5",
		ProjectField:       "Priority",
		ProjectFieldValues: []string{"High", "Critical"},
		ProjectMaxItems:    50,
		ProjectView:        "Sprint 1",
		ProjectViewID:      "PVT_123",
		ProjectIncludePRs:  true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Project.URL != "org:my-org/5" {
		t.Errorf("got URL=%q, want org:my-org/5", cfg.Project.URL)
	}
	if cfg.Project.FieldName != "Priority" {
		t.Errorf("got FieldName=%q, want Priority", cfg.Project.FieldName)
	}
	if len(cfg.Project.FieldValues) != 2 {
		t.Errorf("got %d field values, want 2", len(cfg.Project.FieldValues))
	}
	if cfg.Project.MaxItems != 50 {
		t.Errorf("got MaxItems=%d, want 50", cfg.Project.MaxItems)
	}
	if cfg.Project.ViewName != "Sprint 1" {
		t.Errorf("got ViewName=%q, want Sprint 1", cfg.Project.ViewName)
	}
	if cfg.Project.ViewID != "PVT_123" {
		t.Errorf("got ViewID=%q, want PVT_123", cfg.Project.ViewID)
	}
	if !cfg.Project.IncludePRs {
		t.Error("IncludePRs should be true")
	}
}

func TestErrNoRows_SentinelError(t *testing.T) {
	if ErrNoRows == nil {
		t.Fatal("ErrNoRows should not be nil")
	}
	wrapped := errors.New("wrapper: " + ErrNoRows.Error())
	if errors.Is(wrapped, ErrNoRows) {
		t.Error("wrapped non-wrapping error should not match via errors.Is")
	}
	// Verify the sentinel is usable directly
	if !errors.Is(ErrNoRows, ErrNoRows) {
		t.Error("ErrNoRows should match itself via errors.Is")
	}
}
