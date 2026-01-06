# GitHub Projects Views Integration Plan

This document outlines the plan to add support for GitHub Projects V2 views, allowing users to leverage pre-configured view filters instead of manually specifying field filters.

## Table of Contents

- [Overview](#overview)
- [Current State](#current-state)
- [Motivation](#motivation)
- [Proposed Feature](#proposed-feature)
- [Technical Design](#technical-design)
- [Implementation Plan](#implementation-plan)
- [Usage Examples](#usage-examples)
- [API Details](#api-details)
- [Risk Assessment](#risk-assessment)
- [Success Metrics](#success-metrics)

## Overview

GitHub Projects V2 allows users to create multiple "views" within a project. Each view is a saved configuration that includes:
- Field filters (e.g., Status = "Blocked")
- Layout settings (Table, Board, Roadmap)
- Sorting and grouping rules
- Custom field visibility

This feature will enable users to reference a specific view by name, automatically applying that view's filters to the report generation.

## Current State

### What Works Today

The tool currently supports project board integration with **client-side field filtering**:

```bash
# Current approach: Manual field filtering
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked"
```

**Current Workflow:**
1. User specifies project + field filters manually
2. Tool fetches **all items** from project (up to `--project-max-items`)
3. Tool applies filters **client-side**
4. Returns matching items

### Current Limitations

1. ❌ **No view awareness** - Cannot reference pre-configured views
2. ❌ **Manual filter specification** - Users must know and type field names/values
3. ❌ **Potential inefficiency** - Fetches all items even when view has narrow filter
4. ❌ **No complex filters** - Cannot leverage multi-field view filters easily
5. ❌ **Terminology mismatch** - Users think in "views" but must use "fields"

## Motivation

### User Pain Points

**Problem 1: Manual Filter Maintenance**
```bash
# User has view in GitHub UI called "Critical Blockers"
# But must manually replicate its filters:
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --project-field "Priority" \
  --project-field-values "Critical,High"
```

**Problem 2: Filter Drift**
- User updates view filter in GitHub UI
- CLI command still uses old filters
- Reports become stale/incorrect

**Problem 3: Terminology Confusion**
- Users manage work in views (GitHub UI terminology)
- Tool requires field-based filtering (implementation detail)
- Mental model mismatch

### Desired Experience

```bash
# Proposed: Reference view directly
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Critical Blockers"
```

**Benefits:**
- ✅ Matches GitHub UI terminology
- ✅ Single source of truth (view filter definition)
- ✅ No manual filter replication
- ✅ Automatic sync with view changes
- ✅ Simpler CLI invocation

## Proposed Feature

### Core Functionality

Add support for specifying a view by name or ID:

```bash
# By view name (recommended for users)
--project-view "View Name"

# By view ID (recommended for automation)
--project-view-id "PVT_kwDOABCDEF"
```

### Backward Compatibility

**Existing functionality remains unchanged:**
- Default behavior (no flags) → defaults to "Status" field filtering
- Manual field filters → continue to work exactly as before
- Mixed mode (project + URL list) → continues to work

**New capability:**
- Can specify view instead of/in addition to field filters

### Filter Merging Strategy

When both view and manual filters are specified, **view acts as base** with manual filters added (AND logic):

```bash
# View filters + manual filters
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Current Sprint" \
  --project-field "Priority" \
  --project-field-values "High"

# Result: Items matching "Current Sprint" view filters AND Priority=High
```

**Rationale:** Additive approach is most flexible and predictable.

## Technical Design

### Architecture Components

```
┌─────────────────────────────────────────────────────────┐
│ CLI Layer (cmd/generate.go)                             │
│ - Parse --project-view and --project-view-id flags      │
│ - Pass to resolver config                                │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│ Input Resolver (internal/input/resolver.go)             │
│ - Determine input mode (project + view)                 │
│ - Delegate to project client adapter                    │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│ Project Client Adapter (cmd/generate.go)                │
│ - Convert resolver config to project config             │
│ - Add view name/ID to project config                    │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│ Projects Client (internal/projects/client.go)           │
│ - NEW: FetchProjectViews()                              │
│ - NEW: FindViewByName()                                 │
│ - Modified: FetchProjectItems() - view-aware            │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│ View Filter Parser (internal/projects/view_filter.go)   │
│ - NEW: ParseViewFilter()                                │
│ - NEW: MergeFilters()                                   │
└─────────────────────┬───────────────────────────────────┘
                      │
                      ▼
┌─────────────────────────────────────────────────────────┐
│ Filter Logic (internal/projects/filter.go)              │
│ - Existing: FilterProjectItems() - no changes needed    │
│ - Existing: MatchesFilters() - no changes needed        │
└─────────────────────────────────────────────────────────┘
```

### Data Flow

**Without View (Current):**
```
User → Manual Filters → Fetch All Items → Filter Client-Side → Results
```

**With View (Proposed):**
```
User → View Name → Fetch Views → Find View → Parse View Filter →
Merge with Manual Filters → Fetch All Items → Filter Client-Side → Results
```

**Future Optimization (Phase 7):**
```
User → View ID → Fetch Items via View.items() → Results
                 (server-side filtering, if supported)
```

### New Types

```go
// internal/projects/types.go

// ProjectView represents a GitHub Projects V2 view
type ProjectView struct {
    ID     string // Global node ID (e.g., "PVT_kwDOABCDEF")
    Name   string // Human-readable name
    Filter string // JSON filter configuration
    Layout string // TABLE_LAYOUT, BOARD_LAYOUT, ROADMAP_LAYOUT
}

// ProjectConfig (modified)
type ProjectConfig struct {
    Ref          ProjectRef
    ViewName     string        // NEW: Optional view name
    ViewID       string        // NEW: Optional view ID (takes precedence)
    FieldFilters []FieldFilter // Existing: Manual field filters
    IncludePRs   bool
    MaxItems     int
}
```

### New GraphQL Queries

```graphql
# Query to fetch all views from a project
query($owner: String!, $number: Int!) {
  organization(login: $owner) {
    projectV2(number: $number) {
      id
      title
      views(first: 20) {
        nodes {
          id
          name
          filter
          layout
        }
      }
    }
  }
}
```

## Implementation Plan

### Phase Breakdown

| Phase | Description | Effort | Risk |
|-------|-------------|--------|------|
| 1 | Research & Validation ✅ | 0.5 days | Low |
| 2 | GraphQL Query Enhancement | 0.5 days | Low |
| 3 | View Filter Parsing | 1-2 days | Medium |
| 4 | CLI Flag Addition | 0.5 days | Low |
| 5 | Integration & Resolution | 0.5-1 days | Low |
| 6 | Testing & Documentation | 0.5-1 days | Low |
| 7 | Server-Side Optimization (Optional) | 0.5-1 days | Medium |
| **Total MVP (Phases 1-6)** | | **3.5-5.5 days** | **Low** |

### Phase 1: Research & Validation ✅

**Status:** Complete

**Findings:**
- ✅ GitHub GraphQL API supports `ProjectV2View` object
- ✅ Views have `id`, `name`, `filter`, `layout` fields
- ✅ Views are queryable via `projectV2.views()`
- ⚠️ View filter format is JSON but not well-documented
- ❓ Unknown: Does `view.items()` support server-side filtering?

### Phase 2: GraphQL Query Enhancement

**Files to create/modify:**
- `internal/projects/query.go` (modify)
- `internal/projects/types.go` (modify)

**Tasks:**
1. Add `projectViewsQuery` constant
2. Add `FetchProjectViews()` method to Client
3. Add `ProjectView` type
4. Add GraphQL response types for views
5. Update `projectV2` response type to include views

**Deliverables:**
- `FetchProjectViews(ctx, ref) ([]ProjectView, error)`
- Unit tests for view fetching

**Acceptance Criteria:**
- Can query and list all views from a project
- View ID, name, filter, and layout are correctly parsed
- Error handling for projects without views
- Logging for view fetching operations

### Phase 3: View Filter Parsing

**Files to create:**
- `internal/projects/view_filter.go` (new)
- `internal/projects/view_filter_test.go` (new)

**Tasks:**
1. Research GitHub's view filter JSON format
   - Create test views in GitHub UI
   - Inspect filter JSON via GraphQL
   - Document common patterns
2. Implement `ParseViewFilter(filterJSON) ([]FieldFilter, error)`
3. Support common filter types:
   - Field equals value
   - Field in list of values
   - Field contains substring (for text fields)
4. Implement `MergeFilters(viewFilters, userFilters) []FieldFilter`
5. Add clear error messages for unsupported filter types

**Filter Support Scope:**
- ✅ **Single-select fields**: Exact match
- ✅ **Text fields**: Contains/equals
- ✅ **Multiple field filters**: AND logic
- ✅ **Multiple values per field**: OR logic
- ❌ **Complex boolean logic**: Not supported (with clear error)
- ❌ **Date range filters**: Not supported initially (with clear error)
- ❌ **Iteration/milestone filters**: Not supported initially (with clear error)

**Deliverables:**
- `ParseViewFilter()` function
- `MergeFilters()` function
- Comprehensive unit tests
- Documentation of supported/unsupported filter types

**Acceptance Criteria:**
- Can parse common view filters (80% use case)
- Clear error messages for unsupported filters
- Merged filters maintain AND/OR logic correctly
- Well-tested edge cases

### Phase 4: CLI Flag Addition

**Files to modify:**
- `cmd/generate.go` (modify)
- `internal/input/resolver.go` (modify)
- `internal/config/config.go` (modify)

**Tasks:**
1. Add CLI flags:
   ```go
   var (
       projectView   string
       projectViewID string
   )
   
   generateCmd.Flags().StringVar(&projectView, "project-view", "",
       "GitHub project view name (e.g., 'Blocked Items')")
   generateCmd.Flags().StringVar(&projectViewID, "project-view-id", "",
       "GitHub project view ID (e.g., 'PVT_kwDOABCDEF') - takes precedence over --project-view")
   ```

2. Update `ResolverConfig`:
   ```go
   type ResolverConfig struct {
       // ... existing fields ...
       ProjectView   string
       ProjectViewID string
   }
   ```

3. Update `Config` struct:
   ```go
   type Config struct {
       // ... existing fields ...
       Project struct {
           // ... existing fields ...
           ViewName string
           ViewID   string
       }
   }
   ```

4. Update flag parsing in `runGenerate()`

5. Update help text and examples

**Deliverables:**
- CLI flags functional
- Config structs updated
- Help text updated

**Acceptance Criteria:**
- Flags appear in `--help` output
- Flags are parsed correctly
- Validation: Cannot specify view and manual filters incorrectly
- Clear error messages for invalid combinations

### Phase 5: Integration & Resolution Logic

**Files to modify:**
- `cmd/generate.go` (modify `projectClientAdapter`)
- `internal/projects/client.go` (modify `FetchProjectItems`)

**Tasks:**
1. Implement view resolution workflow in `FetchProjectItems()`:
   ```go
   func (c *Client) FetchProjectItems(ctx context.Context, config ProjectConfig) ([]ProjectItem, error) {
       var viewFilters []FieldFilter
       
       // If view specified, resolve it
       if config.ViewID != "" || config.ViewName != "" {
           view, err := c.resolveView(ctx, config)
           if err != nil {
               return nil, err
           }
           
           // Parse view filter
           viewFilters, err = ParseViewFilter(view.Filter)
           if err != nil {
               return nil, fmt.Errorf("failed to parse view filter: %w", err)
           }
       }
       
       // Merge with manual filters
       finalFilters := MergeFilters(viewFilters, config.FieldFilters)
       
       // Update config with merged filters
       config.FieldFilters = finalFilters
       
       // Continue with existing fetch logic...
   }
   ```

2. Implement `resolveView()` helper:
   ```go
   func (c *Client) resolveView(ctx context.Context, config ProjectConfig) (*ProjectView, error) {
       // If ViewID provided, try to use it directly (future optimization)
       // For now, fetch all views and find by name or ID
       
       views, err := c.FetchProjectViews(ctx, config.Ref)
       if err != nil {
           return nil, err
       }
       
       if config.ViewID != "" {
           return findViewByID(views, config.ViewID)
       }
       
       return findViewByName(views, config.ViewName)
   }
   ```

3. Implement error handling with helpful messages:
   ```go
   if viewNotFound {
       availableViews := listViewNames(views)
       return nil, fmt.Errorf("view '%s' not found in project '%s'.\nAvailable views: %s",
           viewName, ref.String(), availableViews)
   }
   ```

4. Update `projectClientAdapter.FetchProjectItems()` to pass view config

**Deliverables:**
- End-to-end view resolution working
- Helpful error messages
- Logging at appropriate levels

**Acceptance Criteria:**
- Can fetch items using view name
- Can fetch items using view ID
- View filters correctly applied
- Manual filters correctly merged
- Error messages list available views
- No regression in non-view usage

### Phase 6: Testing & Documentation

**Files to create/modify:**
- `internal/projects/client_test.go` (add tests)
- `internal/projects/view_filter_test.go` (comprehensive tests)
- `cmd/generate_test.go` (integration tests)
- `docs/PROJECT_VIEWS.md` (new documentation)
- `README.md` (update with view examples)
- `CLAUDE.md` (update architecture section)

**Tasks:**

1. **Unit Tests**
   - View fetching (mock GraphQL responses)
   - View lookup by name (case-insensitive)
   - View lookup by ID
   - Filter parsing (various formats)
   - Filter merging (edge cases)

2. **Integration Tests**
   - End-to-end view resolution
   - Error cases (view not found, invalid filter, etc.)
   - Mixed mode (view + manual filters)
   - Backward compatibility (no view specified)

3. **Documentation**
   - Create `docs/PROJECT_VIEWS.md` (this document)
   - Add examples to README.md
   - Update CLAUDE.md architecture section
   - Add troubleshooting section

4. **Manual Testing Checklist**
   - [ ] Fetch items by view name
   - [ ] Fetch items by view ID
   - [ ] Combine view with manual filters
   - [ ] Error when view not found (lists available views)
   - [ ] Error when filter parsing fails (clear message)
   - [ ] Backward compatibility (existing commands work)
   - [ ] Performance (no significant slowdown)

**Deliverables:**
- >80% test coverage for new code
- Comprehensive documentation
- Updated README examples
- Manual testing completed

**Acceptance Criteria:**
- All tests pass
- Documentation complete and accurate
- No regressions in existing features
- Performance acceptable

### Phase 7: Server-Side Optimization (Optional)

**Status:** Research needed

**Question:** Does GitHub GraphQL support querying items via `view.items()`?

**Hypothesis:**
```graphql
query($owner: String!, $number: Int!, $viewId: ID!, $first: Int!) {
  organization(login: $owner) {
    projectV2(number: $number) {
      view(id: $viewId) {
        items(first: $first) {
          nodes {
            # Only items matching view's filter
          }
        }
      }
    }
  }
}
```

**If Supported:**
- Implement optimized query path
- Benchmark performance improvement
- Document API quota savings

**If Not Supported:**
- Document limitation
- Continue with client-side filtering
- Still valuable for UX (view names vs. manual filters)

**Deliverables:**
- Research findings documented
- If supported: Optimized query implementation
- Performance benchmarks

**Acceptance Criteria:**
- Confirmed whether server-side filtering is available
- If yes: Implemented and tested
- If no: Documented clearly

## Usage Examples

### Example 1: Simple View Usage

```bash
# Fetch items from "Blocked Items" view
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Blocked Items" \
  --since-days 7
```

### Example 2: View with Additional Filters

```bash
# Start with "Current Sprint" view, add Priority filter
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Current Sprint" \
  --project-field "Priority" \
  --project-field-values "High,Critical" \
  --since-days 7
```

### Example 3: View by ID (Automation)

```bash
# Use view ID for stable automation scripts
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view-id "PVT_kwDOABCDEF4567890" \
  --since-days 14
```

### Example 4: Mixed Mode with View

```bash
# Combine view-based filtering with manual URL list
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Critical Issues" \
  --input additional-issues.txt \
  --since-days 7
```

### Example 5: Backward Compatible (No View)

```bash
# Existing usage continues to work unchanged
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --since-days 7
```

## API Details

### ProjectV2View Object

**GraphQL Schema:**
```graphql
type ProjectV2View {
  id: ID!
  name: String!
  filter: String
  layout: ProjectV2ViewLayout!
  fields(first: Int): ProjectV2FieldConfigurationConnection!
}

enum ProjectV2ViewLayout {
  TABLE_LAYOUT
  BOARD_LAYOUT
  ROADMAP_LAYOUT
}
```

### View Filter Format

**Research Needed:** GitHub's view filter format is not well-documented.

**Known Structure (to be validated):**
```json
{
  "filters": [
    {
      "field": "Status",
      "operator": "equals",
      "value": "Blocked"
    },
    {
      "field": "Priority",
      "operator": "in",
      "values": ["High", "Critical"]
    }
  ],
  "logic": "AND"
}
```

**Note:** Actual format will be determined during Phase 3 through experimentation.

### Supported Filter Types (Initial Scope)

| Filter Type | Support Level | Example |
|-------------|---------------|---------|
| Field equals value | ✅ Full | `Status = "Blocked"` |
| Field in values | ✅ Full | `Priority in ["High", "Critical"]` |
| Field contains text | ✅ Full | `Title contains "bug"` |
| Multiple fields (AND) | ✅ Full | `Status = "Blocked" AND Priority = "High"` |
| Date comparisons | ❌ Future | `TargetDate < "2025-12-31"` |
| Complex boolean | ❌ Won't support | `(A OR B) AND (C OR D)` |
| Iteration filters | ❌ Future | `Iteration = "Sprint 12"` |

### Error Messages

**View Not Found:**
```
Error: view 'Invalid Name' not found in project 'org:my-org/5'

Available views:
  - Blocked Items
  - Current Sprint
  - Q1 Roadmap
  - Completed Work

Tip: View names are case-sensitive. Use --project-view "Blocked Items"
```

**Unsupported Filter Type:**
```
Error: unable to parse view filter for 'Complex View'

Reason: Date range filters are not yet supported.

Workaround: Use manual field filters with --project-field instead:
  --project-field "Status" --project-field-values "Blocked"

Supported filter types: field equals, field in list, field contains
```

**View ID Not Found:**
```
Error: view with ID 'PVT_kwDOABCDEF' not found in project 'org:my-org/5'

This may indicate:
  - The view was deleted
  - The view ID is incorrect
  - The view belongs to a different project

Tip: List views with --project "org:my-org/5" and omit --project-view-id
```

## Risk Assessment

### Technical Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| View filter format is complex/undocumented | High | Medium | Limit to common filters, clear error messages |
| Filter parsing introduces bugs | Medium | High | Comprehensive tests, gradual rollout |
| Performance regression | Low | Medium | Benchmark all code paths, lazy loading |
| Breaking changes to GraphQL API | Low | High | Version detection, graceful degradation |
| Server-side filtering not available | Medium | Low | Client-side filtering works fine (no regression) |

### User Experience Risks

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|-----------|
| Confusion about view vs. field filtering | Medium | Low | Clear documentation, helpful examples |
| View names with special characters | Medium | Low | Proper escaping, clear error messages |
| Unexpected filter merging behavior | Low | Medium | Document merging logic clearly, examples |
| View renamed in GitHub but scripts use old name | High | Low | Error message lists available views |

### Mitigation Strategies

1. **Comprehensive Testing**
   - Unit tests for all new code
   - Integration tests for end-to-end workflows
   - Manual testing with real projects

2. **Clear Documentation**
   - Detailed usage examples
   - Troubleshooting guide
   - Limitations clearly documented

3. **Helpful Error Messages**
   - List available views when view not found
   - Suggest alternatives when filter parsing fails
   - Provide workarounds for unsupported features

4. **Backward Compatibility**
   - All existing flags continue to work
   - No breaking changes to current behavior
   - View support is purely additive

5. **Gradual Rollout**
   - MVP with common filter types only
   - Gather feedback before expanding support
   - Monitor for issues in production use

## Success Metrics

### Functionality Metrics

**Must Have (MVP):**
- ✅ Users can specify view by name
- ✅ Users can specify view by ID
- ✅ View filters correctly parsed (80% common cases)
- ✅ View filters correctly merged with manual filters
- ✅ Error messages list available views
- ✅ Backward compatible with existing flags
- ✅ No performance regression for non-view users

**Nice to Have (Post-MVP):**
- ✅ Server-side filtering optimization
- ✅ Support for date range filters
- ✅ Support for iteration/milestone filters
- ✅ View discovery command (`list-views`)

### User Experience Metrics

**Qualitative:**
- Users report simpler CLI invocations
- Reduced confusion about field names/values
- Better alignment with GitHub UI terminology
- Fewer support requests about filtering

**Quantitative:**
- >80% of view filter types successfully parsed
- Error messages provide actionable guidance in >90% of cases
- No increase in command execution time for non-view users
- <5% performance overhead for view-based queries

### Code Quality Metrics

- >80% test coverage for new code
- All linting checks pass
- No security vulnerabilities introduced
- Clear, maintainable code structure
- Comprehensive documentation

## Timeline

### Week 1: Core Implementation
- **Days 1-2:** Phases 2-3 (GraphQL queries, filter parsing)
- **Days 3-4:** Phases 4-5 (CLI integration, resolution logic)
- **Day 5:** Phase 6 (Testing, documentation)

### Week 2: Refinement (If Needed)
- **Days 1-2:** Bug fixes, edge cases
- **Days 3-4:** Phase 7 (Optional optimization)
- **Day 5:** Final review, release prep

**Total:** 5-10 days depending on complexity

## Future Enhancements

### Post-MVP Features

1. **View Discovery Command**
   ```bash
   weekly-report-cli project list-views --project "org:my-org/5"
   ```

2. **Advanced Filter Support**
   - Date range filters
   - Iteration/milestone filters
   - Numeric comparisons
   - Regex matching

3. **View Caching**
   - Cache view definitions locally
   - Reduce API calls for repeated usage
   - Invalidation strategy

4. **View Field Mapping**
   - Map view fields to report fields
   - Custom field transformations
   - Aliasing support

5. **View-Based Defaults**
   - Set default view per project
   - Environment variable support
   - Config file support

## Conclusion

Adding GitHub Projects V2 view support will significantly improve the user experience by:
- Aligning with GitHub UI terminology (views vs. fields)
- Reducing manual filter specification
- Providing single source of truth for filters
- Simplifying CLI invocations

The implementation is well-scoped with clear phases, manageable risks, and strong backward compatibility guarantees. The MVP can be delivered in 3.5-5.5 days with optional optimization available as a follow-up.

## Appendix

### Related Documents
- [PROJECT_BOARDS.md](./PROJECT_BOARDS.md) - Current project board integration
- [README.md](../README.md) - Main documentation
- [CLAUDE.md](../CLAUDE.md) - Development guide

### Decision Log

**2025-01-05: Initial Design Decisions**
- Filter merging strategy: Option A (view as base + manual filters)
- View discovery: Option B (error messages only for MVP)
- Filter support scope: Option B (common filters with clear errors)
- Effort estimate: 3.5-5.5 days for MVP

### Open Questions

1. **Q:** What is the exact format of GitHub's view filter JSON?
   - **Status:** To be determined in Phase 3
   - **Impact:** May limit initial filter type support

2. **Q:** Does GitHub support `view.items()` for server-side filtering?
   - **Status:** To be researched in Phase 7
   - **Impact:** Performance optimization opportunity

3. **Q:** Should view names be case-sensitive?
   - **Status:** Recommend case-insensitive matching
   - **Impact:** Better UX, matches typical expectations

4. **Q:** How to handle views with no filters?
   - **Status:** Treat as "fetch all items"
   - **Impact:** Minor edge case, clear behavior

### References

- [GitHub GraphQL API Documentation](https://docs.github.com/en/graphql)
- [GitHub Projects V2 Documentation](https://docs.github.com/en/issues/planning-and-tracking-with-projects)
- [GraphQL Explorer](https://docs.github.com/en/graphql/overview/explorer)
