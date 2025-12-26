package projects

import (
	"fmt"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

// ProjectType represents the type of project owner
type ProjectType int

const (
	// ProjectTypeOrg represents an organization-owned project
	ProjectTypeOrg ProjectType = iota
	// ProjectTypeUser represents a user-owned project
	ProjectTypeUser
)

// String returns the string representation of ProjectType
func (pt ProjectType) String() string {
	switch pt {
	case ProjectTypeOrg:
		return "organization"
	case ProjectTypeUser:
		return "user"
	default:
		return "unknown"
	}
}

// ProjectRef represents a reference to a GitHub Project
type ProjectRef struct {
	Type   ProjectType // Organization or User
	Owner  string      // Org name or username
	Number int         // Project number
	URL    string      // Canonical URL
}

// String returns a string representation of the ProjectRef
func (ref ProjectRef) String() string {
	return fmt.Sprintf("%s:%s/%d", ref.Type, ref.Owner, ref.Number)
}

// ContentType represents the type of content in a project item
type ContentType int

const (
	// ContentTypeIssue represents a GitHub issue
	ContentTypeIssue ContentType = iota
	// ContentTypePullRequest represents a GitHub pull request
	ContentTypePullRequest
	// ContentTypeDraftIssue represents a draft issue (created directly in the project)
	ContentTypeDraftIssue
)

// String returns the string representation of ContentType
func (ct ContentType) String() string {
	switch ct {
	case ContentTypeIssue:
		return "Issue"
	case ContentTypePullRequest:
		return "PullRequest"
	case ContentTypeDraftIssue:
		return "DraftIssue"
	default:
		return "Unknown"
	}
}

// FieldType represents the type of a project field
type FieldType int

const (
	// FieldTypeText represents a text field
	FieldTypeText FieldType = iota
	// FieldTypeSingleSelect represents a single-select dropdown field
	FieldTypeSingleSelect
	// FieldTypeDate represents a date field
	FieldTypeDate
	// FieldTypeNumber represents a number field
	FieldTypeNumber
)

// String returns the string representation of FieldType
func (ft FieldType) String() string {
	switch ft {
	case FieldTypeText:
		return "Text"
	case FieldTypeSingleSelect:
		return "SingleSelect"
	case FieldTypeDate:
		return "Date"
	case FieldTypeNumber:
		return "Number"
	default:
		return "Unknown"
	}
}

// FieldValue represents a project field value (multiple types)
type FieldValue struct {
	Type   FieldType
	Text   string     // For text/single-select fields
	Date   *time.Time // For date fields
	Number float64    // For number fields
}

// String returns a string representation of the FieldValue
func (fv FieldValue) String() string {
	switch fv.Type {
	case FieldTypeText, FieldTypeSingleSelect:
		return fv.Text
	case FieldTypeDate:
		if fv.Date != nil {
			return fv.Date.Format("2006-01-02")
		}
		return ""
	case FieldTypeNumber:
		return fmt.Sprintf("%f", fv.Number)
	default:
		return ""
	}
}

// ProjectItem represents an item in a GitHub Project
type ProjectItem struct {
	ContentType ContentType           // Issue, PullRequest, or DraftIssue
	IssueRef    *input.IssueRef       // nil for draft issues or PRs (when not included)
	FieldValues map[string]FieldValue // Field name -> Field value
}

// FieldFilter represents filtering criteria for project items
type FieldFilter struct {
	FieldName string   // Name of the field to filter by
	Values    []string // Values to match (OR logic within this filter)
}

// ProjectConfig holds project query configuration
type ProjectConfig struct {
	Ref          ProjectRef    // Project reference
	FieldFilters []FieldFilter // Field filters to apply (AND logic between filters)
	IncludePRs   bool          // Whether to include pull requests
	MaxItems     int           // Maximum number of items to fetch
}
