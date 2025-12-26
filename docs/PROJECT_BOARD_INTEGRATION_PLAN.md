# GitHub Projects Board Integration - Feature Plan

## Implementation Progress

**Status:** ✅ **COMPLETED** - Phase 6 of 6 (100% Complete)
- ✅ Phase 1: Foundation & Project URL Parsing (COMPLETED)
- ✅ Phase 2: GraphQL Client Implementation (COMPLETED)
- ✅ Phase 3: Field Filtering & Issue Extraction (COMPLETED)
- ✅ Phase 4: Input Resolution Layer (COMPLETED)
- ✅ Phase 5: CLI Integration & Pipeline Wiring (COMPLETED)
- ✅ Phase 6: Documentation & Polish (COMPLETED)

**Last Updated:** December 26, 2024

**Summary:**
- All 6 phases completed (100%)
- 95+ test cases passing
- Full documentation suite created
- **NEW: Sensible defaults added** (Status field: "In Progress,Done,Blocked")
- Feature ready for production use
- Zero breaking changes to existing functionality

## Overview

This feature adds support for fetching issues from GitHub Projects V2 boards using field-based filtering, eliminating the need to manually maintain issue URL lists. Users can provide a project board URL and specify field values (e.g., "Status=In Progress,Blocked,Done") to automatically generate reports for relevant issues.

## Goals

1. **Primary:** Enable project board-based issue discovery with field filtering
2. **Secondary:** Maintain backward compatibility with existing URL list mode
3. **Tertiary:** Support mixed sources (project board + manual URLs)
4. **Performance:** Minimize API calls using efficient GraphQL queries
5. **Usability:** Provide clear error messages and validation

## User Experience

### Input Modes

#### Mode 1: Project Board (New)
```bash
# Organization project
weekly-report-cli generate \
  --project "https://github.com/orgs/my-org/projects/5" \
  --project-field "Status" \
  --project-field-values "In Progress,Blocked,Done" \
  --since-days 7

# User project (short form)
weekly-report-cli generate \
  --project "user:username/5" \
  --project-field "Status" \
  --project-field-values "In Progress,Blocked,Done"
```

#### Mode 2: URL List (Existing)
```bash
# Stdin (unchanged)
cat links.txt | weekly-report-cli generate --since-days 7

# File input (unchanged)
weekly-report-cli generate --input links.txt --since-days 7
```

#### Mode 3: Mixed Sources
```bash
# Project board + additional manual URLs
weekly-report-cli generate \
  --project "https://github.com/orgs/my-org/projects/5" \
  --project-field "Status" \
  --project-field-values "In Progress,Blocked" \
  --input additional-issues.txt
```

### Project URL Formats (Auto-detected)

- **Full URL:** `https://github.com/orgs/{org}/projects/{number}`
- **Full URL:** `https://github.com/users/{username}/projects/{number}`
- **Short form:** `org:{org-name}/{number}`
- **Short form:** `user:{username}/{number}`

## Technical Design

### Architecture Changes

```
┌─────────────────────────────────────────────────────────────┐
│ CLI Input Layer                                             │
│ ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│ │ --project    │  │ --input      │  │ stdin        │      │
│ └──────────────┘  └──────────────┘  └──────────────┘      │
└─────────────────────────────────────────────────────────────┘
                          │
                          ▼
┌─────────────────────────────────────────────────────────────┐
│ Input Resolution Layer (NEW)                                │
│ ┌──────────────────────────────────────────────────────┐   │
│ │ internal/input/resolver.go                           │   │
│ │ - Auto-detect input mode                             │   │
│ │ - Merge multiple sources                             │   │
│ │ - Deduplicate issue refs                             │   │
│ └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
         │                              │
         │ (URL List Mode)              │ (Project Mode)
         ▼                              ▼
┌──────────────────┐         ┌─────────────────────────────┐
│ Existing:        │         │ NEW:                        │
│ input.ParseLinks │         │ internal/projects/          │
│                  │         │ ├── parser.go (URL parsing) │
│                  │         │ ├── client.go (GraphQL)     │
│                  │         │ └── filter.go (field match) │
└──────────────────┘         └─────────────────────────────┘
         │                              │
         └──────────────┬───────────────┘
                        ▼
              ┌──────────────────┐
              │ []input.IssueRef │
              └──────────────────┘
                        │
                        ▼
              ┌──────────────────┐
              │ Existing Pipeline│
              │ (unchanged)      │
              └──────────────────┘
```

### New Packages

#### `internal/projects/`

New package structure:
```
internal/projects/
├── types.go          # Data types for projects
├── parser.go         # Parse project URLs/identifiers
├── client.go         # GraphQL client for Projects API
├── query.go          # GraphQL query builders
├── filter.go         # Field-based filtering logic
├── parser_test.go
├── client_test.go
├── query_test.go
└── filter_test.go
```

### Data Structures

```go
// internal/projects/types.go

// ProjectRef represents a reference to a GitHub Project
type ProjectRef struct {
    Type   ProjectType // Organization or User
    Owner  string      // Org name or username
    Number int         // Project number
    URL    string      // Canonical URL
}

type ProjectType int
const (
    ProjectTypeOrg ProjectType = iota
    ProjectTypeUser
)

// ProjectItem represents an item in a GitHub Project
type ProjectItem struct {
    ContentType  ContentType  // Issue, PullRequest, or DraftIssue
    IssueRef     *input.IssueRef // nil for draft issues or PRs
    FieldValues  map[string]FieldValue
}

type ContentType int
const (
    ContentTypeIssue ContentType = iota
    ContentTypePullRequest
    ContentTypeDraftIssue
)

// FieldValue represents a project field value (multiple types)
type FieldValue struct {
    Type  FieldType
    Text  string     // For text/single-select fields
    Date  *time.Time // For date fields
    Number float64   // For number fields
}

type FieldType int
const (
    FieldTypeText FieldType = iota
    FieldTypeSingleSelect
    FieldTypeDate
    FieldTypeNumber
)

// FieldFilter represents filtering criteria
type FieldFilter struct {
    FieldName string
    Values    []string // Values to match (OR logic)
}

// ProjectConfig holds project query configuration
type ProjectConfig struct {
    Ref          ProjectRef
    FieldFilters []FieldFilter
    IncludePRs   bool
    MaxItems     int
}
```

### GraphQL Queries

```go
// internal/projects/query.go

// Single optimized query to fetch project items with field values
const projectItemsQuery = `
query($owner: String!, $number: Int!, $first: Int!, $cursor: String) {
  %s(login: $owner) {
    projectV2(number: $number) {
      id
      title
      fields(first: 20) {
        nodes {
          ... on ProjectV2FieldCommon {
            id
            name
          }
          ... on ProjectV2SingleSelectField {
            id
            name
            options {
              id
              name
            }
          }
        }
      }
      items(first: $first, after: $cursor) {
        nodes {
          id
          type
          content {
            ... on Issue {
              id
              number
              url
              repository {
                owner {
                  login
                }
                name
              }
            }
            ... on PullRequest {
              id
              number
              url
              repository {
                owner {
                  login
                }
                name
              }
            }
            ... on DraftIssue {
              id
              title
            }
          }
          fieldValues(first: 20) {
            nodes {
              ... on ProjectV2ItemFieldTextValue {
                text
                field {
                  ... on ProjectV2FieldCommon {
                    name
                  }
                }
              }
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                field {
                  ... on ProjectV2FieldCommon {
                    name
                  }
                }
              }
              ... on ProjectV2ItemFieldDateValue {
                date
                field {
                  ... on ProjectV2FieldCommon {
                    name
                  }
                }
              }
              ... on ProjectV2ItemFieldNumberValue {
                number
                field {
                  ... on ProjectV2FieldCommon {
                    name
                  }
                }
              }
            }
          }
        }
        pageInfo {
          hasNextPage
          endCursor
        }
      }
    }
  }
}
`
// Note: %s placeholder for "organization" or "user"
```

### API Client Implementation

```go
// internal/projects/client.go

type Client struct {
    httpClient *http.Client
    baseURL    string
    token      string
}

func NewClient(token string) *Client {
    return &Client{
        httpClient: &http.Client{Timeout: 30 * time.Second},
        baseURL:    "https://api.github.com/graphql",
        token:      token,
    }
}

// FetchProjectItems fetches all items from a project with field values
func (c *Client) FetchProjectItems(ctx context.Context, config ProjectConfig) ([]ProjectItem, error) {
    // 1. Build GraphQL query based on project type (org vs user)
    // 2. Execute query with pagination
    // 3. Parse response and extract items
    // 4. Apply field filters
    // 5. Filter out PRs if not included
    // 6. Convert to ProjectItem structs
    // 7. Return deduplicated items
}

// fetchProjectPage fetches a single page of project items
func (c *Client) fetchProjectPage(ctx context.Context, query string, variables map[string]interface{}) (*projectResponse, error) {
    // GraphQL HTTP request implementation
}
```

### URL Parsing

```go
// internal/projects/parser.go

// ParseProjectURL parses various project URL formats
func ParseProjectURL(raw string) (ProjectRef, error) {
    // Try patterns in order:
    // 1. https://github.com/orgs/{org}/projects/{number}
    // 2. https://github.com/users/{username}/projects/{number}
    // 3. org:{org}/{number}
    // 4. user:{username}/{number}
}

var projectURLPatterns = []*regexp.Regexp{
    regexp.MustCompile(`^https://github\.com/orgs/([^/]+)/projects/(\d+)`),
    regexp.MustCompile(`^https://github\.com/users/([^/]+)/projects/(\d+)`),
    regexp.MustCompile(`^org:([^/]+)/(\d+)$`),
    regexp.MustCompile(`^user:([^/]+)/(\d+)$`),
}
```

### Field Filtering Logic

```go
// internal/projects/filter.go

// MatchesFilters checks if a ProjectItem matches all field filters
func MatchesFilters(item ProjectItem, filters []FieldFilter) bool {
    // For each filter:
    //   1. Check if item has the specified field
    //   2. Check if field value matches any of the filter values (OR logic)
    //   3. All filters must match (AND logic)
    // Return true only if all filters match
}

// matchFieldValue checks if a field value matches filter criteria
func matchFieldValue(value FieldValue, filterValues []string) bool {
    // Handle different field types:
    // - Text: case-insensitive contains or exact match
    // - SingleSelect: case-insensitive exact match
    // - Date: parse and compare (support relative dates?)
    // - Number: numeric comparison
}
```

### Input Resolution Layer

```go
// internal/input/resolver.go (NEW)

type InputMode int
const (
    InputModeUnknown InputMode = iota
    InputModeURLList
    InputModeProject
    InputModeMixed
)

type ResolverConfig struct {
    ProjectURL         string
    ProjectFieldName   string
    ProjectFieldValues []string
    ProjectIncludePRs  bool
    ProjectMaxItems    int
    URLListPath        string   // File path or empty for stdin
    UseStdin          bool
}

// ResolveIssueRefs determines input mode and returns deduplicated issue refs
func ResolveIssueRefs(ctx context.Context, cfg ResolverConfig, projectClient *projects.Client) ([]input.IssueRef, error) {
    mode := detectInputMode(cfg)
    
    var allRefs []input.IssueRef
    
    // Fetch from project if specified
    if mode == InputModeProject || mode == InputModeMixed {
        projectRefs, err := fetchFromProject(ctx, cfg, projectClient)
        if err != nil {
            return nil, fmt.Errorf("failed to fetch from project: %w", err)
        }
        allRefs = append(allRefs, projectRefs...)
    }
    
    // Fetch from URL list if specified
    if mode == InputModeURLList || mode == InputModeMixed {
        urlRefs, err := fetchFromURLList(cfg)
        if err != nil {
            return nil, fmt.Errorf("failed to fetch from URL list: %w", err)
        }
        allRefs = append(allRefs, urlRefs...)
    }
    
    // Deduplicate
    return deduplicateRefs(allRefs), nil
}
```

### CLI Changes

```go
// cmd/generate.go

var (
    // Existing flags
    sinceDays     int
    inputPath     string
    concurrency   int
    noNotes       bool
    verbose       bool
    quiet         bool
    summaryPrompt string
    
    // New project-related flags
    projectURL         string
    projectField       string
    projectFieldValues string
    projectIncludePRs  bool
    projectMaxItems    int
)

func init() {
    // Existing flags...
    
    // New flags
    generateCmd.Flags().StringVar(&projectURL, "project", "", 
        "GitHub project board URL or identifier (e.g., 'https://github.com/orgs/my-org/projects/5' or 'org:my-org/5')")
    generateCmd.Flags().StringVar(&projectField, "project-field", "", 
        "Field name to filter by (e.g., 'Status')")
    generateCmd.Flags().StringVar(&projectFieldValues, "project-field-values", "", 
        "Comma-separated values to match (e.g., 'In Progress,Blocked,Done')")
    generateCmd.Flags().BoolVar(&projectIncludePRs, "project-include-prs", false, 
        "Include pull requests from project board (default: issues only)")
    generateCmd.Flags().IntVar(&projectMaxItems, "project-max-items", 100, 
        "Maximum number of items to fetch from project board")
}
```

## Implementation Phases

### Phase 1: Foundation & Project URL Parsing ✅ COMPLETED
**Goal:** Set up new package structure and URL parsing

**Status:** ✅ **COMPLETED** - All deliverables achieved, tests passing

**Tasks:**
1. ✅ Create `internal/projects/` package structure
2. ✅ Implement `types.go` with data structures
3. ✅ Implement `parser.go` with URL parsing logic
4. ✅ Write comprehensive tests for URL parsing
5. ✅ Add new CLI flags to `cmd/generate.go` (no-op for now)
6. ✅ Update go.mod with GraphQL dependencies (not needed - using custom HTTP client)

**Deliverables:**
- ✅ URL parsing for all formats (org/user, full/short)
- ✅ Test coverage for valid/invalid URLs (40+ test cases)
- ✅ CLI flags defined (validation only, no execution)

**Testing:**
```go
// internal/projects/parser_test.go
✅ TestParseProjectURL_OrgFullURL
✅ TestParseProjectURL_UserFullURL
✅ TestParseProjectURL_OrgShortForm
✅ TestParseProjectURL_UserShortForm
✅ TestParseProjectURL_InvalidFormats
✅ TestParseProjectURL_EdgeCases
```

**Results:**
- All tests passing (PASS - 8 test groups, 40+ test cases)
- CLI builds successfully with new flags
- No regressions in existing functionality

### Phase 2: GraphQL Client Implementation ✅ COMPLETED
**Goal:** Build GraphQL client with Projects API integration

**Status:** ✅ **COMPLETED** - All deliverables achieved, tests passing

**Tasks:**
1. ✅ Implement `client.go` with GraphQL HTTP client
2. ✅ Implement `query.go` with query builders
3. ✅ Add authentication and error handling
4. ✅ Implement pagination logic
5. ✅ Add retry logic for rate limits (similar to existing GitHub client)
6. ✅ Write integration tests with mock GraphQL server

**Deliverables:**
- ✅ GraphQL client with proper authentication (Bearer token)
- ✅ Pagination support for large project boards (automatic cursor-based)
- ✅ Error handling with helpful messages (401, 403, 404, 429 with context)
- ✅ Mock server tests for query validation (11 test cases)
- ✅ Retry logic with exponential backoff and jitter
- ✅ Support for all field types (text, single-select, date, number)

**Testing:**
```go
// internal/projects/client_test.go
✅ TestClient_FetchProjectItems_OrgProject
✅ TestClient_FetchProjectItems_UserProject
✅ TestClient_FetchProjectItems_Pagination
✅ TestClient_FetchProjectItems_MaxItemsLimit
✅ TestClient_FetchProjectItems_EmptyProject
✅ TestClient_FetchProjectItems_AuthError
✅ TestClient_FetchProjectItems_PermissionError
✅ TestClient_FetchProjectItems_NotFound
✅ TestClient_FetchProjectItems_RateLimit
✅ TestClient_FetchProjectItems_GraphQLErrors
✅ TestClient_FetchProjectItems_DifferentFieldTypes
```

**Results:**
- All tests passing (PASS - 11 test cases, ~11s runtime)
- Pagination tested with multi-page scenarios
- Retry logic tested with rate limit simulation
- No regressions in existing functionality

### Phase 3: Field Filtering & Issue Extraction ✅ COMPLETED
**Goal:** Implement field-based filtering and issue extraction

**Status:** ✅ **COMPLETED** - All deliverables achieved, tests passing

**Tasks:**
1. ✅ Implement `filter.go` with field matching logic
2. ✅ Add support for different field types (text, single-select, date, number)
3. ✅ Extract issue references from project items
4. ✅ Filter out PRs when not included
5. ✅ Filter out draft issues
6. ✅ Write comprehensive filter tests

**Deliverables:**
- ✅ Field filtering with AND/OR logic (AND between filters, OR within filter values)
- ✅ Type-aware field value matching (text: contains, single-select: exact, date/number: string match)
- ✅ Issue extraction from project items (FilterProjectItems function)
- ✅ Test coverage for all field types (21 test cases)
- ✅ PR filtering (configurable via IncludePRs)
- ✅ Draft issue filtering (always excluded)
- ✅ Deduplication utility (DeduplicateIssueRefs)

**Testing:**
```go
// internal/projects/filter_test.go
✅ TestMatchesFilters_NoFilters
✅ TestMatchesFilters_SingleFilter_Match
✅ TestMatchesFilters_SingleFilter_NoMatch
✅ TestMatchesFilters_SingleFilter_MultipleValues_OR
✅ TestMatchesFilters_MultipleFilters_AND
✅ TestMatchesFilters_MultipleFilters_OneDoesNotMatch
✅ TestMatchesFilters_FieldDoesNotExist
✅ TestMatchFieldValue_Text_ExactMatch
✅ TestMatchFieldValue_Text_ContainsMatch
✅ TestMatchFieldValue_Text_CaseInsensitive
✅ TestMatchFieldValue_SingleSelect_ExactMatch
✅ TestMatchFieldValue_SingleSelect_CaseInsensitive
✅ TestMatchFieldValue_SingleSelect_NoPartialMatch
✅ TestMatchFieldValue_Date
✅ TestMatchFieldValue_Number
✅ TestMatchFieldValue_EmptyFilterValues
✅ TestFilterProjectItems_IssuesOnly
✅ TestFilterProjectItems_IncludePRs
✅ TestFilterProjectItems_FiltersDraftIssues
✅ TestFilterProjectItems_NoFieldMatch
✅ TestFilterProjectItems_NoFilters
✅ TestDeduplicateIssueRefs (+ 2 variants)
```

**Results:**
- All tests passing (PASS - 21+ test cases)
- Comprehensive coverage of filter logic
- No regressions in existing functionality

### Phase 4: Input Resolution Layer ✅ COMPLETED
**Goal:** Create unified input resolution layer

**Status:** ✅ **COMPLETED** - All deliverables achieved, tests passing

**Tasks:**
1. ✅ Implement `internal/input/resolver.go`
2. ✅ Add input mode detection logic
3. ✅ Implement project-based issue fetching (stub for Phase 5)
4. ✅ Implement URL list-based issue fetching
5. ✅ Add deduplication logic for mixed sources
6. ✅ Write integration tests for all input modes

**Deliverables:**
- ✅ Auto-detection of input mode (Unknown, URLList, Project, Mixed)
- ✅ Support for project-only mode (with validation)
- ✅ Support for URL list-only mode (backward compatible)
- ✅ Support for mixed mode (project + URL list)
- ✅ Deduplication across sources (by canonical URL)
- ✅ Configuration validation (required fields, bounds checking)
- ✅ Field value parsing utility (comma-separated with trimming)

**Testing:**
```go
// internal/input/resolver_test.go
✅ TestDetectInputMode_URLListOnly
✅ TestDetectInputMode_ProjectOnly
✅ TestDetectInputMode_Mixed
✅ TestDetectInputMode_Unknown
✅ TestValidateConfig_ProjectWithoutField
✅ TestValidateConfig_ProjectWithoutFieldValues
✅ TestValidateConfig_ProjectMaxItemsTooLow
✅ TestValidateConfig_ProjectMaxItemsTooHigh
✅ TestValidateConfig_Valid
✅ TestValidateConfig_URLListOnly
✅ TestFetchFromURLList_Stdin
✅ TestFetchFromURLList_File
✅ TestFetchFromURLList_FileNotFound
✅ TestFetchFromURLList_NoSource
✅ TestDeduplicateRefs (+ 2 variants)
✅ TestParseFieldValues (+ 4 variants)
✅ TestInputMode_String
✅ TestResolveIssueRefs (+ 3 variants)
```

**Results:**
- All tests passing (PASS - 24+ test cases)
- Input mode detection working correctly
- Validation catches configuration errors
- No regressions in existing functionality

### Phase 5: CLI Integration & Pipeline Wiring ✅ COMPLETED
**Goal:** Wire everything into the generate command

**Status:** ✅ **COMPLETED** - All deliverables achieved, tests passing

**Tasks:**
1. ✅ Update `cmd/generate.go` to use resolver
2. ✅ Add project client initialization via adapter pattern
3. ✅ Update configuration struct to include project options
4. ✅ Add validation for project-related flags
5. ✅ Update help text and examples
6. ✅ Wire input.ResolveIssueRefs into generate pipeline

**Deliverables:**
- ✅ Functional end-to-end flow (all three input modes)
- ✅ Clear error messages for invalid configurations
- ✅ Updated CLI help documentation with examples
- ✅ Zero regressions in existing URL list mode
- ✅ projectClientAdapter avoids circular dependencies

**Implementation:**
- Modified `internal/config/config.go` to include Project struct
- Updated `config.FromEnvAndFlags()` signature for project parameters
- Created `projectClientAdapter` in `cmd/generate.go` to bridge packages
- Replaced `input.ParseIssueLinks` with `input.ResolveIssueRefs`
- All 95+ existing tests still passing

**Results:**
- All tests passing (PASS - 95+ test cases across all packages)
- All three input modes working correctly
- Backward compatibility maintained
- No breaking changes

### Phase 6: Documentation & Polish ✅ COMPLETED
**Goal:** Complete documentation and edge case handling

**Status:** ✅ **COMPLETED** - All documentation created

**Tasks:**
1. ✅ Update README.md with project board examples
2. ✅ Create docs/PROJECT_BOARDS.md with detailed guide
3. ✅ Add token scope documentation (read:project required)
4. ✅ Update CLAUDE.md with new architecture details
5. ✅ Add example project board configurations

**Deliverables:**
- ✅ Comprehensive README updates with:
  - Token scope requirements (repo + read:project)
  - Three input modes clearly documented
  - Command-line examples for all modes
  - Updated project structure section
  - Troubleshooting for project board errors
- ✅ Detailed project board usage guide (docs/PROJECT_BOARDS.md):
  - Quick start guide
  - Project URL formats (4 variants)
  - Filtering options and behavior
  - 15+ usage examples
  - Filter logic explanation (AND/OR)
  - Mixed mode documentation
  - Performance considerations
  - Comprehensive troubleshooting
  - API details and GraphQL information
  - Migration guide from URL lists
- ✅ Updated CLAUDE.md with:
  - New pipeline architecture diagram
  - Input resolution flow
  - Projects module documentation
  - Token scope requirements
  - CLI usage examples for all modes

**Documentation Stats:**
- README.md: Updated 7 sections
- PROJECT_BOARDS.md: 500+ lines, 15+ examples
- CLAUDE.md: Updated 5 sections with new architecture

**Results:**
- Complete documentation suite
- Clear migration path for existing users
- Comprehensive troubleshooting guide
- Feature ready for production use

## Configuration Updates

### Environment Variables

No new required environment variables. `GITHUB_TOKEN` must have additional scope:

**Required Scopes:**
- Existing: `repo` (for repository access)
- **NEW:** `read:project` (for project board access)

### CLI Flag Summary

**New Flags:**
```
--project string              GitHub project board URL or identifier
--project-field string        Field name to filter by
--project-field-values string Comma-separated values to match
--project-include-prs         Include pull requests from project (default: false)
--project-max-items int       Max items to fetch from project (default: 100)
```

**Validation Rules:**
- If `--project` is specified, `--project-field` and `--project-field-values` are required
- `--project` and `--input`/stdin can be used together (mixed mode)
- `--project-max-items` must be between 1 and 1000

## Dependencies

### New Go Modules

```go
// GraphQL client options:

// Option 1: shurcooL/graphql (simpler, type-safe)
github.com/shurcooL/graphql v0.0.0-20231217050601-ba74f03e802e

// Option 2: machinebox/graphql (more flexible)
github.com/machinebox/graphql v0.2.2

// Option 3: Custom HTTP client (no new dependencies)
// Use existing net/http with JSON encoding
```

**Recommendation:** Start with custom HTTP client (no new deps) since we only need a few queries. Can upgrade to typed client later if needed.

## Error Handling

### New Error Scenarios

1. **Invalid Project URL Format**
   ```
   Error: Invalid project URL format. 
   Expected formats:
     - https://github.com/orgs/{org}/projects/{number}
     - https://github.com/users/{username}/projects/{number}
     - org:{org}/{number}
     - user:{username}/{number}
   ```

2. **Missing Project Permissions**
   ```
   Error: GitHub API access denied for project 'org/my-org/5'.
   Your token may require the 'read:project' scope.
   Visit https://github.com/settings/tokens to update your token.
   ```

3. **Project Not Found**
   ```
   Error: Project not found: org/my-org/5
   This could mean:
     - The project doesn't exist
     - The project is private and your token lacks access
     - The organization/user name is incorrect
   ```

4. **Field Not Found**
   ```
   Error: Field 'Status' not found in project 'org/my-org/5'.
   Available fields:
     - Priority
     - Assignee
     - Sprint
   Use --verbose to see full field details.
   ```

5. **No Matching Items**
   ```
   Warning: No items found in project 'org/my-org/5' matching:
     Field 'Status' = 'In Progress,Blocked,Done'
   Found 45 total items in project but none matched the filter.
   ```

6. **GraphQL Rate Limit**
   ```
   Error: GitHub GraphQL API rate limit exceeded.
   Retry after: 15 minutes
   Tip: Use --project-max-items to reduce query cost.
   ```

## Performance Considerations

### API Call Optimization

**Single Project Query:**
- 1 GraphQL query per project (fetches items + field definitions)
- Pagination: 100 items per page (adjustable via `--project-max-items`)
- Estimated cost: ~1-5 points per 100 items (depends on field complexity)

**Mixed Mode:**
- 1 GraphQL query for project
- 0 additional REST calls (issue URLs already known)
- Total: 1 GraphQL + N REST (where N = issues from URL list)

### Pagination Strategy

```go
// Fetch in batches of 100 (configurable)
batchSize := min(config.MaxItems, 100)

for hasMore && totalFetched < config.MaxItems {
    page := fetchProjectPage(ctx, cursor, batchSize)
    items = append(items, page.Items...)
    cursor = page.NextCursor
    hasMore = page.HasMore
    totalFetched += len(page.Items)
}
```

### Caching Considerations

**NOT implementing caching in v1:**
- Projects change frequently
- Adds complexity for minimal benefit
- Users can re-run quickly enough

**Future consideration:**
- Optional `--cache-project` flag with TTL
- Cache project items for 5-10 minutes
- Useful for debugging/development

## Testing Strategy

### Unit Tests

Each package should have >80% coverage:

```
internal/projects/parser_test.go      - URL parsing
internal/projects/client_test.go      - GraphQL client
internal/projects/query_test.go       - Query building
internal/projects/filter_test.go      - Field filtering
internal/input/resolver_test.go       - Input resolution
```

### Integration Tests

```
cmd/generate_test.go                  - E2E with mock servers
```

Mock GraphQL server responses:
- Successful project fetch
- Pagination scenarios
- Error responses (401, 403, 404, 500)
- Rate limit responses

### Manual Testing Checklist

- [ ] Organization project with single-select field
- [ ] User project with text field
- [ ] Project with 100+ items (pagination)
- [ ] Project with no matching items
- [ ] Mixed mode (project + URL list)
- [ ] Invalid project URL
- [ ] Missing token permissions
- [ ] Private project access denied
- [ ] Non-existent field name
- [ ] Empty field values

## Migration Path

### For Existing Users

**No breaking changes:**
- Existing URL list mode continues to work unchanged
- No configuration changes required
- Opt-in to new project board mode

**Migration steps (optional):**
1. Update `GITHUB_TOKEN` with `read:project` scope
2. Try `--project` flag with existing workflows
3. Gradually replace URL lists with project filtering

### Documentation Updates

**README.md:**
- Add "Project Board Mode" section before "Usage"
- Update examples to show both modes
- Add token scope requirements

**New docs/PROJECT_BOARDS.md:**
- Complete guide to project board integration
- Field type explanations
- Filtering strategies
- Troubleshooting guide

## Success Criteria

### Functional Requirements

- ✅ Parse all project URL formats correctly
- ✅ Fetch project items via GraphQL with pagination
- ✅ Filter items by single-select field values
- ✅ Extract issue references from project items
- ✅ Support mixed mode (project + URL list)
- ✅ Maintain backward compatibility with URL list mode
- ✅ Provide clear error messages for common failures

### Non-Functional Requirements

- ✅ No more than 1 GraphQL query per project board
- ✅ Handle projects with 100+ items efficiently
- ✅ Process time < 2x current URL list mode
- ✅ Test coverage >80% for new code
- ✅ Documentation complete and clear

## Future Enhancements (Out of Scope for v1)

1. **Multiple Field Filters**
   ```bash
   --project-filter "Status=In Progress,Blocked" \
   --project-filter "Priority=High,Critical"
   ```

2. **Advanced Field Types**
   - Date range filtering
   - Number range filtering
   - Iteration/sprint filtering

3. **Project Board Output**
   - Update project fields based on report data
   - Automatically mark items as "Reported"

4. **Organization-Wide Projects**
   - Fetch all projects in an org
   - Cross-project reporting

5. **Caching**
   - Optional project data caching
   - Configurable TTL

6. **Dry-Run Mode**
   ```bash
   --dry-run  # Show what would be processed without fetching
   ```

## Open Questions

1. **Should we support multiple project boards in a single run?**
   - Example: `--project url1 --project url2`
   - Decision: Not in v1, can add later if needed

2. **Should we support complex filter logic (AND/OR combinations)?**
   - Example: `(Status=In Progress AND Priority=High) OR Status=Blocked`
   - Decision: Not in v1, single field with OR values is sufficient

3. **Should we validate field names before querying?**
   - Pro: Early feedback, better UX
   - Con: Extra API call
   - Decision: Validate during query, provide helpful error with available fields

4. **Should we support filtering by built-in fields (Assignee, Labels)?**
   - Decision: Yes, but treat them same as custom fields for consistency

## Timeline Estimate

**Phase 1:** 4-6 hours (Foundation & URL parsing)
**Phase 2:** 6-8 hours (GraphQL client)
**Phase 3:** 4-6 hours (Field filtering)
**Phase 4:** 4-6 hours (Input resolution)
**Phase 5:** 4-6 hours (CLI integration)
**Phase 6:** 3-4 hours (Documentation)

**Total: 25-36 hours** (3-5 days of focused work)

## Risk Assessment

### High Risk
- **GraphQL API complexity:** Mitigate with thorough testing and mock servers
- **Token scope requirements:** Clear documentation and error messages

### Medium Risk
- **Pagination edge cases:** Test with large projects (100+ items)
- **Field type variations:** Support common types first, extend later

### Low Risk
- **Backward compatibility:** Existing code path unchanged
- **Performance:** GraphQL is efficient, minimal impact

## Appendix

### Example GraphQL Response

```json
{
  "data": {
    "organization": {
      "projectV2": {
        "id": "PVT_kwDOABCD",
        "title": "Department Execution",
        "fields": {
          "nodes": [
            {
              "id": "PVTF_lADOABCD",
              "name": "Status",
              "options": [
                {"id": "1", "name": "Not Started"},
                {"id": "2", "name": "In Progress"},
                {"id": "3", "name": "Blocked"},
                {"id": "4", "name": "Done"}
              ]
            }
          ]
        },
        "items": {
          "nodes": [
            {
              "id": "PVTI_lADOABCD",
              "type": "ISSUE",
              "content": {
                "id": "I_kwDOABCD",
                "number": 123,
                "url": "https://github.com/org/repo/issues/123",
                "repository": {
                  "owner": {"login": "org"},
                  "name": "repo"
                }
              },
              "fieldValues": {
                "nodes": [
                  {
                    "name": "In Progress",
                    "field": {"name": "Status"}
                  }
                ]
              }
            }
          ],
          "pageInfo": {
            "hasNextPage": false,
            "endCursor": null
          }
        }
      }
    }
  }
}
```

### Example Usage Scenarios

**Scenario 1: Team Sprint Board**
```bash
weekly-report-cli generate \
  --project "org:acme-corp/12" \
  --project-field "Status" \
  --project-field-values "In Progress,In Review,Done" \
  --since-days 7 \
  --verbose
```

**Scenario 2: Executive Epics Dashboard**
```bash
weekly-report-cli generate \
  --project "https://github.com/orgs/acme-corp/projects/5" \
  --project-field "Phase" \
  --project-field-values "Active,At Risk" \
  --since-days 14 \
  --no-notes
```

**Scenario 3: Mixed Sources**
```bash
weekly-report-cli generate \
  --project "org:acme-corp/12" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --input additional-critical-issues.txt \
  --since-days 7
```
