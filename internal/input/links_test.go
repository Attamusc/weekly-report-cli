package input

import (
	"strings"
	"testing"
)

func TestParseIssueLinks_ValidURLs(t *testing.T) {
	input := `https://github.com/owner/repo/issues/123
https://github.com/another/project/issues/456
https://github.com/test/example/issues/789?ref=branch
https://github.com/test/example/issues/999#issuecomment-123456`

	reader := strings.NewReader(input)
	refs, err := ParseIssueLinks(reader)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []IssueRef{
		{Owner: "owner", Repo: "repo", Number: 123, URL: "https://github.com/owner/repo/issues/123"},
		{Owner: "another", Repo: "project", Number: 456, URL: "https://github.com/another/project/issues/456"},
		{Owner: "test", Repo: "example", Number: 789, URL: "https://github.com/test/example/issues/789"},
		{Owner: "test", Repo: "example", Number: 999, URL: "https://github.com/test/example/issues/999"},
	}

	if len(refs) != len(expected) {
		t.Fatalf("expected %d refs, got %d", len(expected), len(refs))
	}

	for i, ref := range refs {
		if ref != expected[i] {
			t.Errorf("expected ref %d to be %+v, got %+v", i, expected[i], ref)
		}
	}
}

func TestParseIssueLinks_Deduplication(t *testing.T) {
	input := `https://github.com/owner/repo/issues/123
https://github.com/owner/repo/issues/123?query=param
https://github.com/owner/repo/issues/123#comment
https://github.com/owner/repo/issues/456`

	reader := strings.NewReader(input)
	refs, err := ParseIssueLinks(reader)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 unique refs after deduplication, got %d", len(refs))
	}

	// Should keep the first occurrence
	expected := []IssueRef{
		{Owner: "owner", Repo: "repo", Number: 123, URL: "https://github.com/owner/repo/issues/123"},
		{Owner: "owner", Repo: "repo", Number: 456, URL: "https://github.com/owner/repo/issues/456"},
	}

	for i, ref := range refs {
		if ref != expected[i] {
			t.Errorf("expected ref %d to be %+v, got %+v", i, expected[i], ref)
		}
	}
}

func TestParseIssueLinks_EmptyLinesAndComments(t *testing.T) {
	input := `# This is a comment
https://github.com/owner/repo/issues/123

# Another comment
https://github.com/owner/repo/issues/456

# Empty lines should be ignored`

	reader := strings.NewReader(input)
	refs, err := ParseIssueLinks(reader)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
}

func TestParseIssueLinks_InvalidURLs(t *testing.T) {
	testCases := []struct {
		name  string
		input string
	}{
		{
			name:  "not a github URL",
			input: "https://gitlab.com/owner/repo/issues/123",
		},
		{
			name:  "pull request instead of issue",
			input: "https://github.com/owner/repo/pull/123",
		},
		{
			name:  "missing issue number",
			input: "https://github.com/owner/repo/issues/",
		},
		{
			name:  "invalid issue number",
			input: "https://github.com/owner/repo/issues/abc",
		},
		{
			name:  "malformed URL",
			input: "not-a-url",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := strings.NewReader(tc.input)
			_, err := ParseIssueLinks(reader)

			if err == nil {
				t.Errorf("expected error for invalid input: %s", tc.input)
			}
		})
	}
}

func TestParseIssueLinks_EmptyInput(t *testing.T) {
	reader := strings.NewReader("")
	refs, err := ParseIssueLinks(reader)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(refs) != 0 {
		t.Fatalf("expected 0 refs for empty input, got %d", len(refs))
	}
}

func TestIssueRef_String(t *testing.T) {
	ref := IssueRef{
		Owner:  "owner",
		Repo:   "repo",
		Number: 123,
		URL:    "https://github.com/owner/repo/issues/123",
	}

	expected := "owner/repo#123"
	if ref.String() != expected {
		t.Errorf("expected %s, got %s", expected, ref.String())
	}
}
