package projects

import (
	"strings"
	"testing"
)

// TestConvertFieldFiltersToQueryString_Empty tests empty filter array
func TestConvertFieldFiltersToQueryString_Empty(t *testing.T) {
	result := ConvertFieldFiltersToQueryString([]FieldFilter{})

	if result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// TestConvertFieldFiltersToQueryString_Single tests single filter with single value
func TestConvertFieldFiltersToQueryString_Single(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"Blocked"}},
	}

	result := ConvertFieldFiltersToQueryString(filters)
	expected := "Status:Blocked"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestConvertFieldFiltersToQueryString_MultipleValues tests single filter with multiple values
func TestConvertFieldFiltersToQueryString_MultipleValues(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"Blocked", "Done"}},
	}

	result := ConvertFieldFiltersToQueryString(filters)
	expected := "Status:Blocked,Done"

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestConvertFieldFiltersToQueryString_WithSpaces tests values containing spaces
func TestConvertFieldFiltersToQueryString_WithSpaces(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"In Progress", "Not Started"}},
	}

	result := ConvertFieldFiltersToQueryString(filters)
	expected := `Status:"In Progress","Not Started"`

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestConvertFieldFiltersToQueryString_MultipleFields tests multiple fields
func TestConvertFieldFiltersToQueryString_MultipleFields(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"Blocked"}},
		{FieldName: "Priority", Values: []string{"High", "Critical"}},
	}

	result := ConvertFieldFiltersToQueryString(filters)

	// Should contain both filters
	if !strings.Contains(result, "Status:Blocked") {
		t.Error("missing Status:Blocked")
	}
	if !strings.Contains(result, "Priority:High,Critical") {
		t.Error("missing Priority:High,Critical")
	}
	if !strings.Contains(result, " ") {
		t.Error("filters should be space-separated")
	}
}

// TestConvertFieldFiltersToQueryString_MixedSpaces tests mix of values with and without spaces
func TestConvertFieldFiltersToQueryString_MixedSpaces(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"Blocked", "In Progress"}},
		{FieldName: "type", Values: []string{"Epic", "Initiative"}},
	}

	result := ConvertFieldFiltersToQueryString(filters)

	// Status should have quotes around "In Progress" but not "Blocked"
	if !strings.Contains(result, `Status:Blocked,"In Progress"`) {
		t.Errorf("expected mixed quoting in Status field, got %q", result)
	}
	// type should have no quotes
	if !strings.Contains(result, "type:Epic,Initiative") {
		t.Errorf("expected unquoted type field, got %q", result)
	}
}

// TestConvertFieldFiltersToQueryString_QuoteEscaping tests escaping quotes in values
func TestConvertFieldFiltersToQueryString_QuoteEscaping(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "label", Values: []string{`bug "critical"`}},
	}

	result := ConvertFieldFiltersToQueryString(filters)
	expected := `label:"bug \"critical\""`

	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestFormatFilterSummary_Empty tests formatting empty filter list
func TestFormatFilterSummary_Empty(t *testing.T) {
	summary := FormatFilterSummary([]FieldFilter{})

	if summary != "no filters" {
		t.Errorf("expected 'no filters', got '%s'", summary)
	}
}

// TestFormatFilterSummary_SingleFilter tests formatting single filter
func TestFormatFilterSummary_SingleFilter(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"Blocked"}},
	}

	summary := FormatFilterSummary(filters)

	expected := "Status=[Blocked]"
	if summary != expected {
		t.Errorf("expected '%s', got '%s'", expected, summary)
	}
}

// TestFormatFilterSummary_MultipleFilters tests formatting multiple filters
func TestFormatFilterSummary_MultipleFilters(t *testing.T) {
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"Blocked"}},
		{FieldName: "Priority", Values: []string{"High", "Critical"}},
	}

	summary := FormatFilterSummary(filters)

	// Should contain both filters with AND
	if !containsString(summary, "Status=[Blocked]") {
		t.Errorf("expected summary to contain 'Status=[Blocked]', got '%s'", summary)
	}
	if !containsString(summary, "Priority=[High, Critical]") {
		t.Errorf("expected summary to contain 'Priority=[High, Critical]', got '%s'", summary)
	}
	if !containsString(summary, "AND") {
		t.Errorf("expected summary to contain 'AND', got '%s'", summary)
	}
}

// Helper function to check if a string contains a substring (case-sensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
