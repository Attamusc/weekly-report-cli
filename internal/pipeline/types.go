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
