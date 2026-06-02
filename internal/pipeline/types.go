package pipeline

import (
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/derive"
	"github.com/Attamusc/weekly-report-cli/internal/format"
	"github.com/Attamusc/weekly-report-cli/internal/report"
)

// SummaryCompleted is the default summary for done/closed issues that don't need AI summarization.
const SummaryCompleted = "Completed"

// IssueData represents collected data from an issue before AI summarization.
type IssueData struct {
	IssueURL              string
	IssueTitle            string
	IssueState            string
	CreatedAt             time.Time
	ClosedAt              *time.Time
	CloseReason           string
	Labels                []string
	Assignees             []string          // Issue assignees (usernames)
	ExtraColumns          map[string]string // Project field values for custom columns
	Reports               []report.Report
	UpdateTexts           []string
	Status                derive.Status
	ReportedStatusCaption string
	TargetDate            *time.Time
	ShouldSummarize       bool
	FallbackSummary       string
	Note                  *format.Note
}

// IssueDataResult represents the result of collecting issue data.
type IssueDataResult struct {
	Data IssueData
	Err  error
}

// IssueResult represents the result of processing a single issue.
type IssueResult struct {
	IssueURL string
	Row      *format.Row
	Note     *format.Note
	Err      error
}

// DescribeIssueData represents collected data from an issue for the describe command.
type DescribeIssueData struct {
	IssueURL            string
	IssueTitle          string
	IssueBody           string
	Labels              []string
	Assignees           []string
	FallbackDescription string
}

// DescribeIssueDataResult represents the result of collecting issue data for describe.
type DescribeIssueDataResult struct {
	Data DescribeIssueData
	Err  error
}

// HighlightData represents lightweight data collected for the highlights command.
// Unlike IssueData, this skips report extraction, close-reason fetching,
// and status derivation — just metadata + recent comment text.
//
// Deprecated: replaced by NarrativeItem. Will be deleted in TODO-narrative-cleanup.
type HighlightData struct {
	IssueURL    string
	IssueTitle  string
	IssueState  string
	Labels      []string
	UpdateTexts []string
}

// HighlightDataResult represents the result of collecting highlight data.
type HighlightDataResult struct {
	Data HighlightData
	Err  error
}

// Truncation caps applied at collection time for NarrativeItem fields.
// These bound token budget for the narrative AI call.
const (
	MaxBodyChars    = 500 // max chars for issue/PR body
	MaxCommentChars = 300 // max chars per comment body
	MaxComments     = 5   // max recent comments per item
	MaxEvents       = 10  // max timeline events per item
)

// NarrativeItem is the rich per-survivor data shape consumed by the narrative
// AI call. Replaces HighlightData, which carried only unattributed comment bodies.
type NarrativeItem struct {
	URL            string
	Title          string
	Body           string // truncated to MaxBodyChars at collection time
	State          string // "open" | "closed"
	IsPR           bool
	Author         string // issue/PR opener
	Assignees      []string
	Labels         []string
	OpenedAt       time.Time
	ClosedAt       *time.Time
	MergedAt       *time.Time // PRs only, nil if not merged
	ClosedThisWeek bool
	MergedThisWeek bool
	RecentComments []NarrativeComment // chronological, since lookback
	Events         []NarrativeEvent   // chronological, since lookback, filtered
}

// NarrativeComment is an attributed comment included in a NarrativeItem.
type NarrativeComment struct {
	Author    string
	CreatedAt time.Time
	Body      string // truncated to MaxCommentChars at collection time
}

// NarrativeEvent is a filtered timeline event included in a NarrativeItem.
type NarrativeEvent struct {
	Type   string // e.g. "closed", "merged", "reviewed", "review_requested"
	Actor  string // username who performed the event
	At     time.Time
	Detail string // event-specific text (close reason, review state, etc.)
}

// NarrativeItemResult represents the result of collecting a NarrativeItem.
type NarrativeItemResult struct {
	Data NarrativeItem
	Err  error
}
