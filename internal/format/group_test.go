package format

import (
	"testing"
	"time"
)

// --- ParseGroupBy ---

func TestParseGroupBy_ValidFormats(t *testing.T) {
	cases := []struct {
		raw         string
		wantMode    GroupMode
		wantPattern string
	}{
		{"assignee", GroupByAssignee, ""},
		{"label:team-*", GroupByLabel, "team-*"},
		{"label:bug", GroupByLabel, "bug"},
		{"field:Priority", GroupByField, "Priority"},
		{"field:Team", GroupByField, "Team"},
	}
	for _, tc := range cases {
		t.Run(tc.raw, func(t *testing.T) {
			got, err := ParseGroupBy(tc.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Mode != tc.wantMode {
				t.Errorf("mode: got %d, want %d", got.Mode, tc.wantMode)
			}
			if got.Pattern != tc.wantPattern {
				t.Errorf("pattern: got %q, want %q", got.Pattern, tc.wantPattern)
			}
		})
	}
}

func TestParseGroupBy_Invalid(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"unknown", "owner"},
		{"label no pattern", "label:"},
		{"field no name", "field:"},
		{"invalid glob", "label:[["},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseGroupBy(tc.raw)
			if err == nil {
				t.Errorf("expected error for %q, got nil", tc.raw)
			}
		})
	}
}

// --- GroupRows helpers ---

func makeRow(assignees, labels []string, extra map[string]string, days int) Row {
	var td *time.Time
	if days >= 0 {
		t := time.Now().AddDate(0, 0, days)
		td = &t
	}
	return Row{
		EpicTitle:    "issue",
		Assignees:    assignees,
		Labels:       labels,
		ExtraColumns: extra,
		TargetDate:   td,
	}
}

// --- GroupRows by assignee ---

func TestGroupRows_ByAssignee(t *testing.T) {
	rows := []Row{
		makeRow([]string{"alice"}, nil, nil, 5),
		makeRow([]string{"bob"}, nil, nil, 3),
		makeRow([]string{"alice"}, nil, nil, 1),
		makeRow(nil, nil, nil, 2), // unassigned
	}
	cfg := GroupConfig{Mode: GroupByAssignee}
	groups := GroupRows(rows, cfg)

	if len(groups) != 3 {
		t.Fatalf("expected 3 groups, got %d", len(groups))
	}
	// Alphabetical: alice, bob, Unassigned last
	if groups[0].Title != "alice" {
		t.Errorf("expected alice first, got %q", groups[0].Title)
	}
	if groups[1].Title != "bob" {
		t.Errorf("expected bob second, got %q", groups[1].Title)
	}
	if groups[2].Title != "Unassigned" {
		t.Errorf("expected Unassigned last, got %q", groups[2].Title)
	}
	if len(groups[0].Rows) != 2 {
		t.Errorf("alice: expected 2 rows, got %d", len(groups[0].Rows))
	}
}

// --- GroupRows by label ---

func TestGroupRows_ByLabel(t *testing.T) {
	rows := []Row{
		makeRow(nil, []string{"team-backend"}, nil, 1),
		makeRow(nil, []string{"team-frontend"}, nil, 2),
		makeRow(nil, []string{"bug"}, nil, 3), // no match → Other
		makeRow(nil, nil, nil, 4),             // no labels → Other
	}
	cfg := GroupConfig{Mode: GroupByLabel, Pattern: "team-*"}
	groups := GroupRows(rows, cfg)

	titles := make(map[string]int)
	for _, g := range groups {
		titles[g.Title] = len(g.Rows)
	}
	if titles["team-backend"] != 1 {
		t.Errorf("team-backend: want 1 row, got %d", titles["team-backend"])
	}
	if titles["team-frontend"] != 1 {
		t.Errorf("team-frontend: want 1 row, got %d", titles["team-frontend"])
	}
	if titles["Other"] != 2 {
		t.Errorf("Other: want 2 rows, got %d", titles["Other"])
	}
	// Other must be last
	if groups[len(groups)-1].Title != "Other" {
		t.Errorf("Other should be last, got %q", groups[len(groups)-1].Title)
	}
}

// --- GroupRows by field ---

func TestGroupRows_ByField(t *testing.T) {
	rows := []Row{
		makeRow(nil, nil, map[string]string{"Priority": "High"}, 1),
		makeRow(nil, nil, map[string]string{"Priority": "Low"}, 2),
		makeRow(nil, nil, map[string]string{"Priority": "High"}, 3),
		makeRow(nil, nil, nil, 4), // missing extra → Other
	}
	cfg := GroupConfig{Mode: GroupByField, Pattern: "Priority"}
	groups := GroupRows(rows, cfg)

	titles := make(map[string]int)
	for _, g := range groups {
		titles[g.Title] = len(g.Rows)
	}
	if titles["High"] != 2 {
		t.Errorf("High: want 2, got %d", titles["High"])
	}
	if titles["Low"] != 1 {
		t.Errorf("Low: want 1, got %d", titles["Low"])
	}
	if titles["Other"] != 1 {
		t.Errorf("Other: want 1, got %d", titles["Other"])
	}
	if groups[len(groups)-1].Title != "Other" {
		t.Errorf("Other should be last, got %q", groups[len(groups)-1].Title)
	}
}

// --- Fallback always last ---

func TestGroupRows_FallbackLast(t *testing.T) {
	rows := []Row{
		makeRow(nil, nil, nil, 1),                // Unassigned
		makeRow([]string{"z-last"}, nil, nil, 2), // would sort after Unassigned alphabetically
	}
	cfg := GroupConfig{Mode: GroupByAssignee}
	groups := GroupRows(rows, cfg)
	if groups[len(groups)-1].Title != "Unassigned" {
		t.Errorf("Unassigned should be last, got %q", groups[len(groups)-1].Title)
	}
}

// --- Empty rows ---

func TestGroupRows_Empty(t *testing.T) {
	groups := GroupRows(nil, GroupConfig{Mode: GroupByAssignee})
	if groups != nil {
		t.Errorf("expected nil for empty input, got %v", groups)
	}
}
