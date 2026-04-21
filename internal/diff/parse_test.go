package diff

import (
	"testing"
)

func TestParseReport(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []PreviousRow
	}{
		{
			name: "valid table with multiple rows",
			input: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Issue One](https://github.com/org/repo/issues/1) | 2024-01-15 | Summary text |
| :yellow_circle: At Risk | [Issue Two](https://github.com/org/repo/issues/2) | TBD | Some update |`,
			want: []PreviousRow{
				{IssueURL: "https://github.com/org/repo/issues/1", StatusEmoji: ":green_circle:", StatusCaption: "On Track", TargetDate: "2024-01-15"},
				{IssueURL: "https://github.com/org/repo/issues/2", StatusEmoji: ":yellow_circle:", StatusCaption: "At Risk", TargetDate: "TBD"},
			},
		},
		{
			name:  "empty input",
			input: "",
			want:  nil,
		},
		{
			name: "header only",
			input: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|`,
			want: nil,
		},
		{
			name: "escaped pipes in title",
			input: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Issue \| With Pipe](https://github.com/org/repo/issues/3) | 2024-02-01 | ok |`,
			want: []PreviousRow{
				{IssueURL: "https://github.com/org/repo/issues/3", StatusEmoji: ":green_circle:", StatusCaption: "On Track", TargetDate: "2024-02-01"},
			},
		},
		{
			name: "malformed rows mixed with valid",
			input: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | [Valid Issue](https://github.com/org/repo/issues/1) | 2024-01-15 | ok |
| not a status | no link here | TBD | bad |
| | | | |
| :red_circle: Blocked | [Other Issue](https://github.com/org/repo/issues/5) | TBD | note |`,
			want: []PreviousRow{
				{IssueURL: "https://github.com/org/repo/issues/1", StatusEmoji: ":green_circle:", StatusCaption: "On Track", TargetDate: "2024-01-15"},
				{IssueURL: "https://github.com/org/repo/issues/5", StatusEmoji: ":red_circle:", StatusCaption: "Blocked", TargetDate: "TBD"},
			},
		},
		{
			name: "no markdown link row skipped",
			input: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :green_circle: On Track | Plain Text Title | 2024-01-15 | ok |`,
			want: nil,
		},
		{
			name: "TBD target date preserved",
			input: `| Status | Initiative/Epic | Target Date | Update |
|--------|-----------------|-------------|--------|
| :yellow_circle: At Risk | [An Issue](https://github.com/org/repo/issues/7) | TBD | details |`,
			want: []PreviousRow{
				{IssueURL: "https://github.com/org/repo/issues/7", StatusEmoji: ":yellow_circle:", StatusCaption: "At Risk", TargetDate: "TBD"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseReport(tc.input)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d rows, want %d; rows: %+v", len(got), len(tc.want), got)
			}
			for i, row := range got {
				w := tc.want[i]
				if row.IssueURL != w.IssueURL {
					t.Errorf("row %d: IssueURL got %q, want %q", i, row.IssueURL, w.IssueURL)
				}
				if row.StatusEmoji != w.StatusEmoji {
					t.Errorf("row %d: StatusEmoji got %q, want %q", i, row.StatusEmoji, w.StatusEmoji)
				}
				if row.StatusCaption != w.StatusCaption {
					t.Errorf("row %d: StatusCaption got %q, want %q", i, row.StatusCaption, w.StatusCaption)
				}
				if row.TargetDate != w.TargetDate {
					t.Errorf("row %d: TargetDate got %q, want %q", i, row.TargetDate, w.TargetDate)
				}
			}
		})
	}
}
