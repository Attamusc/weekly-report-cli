package projects

import (
	"testing"
)

func TestParseProjectURL_OrgFullURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ProjectRef
	}{
		{
			name:  "basic org project",
			input: "https://github.com/orgs/my-org/projects/5",
			expected: ProjectRef{
				Type:   ProjectTypeOrg,
				Owner:  "my-org",
				Number: 5,
				URL:    "https://github.com/orgs/my-org/projects/5",
			},
		},
		{
			name:  "org with dashes",
			input: "https://github.com/orgs/acme-corp/projects/123",
			expected: ProjectRef{
				Type:   ProjectTypeOrg,
				Owner:  "acme-corp",
				Number: 123,
				URL:    "https://github.com/orgs/acme-corp/projects/123",
			},
		},
		{
			name:  "org with numbers",
			input: "https://github.com/orgs/org123/projects/1",
			expected: ProjectRef{
				Type:   ProjectTypeOrg,
				Owner:  "org123",
				Number: 1,
				URL:    "https://github.com/orgs/org123/projects/1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseProjectURL(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestParseProjectURL_UserFullURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ProjectRef
	}{
		{
			name:  "basic user project",
			input: "https://github.com/users/johndoe/projects/10",
			expected: ProjectRef{
				Type:   ProjectTypeUser,
				Owner:  "johndoe",
				Number: 10,
				URL:    "https://github.com/users/johndoe/projects/10",
			},
		},
		{
			name:  "user with dashes",
			input: "https://github.com/users/jane-smith/projects/42",
			expected: ProjectRef{
				Type:   ProjectTypeUser,
				Owner:  "jane-smith",
				Number: 42,
				URL:    "https://github.com/users/jane-smith/projects/42",
			},
		},
		{
			name:  "user with numbers",
			input: "https://github.com/users/user123/projects/999",
			expected: ProjectRef{
				Type:   ProjectTypeUser,
				Owner:  "user123",
				Number: 999,
				URL:    "https://github.com/users/user123/projects/999",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseProjectURL(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestParseProjectURL_OrgShortForm(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ProjectRef
	}{
		{
			name:  "basic org short form",
			input: "org:my-org/5",
			expected: ProjectRef{
				Type:   ProjectTypeOrg,
				Owner:  "my-org",
				Number: 5,
				URL:    "https://github.com/orgs/my-org/projects/5",
			},
		},
		{
			name:  "org with dashes short form",
			input: "org:acme-corp/123",
			expected: ProjectRef{
				Type:   ProjectTypeOrg,
				Owner:  "acme-corp",
				Number: 123,
				URL:    "https://github.com/orgs/acme-corp/projects/123",
			},
		},
		{
			name:  "org with numbers short form",
			input: "org:org123/1",
			expected: ProjectRef{
				Type:   ProjectTypeOrg,
				Owner:  "org123",
				Number: 1,
				URL:    "https://github.com/orgs/org123/projects/1",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseProjectURL(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestParseProjectURL_UserShortForm(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected ProjectRef
	}{
		{
			name:  "basic user short form",
			input: "user:johndoe/10",
			expected: ProjectRef{
				Type:   ProjectTypeUser,
				Owner:  "johndoe",
				Number: 10,
				URL:    "https://github.com/users/johndoe/projects/10",
			},
		},
		{
			name:  "user with dashes short form",
			input: "user:jane-smith/42",
			expected: ProjectRef{
				Type:   ProjectTypeUser,
				Owner:  "jane-smith",
				Number: 42,
				URL:    "https://github.com/users/jane-smith/projects/42",
			},
		},
		{
			name:  "user with numbers short form",
			input: "user:user123/999",
			expected: ProjectRef{
				Type:   ProjectTypeUser,
				Owner:  "user123",
				Number: 999,
				URL:    "https://github.com/users/user123/projects/999",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseProjectURL(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.expected {
				t.Errorf("expected %+v, got %+v", tt.expected, result)
			}
		})
	}
}

func TestParseProjectURL_InvalidFormats(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "gitlab URL",
			input: "https://gitlab.com/orgs/my-org/projects/5",
		},
		{
			name:  "wrong path structure",
			input: "https://github.com/my-org/projects/5",
		},
		{
			name:  "missing project number",
			input: "https://github.com/orgs/my-org/projects/",
		},
		{
			name:  "non-numeric project number",
			input: "https://github.com/orgs/my-org/projects/abc",
		},
		{
			name:  "short form without colon",
			input: "org/my-org/5",
		},
		{
			name:  "short form with wrong separator",
			input: "org:my-org:5",
		},
		{
			name:  "malformed URL",
			input: "not-a-url",
		},
		{
			name:  "empty string",
			input: "",
		},
		{
			name:  "whitespace only",
			input: "   ",
		},
		{
			name:  "pull request URL",
			input: "https://github.com/owner/repo/pull/123",
		},
		{
			name:  "issue URL",
			input: "https://github.com/owner/repo/issues/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseProjectURL(tt.input)
			if err == nil {
				t.Errorf("expected error for invalid input: %s", tt.input)
			}
		})
	}
}

func TestParseProjectURL_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "zero project number",
			input:       "org:my-org/0",
			shouldError: true,
			errorMsg:    "project number must be positive",
		},
		{
			name:        "negative project number",
			input:       "org:my-org/-5",
			shouldError: true,
			errorMsg:    "invalid project URL format",
		},
		{
			name:        "very large project number",
			input:       "org:my-org/999999",
			shouldError: false,
		},
		{
			name:        "leading/trailing whitespace in URL",
			input:       "  https://github.com/orgs/my-org/projects/5  ",
			shouldError: false,
		},
		{
			name:        "leading/trailing whitespace in short form",
			input:       "  org:my-org/5  ",
			shouldError: false,
		},
		{
			name:        "org name with underscores",
			input:       "org:my_org/5",
			shouldError: false,
		},
		{
			name:        "org name with dots",
			input:       "org:my.org/5",
			shouldError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseProjectURL(tt.input)

			if tt.shouldError {
				if err == nil {
					t.Errorf("expected error for input: %s", tt.input)
				}
				if tt.errorMsg != "" && err != nil {
					if !contains(err.Error(), tt.errorMsg) {
						t.Errorf("expected error message to contain %q, got %q", tt.errorMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error for input %s: %v", tt.input, err)
				}
				// Verify the canonical URL is generated correctly
				if result.URL == "" {
					t.Errorf("expected non-empty canonical URL for valid input: %s", tt.input)
				}
			}
		})
	}
}

func TestProjectRef_String(t *testing.T) {
	tests := []struct {
		name     string
		ref      ProjectRef
		expected string
	}{
		{
			name: "org project",
			ref: ProjectRef{
				Type:   ProjectTypeOrg,
				Owner:  "my-org",
				Number: 5,
			},
			expected: "organization:my-org/5",
		},
		{
			name: "user project",
			ref: ProjectRef{
				Type:   ProjectTypeUser,
				Owner:  "johndoe",
				Number: 10,
			},
			expected: "user:johndoe/10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ref.String()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestProjectType_String(t *testing.T) {
	tests := []struct {
		name     string
		pt       ProjectType
		expected string
	}{
		{
			name:     "org type",
			pt:       ProjectTypeOrg,
			expected: "organization",
		},
		{
			name:     "user type",
			pt:       ProjectTypeUser,
			expected: "user",
		},
		{
			name:     "invalid type",
			pt:       ProjectType(999),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pt.String()
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && stringContains(s, substr))
}

func stringContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
