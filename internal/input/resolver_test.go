package input

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectInputMode_URLListOnly(t *testing.T) {
	cfg := ResolverConfig{
		UseStdin: true,
	}

	mode := detectInputMode(cfg)
	if mode != InputModeURLList {
		t.Errorf("expected InputModeURLList, got %v", mode)
	}
}

func TestDetectInputMode_ProjectOnly(t *testing.T) {
	cfg := ResolverConfig{
		ProjectURL: "org:test/5",
	}

	mode := detectInputMode(cfg)
	if mode != InputModeProject {
		t.Errorf("expected InputModeProject, got %v", mode)
	}
}

func TestDetectInputMode_Mixed(t *testing.T) {
	cfg := ResolverConfig{
		ProjectURL: "org:test/5",
		UseStdin:   true,
	}

	mode := detectInputMode(cfg)
	if mode != InputModeMixed {
		t.Errorf("expected InputModeMixed, got %v", mode)
	}
}

func TestDetectInputMode_Unknown(t *testing.T) {
	cfg := ResolverConfig{}

	mode := detectInputMode(cfg)
	if mode != InputModeUnknown {
		t.Errorf("expected InputModeUnknown, got %v", mode)
	}
}

func TestValidateConfig_ProjectWithDefaults(t *testing.T) {
	// With defaults, field name and values are optional
	cfg := ResolverConfig{
		ProjectURL:      "org:test/5",
		ProjectMaxItems: 100,
		// ProjectFieldName and ProjectFieldValues can use defaults
	}

	err := validateConfig(cfg)
	if err != nil {
		t.Errorf("expected no error with project using defaults, got %v", err)
	}
}

func TestValidateConfig_ProjectMaxItemsTooLow(t *testing.T) {
	cfg := ResolverConfig{
		ProjectURL:         "org:test/5",
		ProjectFieldName:   "Status",
		ProjectFieldValues: []string{"Done"},
		ProjectMaxItems:    0,
	}

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error when project max items is too low")
	}
}

func TestValidateConfig_ProjectMaxItemsTooHigh(t *testing.T) {
	cfg := ResolverConfig{
		ProjectURL:         "org:test/5",
		ProjectFieldName:   "Status",
		ProjectFieldValues: []string{"Done"},
		ProjectMaxItems:    1001,
	}

	err := validateConfig(cfg)
	if err == nil {
		t.Error("expected error when project max items is too high")
	}
}

func TestValidateConfig_Valid(t *testing.T) {
	cfg := ResolverConfig{
		ProjectURL:         "org:test/5",
		ProjectFieldName:   "Status",
		ProjectFieldValues: []string{"Done"},
		ProjectMaxItems:    100,
	}

	err := validateConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error for valid config: %v", err)
	}
}

func TestValidateConfig_URLListOnly(t *testing.T) {
	cfg := ResolverConfig{
		UseStdin: true,
	}

	err := validateConfig(cfg)
	if err != nil {
		t.Errorf("unexpected error for URL list only config: %v", err)
	}
}

func TestFetchFromURLList_Stdin(t *testing.T) {
	// Create temp file to simulate stdin
	tempFile := createTempFile(t, "https://github.com/test/repo/issues/1\nhttps://github.com/test/repo/issues/2\n")
	defer os.Remove(tempFile)

	// Redirect stdin
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	file, err := os.Open(tempFile)
	if err != nil {
		t.Fatalf("failed to open temp file: %v", err)
	}
	defer file.Close()
	os.Stdin = file

	cfg := ResolverConfig{
		UseStdin: true,
	}

	refs, err := fetchFromURLList(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
}

func TestFetchFromURLList_File(t *testing.T) {
	tempFile := createTempFile(t, "https://github.com/test/repo/issues/1\nhttps://github.com/test/repo/issues/2\n")
	defer os.Remove(tempFile)

	cfg := ResolverConfig{
		URLListPath: tempFile,
	}

	refs, err := fetchFromURLList(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
}

func TestFetchFromURLList_FileNotFound(t *testing.T) {
	cfg := ResolverConfig{
		URLListPath: "/nonexistent/file.txt",
	}

	_, err := fetchFromURLList(cfg)
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestFetchFromURLList_NoSource(t *testing.T) {
	cfg := ResolverConfig{}

	_, err := fetchFromURLList(cfg)
	if err == nil {
		t.Error("expected error when no source specified")
	}
}

func TestDeduplicateRefs(t *testing.T) {
	refs := []IssueRef{
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"}, // Duplicate
		{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"}, // Duplicate again
		{Owner: "test", Repo: "repo", Number: 3, URL: "url3"},
	}

	unique := deduplicateRefs(refs)

	if len(unique) != 3 {
		t.Fatalf("expected 3 unique refs, got %d", len(unique))
	}

	// Check order is preserved
	if unique[0].Number != 1 || unique[1].Number != 2 || unique[2].Number != 3 {
		t.Error("expected deduplication to preserve order")
	}
}

func TestDeduplicateRefs_Empty(t *testing.T) {
	refs := []IssueRef{}
	unique := deduplicateRefs(refs)

	if len(unique) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(unique))
	}
}

func TestDeduplicateRefs_NoDuplicates(t *testing.T) {
	refs := []IssueRef{
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
		{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
		{Owner: "test", Repo: "repo", Number: 3, URL: "url3"},
	}

	unique := deduplicateRefs(refs)

	if len(unique) != 3 {
		t.Fatalf("expected 3 refs, got %d", len(unique))
	}
}

func TestParseFieldValues_CommaSeparated(t *testing.T) {
	raw := "In Progress,Blocked,Done"
	values := ParseFieldValues(raw)

	expected := []string{"In Progress", "Blocked", "Done"}
	if len(values) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(values))
	}

	for i, v := range values {
		if v != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, v)
		}
	}
}

func TestParseFieldValues_WithWhitespace(t *testing.T) {
	raw := " In Progress , Blocked , Done "
	values := ParseFieldValues(raw)

	expected := []string{"In Progress", "Blocked", "Done"}
	if len(values) != len(expected) {
		t.Fatalf("expected %d values, got %d", len(expected), len(values))
	}

	for i, v := range values {
		if v != expected[i] {
			t.Errorf("expected %s at position %d, got %s", expected[i], i, v)
		}
	}
}

func TestParseFieldValues_Empty(t *testing.T) {
	raw := ""
	values := ParseFieldValues(raw)

	if values != nil {
		t.Errorf("expected nil for empty string, got %v", values)
	}
}

func TestParseFieldValues_EmptyAfterSplit(t *testing.T) {
	raw := " , , "
	values := ParseFieldValues(raw)

	if len(values) != 0 {
		t.Errorf("expected 0 values after filtering empty strings, got %d", len(values))
	}
}

func TestParseFieldValues_SingleValue(t *testing.T) {
	raw := "Done"
	values := ParseFieldValues(raw)

	if len(values) != 1 {
		t.Fatalf("expected 1 value, got %d", len(values))
	}

	if values[0] != "Done" {
		t.Errorf("expected 'Done', got %s", values[0])
	}
}

func TestInputMode_String(t *testing.T) {
	tests := []struct {
		mode     InputMode
		expected string
	}{
		{InputModeUnknown, "Unknown"},
		{InputModeURLList, "URL List"},
		{InputModeProject, "Project"},
		{InputModeMixed, "Mixed (Project + URL List)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.mode.String()
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestResolveIssueRefs_NoInput(t *testing.T) {
	cfg := ResolverConfig{}

	_, err := ResolveIssueRefs(context.Background(), cfg, nil)
	if err == nil {
		t.Error("expected error when no input provided")
	}
}

func TestResolveIssueRefs_InvalidProjectConfig(t *testing.T) {
	cfg := ResolverConfig{
		ProjectURL: "org:test/5",
		// Missing required fields
	}

	_, err := ResolveIssueRefs(context.Background(), cfg, nil)
	if err == nil {
		t.Error("expected error for invalid project config")
	}
}

func TestResolveIssueRefs_URLListMode(t *testing.T) {
	tempFile := createTempFile(t, "https://github.com/test/repo/issues/1\nhttps://github.com/test/repo/issues/2\n")
	defer os.Remove(tempFile)

	cfg := ResolverConfig{
		URLListPath: tempFile,
	}

	refs, err := ResolveIssueRefs(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
}

// Helper function to create a temporary file with content
func createTempFile(t *testing.T, content string) string {
	t.Helper()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test-input.txt")

	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	return tmpFile
}
