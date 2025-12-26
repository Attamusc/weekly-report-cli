package projects

import (
	"testing"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

func TestMatchesFilters_NoFilters(t *testing.T) {
	item := ProjectItem{
		ContentType: ContentTypeIssue,
		FieldValues: map[string]FieldValue{
			"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
		},
	}

	// No filters should match everything
	if !MatchesFilters(item, []FieldFilter{}) {
		t.Error("expected item to match when no filters provided")
	}
}

func TestMatchesFilters_SingleFilter_Match(t *testing.T) {
	item := ProjectItem{
		ContentType: ContentTypeIssue,
		FieldValues: map[string]FieldValue{
			"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
		},
	}

	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"In Progress"}},
	}

	if !MatchesFilters(item, filters) {
		t.Error("expected item to match filter")
	}
}

func TestMatchesFilters_SingleFilter_NoMatch(t *testing.T) {
	item := ProjectItem{
		ContentType: ContentTypeIssue,
		FieldValues: map[string]FieldValue{
			"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
		},
	}

	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"Done"}},
	}

	if MatchesFilters(item, filters) {
		t.Error("expected item not to match filter")
	}
}

func TestMatchesFilters_SingleFilter_MultipleValues_OR(t *testing.T) {
	item := ProjectItem{
		ContentType: ContentTypeIssue,
		FieldValues: map[string]FieldValue{
			"Status": {Type: FieldTypeSingleSelect, Text: "Blocked"},
		},
	}

	// Should match any of the values (OR logic)
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"In Progress", "Blocked", "Done"}},
	}

	if !MatchesFilters(item, filters) {
		t.Error("expected item to match one of the filter values")
	}
}

func TestMatchesFilters_MultipleFilters_AND(t *testing.T) {
	item := ProjectItem{
		ContentType: ContentTypeIssue,
		FieldValues: map[string]FieldValue{
			"Status":   {Type: FieldTypeSingleSelect, Text: "In Progress"},
			"Priority": {Type: FieldTypeSingleSelect, Text: "High"},
		},
	}

	// Both filters must match (AND logic)
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"In Progress"}},
		{FieldName: "Priority", Values: []string{"High"}},
	}

	if !MatchesFilters(item, filters) {
		t.Error("expected item to match both filters")
	}
}

func TestMatchesFilters_MultipleFilters_OneDoesNotMatch(t *testing.T) {
	item := ProjectItem{
		ContentType: ContentTypeIssue,
		FieldValues: map[string]FieldValue{
			"Status":   {Type: FieldTypeSingleSelect, Text: "In Progress"},
			"Priority": {Type: FieldTypeSingleSelect, Text: "Low"},
		},
	}

	// Second filter doesn't match
	filters := []FieldFilter{
		{FieldName: "Status", Values: []string{"In Progress"}},
		{FieldName: "Priority", Values: []string{"High"}},
	}

	if MatchesFilters(item, filters) {
		t.Error("expected item not to match when one filter fails")
	}
}

func TestMatchesFilters_FieldDoesNotExist(t *testing.T) {
	item := ProjectItem{
		ContentType: ContentTypeIssue,
		FieldValues: map[string]FieldValue{
			"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
		},
	}

	// Filter for field that doesn't exist
	filters := []FieldFilter{
		{FieldName: "Priority", Values: []string{"High"}},
	}

	if MatchesFilters(item, filters) {
		t.Error("expected item not to match when field doesn't exist")
	}
}

func TestMatchFieldValue_Text_ExactMatch(t *testing.T) {
	value := FieldValue{Type: FieldTypeText, Text: "Bug Fix"}
	filterValues := []string{"Bug Fix"}

	if !matchFieldValue(value, filterValues) {
		t.Error("expected exact text match")
	}
}

func TestMatchFieldValue_Text_ContainsMatch(t *testing.T) {
	value := FieldValue{Type: FieldTypeText, Text: "Implementing new feature"}
	filterValues := []string{"feature"}

	if !matchFieldValue(value, filterValues) {
		t.Error("expected text to contain filter value")
	}
}

func TestMatchFieldValue_Text_CaseInsensitive(t *testing.T) {
	value := FieldValue{Type: FieldTypeText, Text: "IN PROGRESS"}
	filterValues := []string{"in progress"}

	if !matchFieldValue(value, filterValues) {
		t.Error("expected case-insensitive match")
	}
}

func TestMatchFieldValue_SingleSelect_ExactMatch(t *testing.T) {
	value := FieldValue{Type: FieldTypeSingleSelect, Text: "Done"}
	filterValues := []string{"Done"}

	if !matchFieldValue(value, filterValues) {
		t.Error("expected single-select exact match")
	}
}

func TestMatchFieldValue_SingleSelect_CaseInsensitive(t *testing.T) {
	value := FieldValue{Type: FieldTypeSingleSelect, Text: "In Progress"}
	filterValues := []string{"in progress"}

	if !matchFieldValue(value, filterValues) {
		t.Error("expected case-insensitive single-select match")
	}
}

func TestMatchFieldValue_SingleSelect_NoPartialMatch(t *testing.T) {
	value := FieldValue{Type: FieldTypeSingleSelect, Text: "In Progress"}
	filterValues := []string{"Progress"} // Partial should not match for single-select

	if matchFieldValue(value, filterValues) {
		t.Error("single-select should not match partial values")
	}
}

func TestMatchFieldValue_Date(t *testing.T) {
	date := time.Date(2025, 8, 15, 0, 0, 0, 0, time.UTC)
	value := FieldValue{Type: FieldTypeDate, Date: &date}
	filterValues := []string{"2025-08-15"}

	if !matchFieldValue(value, filterValues) {
		t.Error("expected date match")
	}
}

func TestMatchFieldValue_Number(t *testing.T) {
	value := FieldValue{Type: FieldTypeNumber, Number: 5.0}
	filterValues := []string{"5"}

	if !matchFieldValue(value, filterValues) {
		t.Error("expected number match")
	}
}

func TestMatchFieldValue_EmptyFilterValues(t *testing.T) {
	value := FieldValue{Type: FieldTypeText, Text: "Something"}
	filterValues := []string{}

	if matchFieldValue(value, filterValues) {
		t.Error("expected no match when filter values are empty")
	}
}

func TestFilterProjectItems_IssuesOnly(t *testing.T) {
	items := []ProjectItem{
		{
			ContentType: ContentTypeIssue,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
			},
		},
		{
			ContentType: ContentTypePullRequest,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
			},
		},
		{
			ContentType: ContentTypeDraftIssue,
			IssueRef:    nil,
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
			},
		},
	}

	config := ProjectConfig{
		FieldFilters: []FieldFilter{
			{FieldName: "Status", Values: []string{"In Progress"}},
		},
		IncludePRs: false,
	}

	results := FilterProjectItems(items, config)

	// Should only include the issue, not PR or draft
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Number != 1 {
		t.Errorf("expected issue #1, got #%d", results[0].Number)
	}
}

func TestFilterProjectItems_IncludePRs(t *testing.T) {
	items := []ProjectItem{
		{
			ContentType: ContentTypeIssue,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
			},
		},
		{
			ContentType: ContentTypePullRequest,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
			},
		},
	}

	config := ProjectConfig{
		FieldFilters: []FieldFilter{
			{FieldName: "Status", Values: []string{"In Progress"}},
		},
		IncludePRs: true,
	}

	results := FilterProjectItems(items, config)

	// Should include both issue and PR
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
}

func TestFilterProjectItems_FiltersDraftIssues(t *testing.T) {
	items := []ProjectItem{
		{
			ContentType: ContentTypeDraftIssue,
			IssueRef:    nil,
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
			},
		},
	}

	config := ProjectConfig{
		FieldFilters: []FieldFilter{
			{FieldName: "Status", Values: []string{"In Progress"}},
		},
	}

	results := FilterProjectItems(items, config)

	// Draft issues should always be filtered out
	if len(results) != 0 {
		t.Fatalf("expected 0 results (draft issues filtered), got %d", len(results))
	}
}

func TestFilterProjectItems_NoFieldMatch(t *testing.T) {
	items := []ProjectItem{
		{
			ContentType: ContentTypeIssue,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "In Progress"},
			},
		},
		{
			ContentType: ContentTypeIssue,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
			FieldValues: map[string]FieldValue{
				"Status": {Type: FieldTypeSingleSelect, Text: "Blocked"},
			},
		},
	}

	config := ProjectConfig{
		FieldFilters: []FieldFilter{
			{FieldName: "Status", Values: []string{"Done"}},
		},
	}

	results := FilterProjectItems(items, config)

	// No items match the filter
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestFilterProjectItems_NoFilters(t *testing.T) {
	items := []ProjectItem{
		{
			ContentType: ContentTypeIssue,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
			FieldValues: map[string]FieldValue{},
		},
		{
			ContentType: ContentTypePullRequest,
			IssueRef:    &input.IssueRef{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
			FieldValues: map[string]FieldValue{},
		},
	}

	config := ProjectConfig{
		FieldFilters: []FieldFilter{},
		IncludePRs:   false,
	}

	results := FilterProjectItems(items, config)

	// No filters, so should include all issues (but not PRs)
	if len(results) != 1 {
		t.Fatalf("expected 1 result (issue only), got %d", len(results))
	}
}

func TestDeduplicateIssueRefs(t *testing.T) {
	refs := []input.IssueRef{
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"}, // Duplicate
		{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"}, // Duplicate again
	}

	unique := DeduplicateIssueRefs(refs)

	if len(unique) != 2 {
		t.Fatalf("expected 2 unique refs, got %d", len(unique))
	}

	// Check that order is preserved (first occurrence)
	if unique[0].Number != 1 || unique[1].Number != 2 {
		t.Error("expected deduplication to preserve order")
	}
}

func TestDeduplicateIssueRefs_Empty(t *testing.T) {
	refs := []input.IssueRef{}
	unique := DeduplicateIssueRefs(refs)

	if len(unique) != 0 {
		t.Fatalf("expected 0 refs, got %d", len(unique))
	}
}

func TestDeduplicateIssueRefs_NoDeduplicates(t *testing.T) {
	refs := []input.IssueRef{
		{Owner: "test", Repo: "repo", Number: 1, URL: "url1"},
		{Owner: "test", Repo: "repo", Number: 2, URL: "url2"},
		{Owner: "test", Repo: "repo", Number: 3, URL: "url3"},
	}

	unique := DeduplicateIssueRefs(refs)

	if len(unique) != 3 {
		t.Fatalf("expected 3 unique refs, got %d", len(unique))
	}
}
