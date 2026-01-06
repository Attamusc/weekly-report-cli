# GitHub Projects Views Guide

This guide explains how to use GitHub Projects V2 views with `weekly-report-cli` to generate reports based on pre-configured view filters.

## Table of Contents

- [Overview](#overview)
- [Quick Start](#quick-start)
- [Usage Examples](#usage-examples)
- [How It Works](#how-it-works)
- [View Discovery](#view-discovery)
- [Advanced Usage](#advanced-usage)
- [Troubleshooting](#troubleshooting)
- [Limitations](#limitations)

## Overview

GitHub Projects V2 allows you to create multiple **views** within a project. Each view is a saved configuration that includes filters, layout settings, sorting rules, and custom field visibility.

Instead of manually specifying field filters every time, you can now reference a view by name or ID, and the tool will automatically apply that view's filters to report generation.

### Benefits

- **Simpler Commands**: Reference views by name instead of typing out filter combinations
- **Single Source of Truth**: View filters are defined once in GitHub UI
- **Automatic Sync**: Changes to view filters in GitHub are immediately reflected
- **Better UX**: Aligns with GitHub UI terminology (views vs. fields)

## Quick Start

### Basic View Usage

```bash
# Fetch items from "Blocked Items" view
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Blocked Items" \
  --since-days 7
```

### View with Time Window

```bash
# Generate report for last 14 days using "Current Sprint" view
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Current Sprint" \
  --since-days 14
```

## Usage Examples

### Example 1: Simple View Reference

```bash
# Use the "High Priority" view from your project
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "High Priority" \
  --since-days 7
```

This will:

1. Fetch all views from project `org:my-org/5`
2. Find the view named "High Priority"
3. Parse that view's filter configuration
4. Apply the filters to fetch matching items
5. Generate report for items with updates in last 7 days

### Example 2: View by ID (For Automation)

View IDs are stable identifiers that won't change if the view is renamed. This is useful for automation scripts:

```bash
# Use view ID instead of name (recommended for CI/CD)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view-id "PVT_kwDOABCDEF4567890" \
  --since-days 14
```

**How to get a view ID:**

1. Open your project in GitHub
2. Navigate to the desired view
3. The view ID is in the URL after `?view=`
4. Or use the GitHub GraphQL Explorer to query view IDs

### Example 3: Combining View with Additional Filters

You can start with a view's filters and add additional filtering on top:

```bash
# Start with "Current Sprint" view, narrow down to High priority
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Current Sprint" \
  --project-field "Priority" \
  --project-field-values "High,Critical" \
  --since-days 7
```

**Filter Merging Behavior:**

- View filters act as the **base**
- Manual filters are **added** (AND logic)
- If a manual filter targets the same field as the view, the manual filter **overrides** it

### Example 4: Mixed Mode (View + URL List)

Combine view-based filtering with manual issue URLs:

```bash
# Use view for project items, plus add specific critical issues
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Team Blockers" \
  --input critical-issues.txt \
  --since-days 7
```

Issues are automatically deduplicated across both sources.

### Example 5: User Projects

Views work with user projects too:

```bash
# User project with view
weekly-report-cli generate \
  --project "user:johndoe/3" \
  --project-view "My Weekly Focus" \
  --since-days 7
```

## How It Works

### Data Flow

When you specify `--project-view` or `--project-view-id`:

1. **View Discovery**: Tool fetches all views from the project via GitHub GraphQL API
2. **View Lookup**: Finds the requested view by name (case-insensitive) or ID (exact match)
3. **Filter Parsing**: Parses the view's filter configuration (supports both query string and JSON formats) into field filters
4. **Filter Merging**: Combines view filters with any manual `--project-field` filters (if specified)
5. **Item Fetching**: Fetches project items and applies filters client-side
6. **Report Generation**: Generates report based on filtered items

### Supported View Filter Types

The tool supports common filter types used in GitHub Projects views. GitHub uses a query string filter format:

**Filter Format:**

```
type:Epic,Initiative quarter:FY26Q2
```

**Syntax:**

- Space-separated field:value pairs
- Comma-separated values for multiple values in a field
- Field names are case-sensitive as they appear in GitHub

| Filter Type                 | Support          | Example                    |
| --------------------------- | ---------------- | -------------------------- |
| Field equals value          | ✅ Full          | `type:Epic`                |
| Field with multiple values  | ✅ Full          | `type:Epic,Initiative`     |
| Multiple fields (AND logic) | ✅ Full          | `type:Epic quarter:FY26Q2` |
| Complex boolean logic       | ❌ Not supported | `(A OR B) AND (C OR D)`    |
| Date range filters          | ❌ Not supported | `date>2025-01-01`          |

### Filter Merging Details

When both view filters and manual filters are provided:

```
View Filter:     Status = "Blocked"
Manual Filter:   Priority = "High"
Result:          Status = "Blocked" AND Priority = "High"
```

If the same field is specified in both:

```
View Filter:     Status = "In Progress"
Manual Filter:   Status = "Blocked"
Result:          Status = "Blocked"  (manual overrides)
```

## View Discovery

### Finding Available Views

If you're not sure what views are available in your project, the tool will help you discover them through error messages:

```bash
# Try using a non-existent view
$ weekly-report-cli generate \
    --project "org:my-org/5" \
    --project-view "Invalid Name"

Error: view 'Invalid Name' not found in project 'org:my-org/5'

Available views:
  - Blocked Items
  - Current Sprint
  - Q1 Roadmap
  - Completed Work
  - High Priority Tasks

Tip: View names are case-insensitive
```

### View Naming Tips

- View names are matched **case-insensitively** for better UX
- Whitespace is significant: `"Current Sprint"` ≠ `"CurrentSprint"`
- Use quotes for view names with spaces: `--project-view "My View"`
- Avoid special shell characters in view names

## Advanced Usage

### Combining Multiple Features

```bash
# Complex example: view + additional filter + URL list + custom options
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Sprint 23 Active" \
  --project-field "Team" \
  --project-field-values "Backend,Infrastructure" \
  --input critical-blockers.txt \
  --since-days 7 \
  --concurrency 10 \
  --no-notes
```

This command:

1. Fetches items matching "Sprint 23 Active" view filters
2. Further filters to Team = "Backend" OR "Infrastructure"
3. Adds issues from `critical-blockers.txt`
4. Deduplicates across all sources
5. Looks for updates in last 7 days
6. Uses 10 concurrent workers
7. Skips notes section in output

### Performance Considerations

**View Overhead:**

- Fetching views adds ~1 extra API call per execution
- Minimal impact: typically <100ms
- Views are fetched once per command execution

**Caching:**

- Views are NOT cached between executions
- Future enhancement: local view cache with TTL

**Optimization Tips:**

- Use view IDs in automation (skips name lookup)
- Keep `--project-max-items` reasonable (default: 100)
- Consider `--concurrency` for large projects (default: 5)

### Backward Compatibility

All existing commands continue to work exactly as before:

```bash
# Old way (still works)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --since-days 7

# New way (equivalent)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Blocked Items" \
  --since-days 7
```

Views are **purely additive** - no breaking changes to existing behavior.

## Troubleshooting

### View Not Found

**Problem:**

```
Error: view 'My View' not found in project 'org:my-org/5'
```

**Solutions:**

1. Check the view name spelling (case-insensitive but whitespace matters)
2. Verify the view exists in the project (GitHub UI > Project > Views)
3. Ensure you have `read:project` scope in your `GITHUB_TOKEN`
4. Check the error message for list of available views

### View ID Not Found

**Problem:**

```
Error: view with ID 'PVT_kwDOABCDEF' not found
```

**Possible Causes:**

- View was deleted from the project
- View ID is incorrect/typo
- View belongs to a different project

**Solution:**

- Use view name instead: `--project-view "View Name"`
- Verify view exists in GitHub UI
- Check project URL is correct

### Unsupported Filter Type

**Problem:**

```
Error: unable to parse view filter for 'Complex View'
Reason: Complex boolean logic is not yet supported
```

**Workaround:**
Use manual field filters instead:

```bash
# Instead of using the complex view, specify filters manually
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked,At Risk"
```

Supported filter types are listed in [Supported View Filter Types](#supported-view-filter-types).

### Empty Filter View

**Problem:**
View has no filters configured (fetches all items).

**Behavior:**

- Empty view filter = fetch all items (up to `--project-max-items`)
- This is expected behavior
- Add manual filters if you need to narrow down:

  ```bash
  --project-view "All Items" \
  --project-field "Status" \
  --project-field-values "In Progress"
  ```

### Permission Denied

**Problem:**

```
Error: failed to fetch project views: resource not accessible by integration
```

**Solution:**

1. Ensure `GITHUB_TOKEN` includes `read:project` scope
2. Regenerate token if needed (GitHub Settings > Developer settings)
3. Verify token has access to the organization/project:

   ```bash
   export GITHUB_TOKEN=ghp_your_new_token_here
   ```

### View Name with Special Characters

**Problem:**
View name contains quotes, backslashes, or other special characters.

**Solution:**
Use proper shell escaping:

```bash
# View name: My "Special" View
--project-view 'My "Special" View'

# Or escape quotes
--project-view "My \"Special\" View"

# Alternative: use view ID (no escaping needed)
--project-view-id "PVT_kwDOABCDEF"
```

## Limitations

### Current Limitations

1. **Filter Types**: Only common filter types are supported (see [Supported View Filter Types](#supported-view-filter-types))
2. **No Server-Side Filtering**: All filtering happens client-side (fetches all items then filters)
3. **No View Caching**: Views are fetched on every execution
4. **No View Discovery Command**: Use error messages to discover views (dedicated command planned for future)

### Future Enhancements

Planned features (not in current MVP):

- **Server-Side Filtering**: Explore GitHub API support for `view.items()` query
- **View Caching**: Local cache with TTL to reduce API calls
- **View Discovery**: `weekly-report-cli project list-views --project "org:my-org/5"`
- **Advanced Filters**: Date ranges, iterations, numeric comparisons
- **View Defaults**: Set default view per project via config file

### Workarounds for Limitations

**Complex Filters:**

- Use manual `--project-field` flags instead of views
- Combine multiple simple views via URL list mode

**Performance with Large Projects:**

- Use `--project-max-items` to limit fetch size
- Increase `--concurrency` for faster processing
- Use field filters to narrow scope before view application

**View Management:**

- Keep view names simple (no special characters)
- Document view IDs for automation
- Create "report-specific" views for common use cases

## Related Documentation

- [PROJECT_BOARDS.md](./PROJECT_BOARDS.md) - GitHub Projects board integration guide
- [README.md](../README.md) - Main documentation
- [CLAUDE.md](../CLAUDE.md) - Development guide

## Examples Summary

**Quick Reference:**

```bash
# By view name
--project-view "View Name"

# By view ID
--project-view-id "PVT_kwDOABCDEF"

# View + additional filter
--project-view "My View" \
--project-field "Priority" \
--project-field-values "High"

# View + URL list
--project-view "Team View" \
--input issues.txt

# User project
--project "user:johndoe/3" \
--project-view "Personal Backlog"
```

---

**Need Help?**

- Check the [Troubleshooting](#troubleshooting) section
- Review [Usage Examples](#usage-examples)
- See [PROJECT_BOARDS.md](./PROJECT_BOARDS.md) for project board basics
- Open an issue on GitHub for bugs or feature requests
