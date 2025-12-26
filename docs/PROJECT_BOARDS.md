# GitHub Projects Board Integration Guide

This guide covers the GitHub Projects V2 board integration feature, which allows you to automatically fetch issues from project boards using field-based filtering instead of manually maintaining URL lists.

## Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Quick Start](#quick-start)
- [Project URL Formats](#project-url-formats)
- [Filtering Options](#filtering-options)
- [Usage Examples](#usage-examples)
- [Filter Behavior](#filter-behavior)
- [Mixed Mode](#mixed-mode)
- [Performance Considerations](#performance-considerations)
- [Troubleshooting](#troubleshooting)
- [API Details](#api-details)

## Overview

The project board integration feature enables three input modes:

1. **URL List Mode** (Traditional): Provide GitHub issue URLs directly
2. **Project Board Mode** (NEW): Fetch issues from a GitHub Projects V2 board with field filtering
3. **Mixed Mode** (NEW): Combine both project board filtering and manual URL lists

This feature is particularly useful when:
- You manage work using GitHub Projects boards
- You want to generate reports for specific statuses (e.g., "Blocked", "In Progress")
- You need to dynamically fetch issues based on project field values
- You want to combine automated project filtering with manually curated issue lists

## Prerequisites

### GitHub Token Scopes

Your GitHub Personal Access Token **must** have the following scopes:

- `repo` - For accessing private repositories (or `public_repo` for public repos only)
- **`read:project`** - **Required for project board access** (NEW)

### Setting Up Token

1. Go to [GitHub Settings > Developer settings > Personal access tokens](https://github.com/settings/tokens)
2. Generate a new token (classic) or fine-grained token
3. Select the required scopes:
   - ✅ `repo` (Full control of private repositories)
   - ✅ `read:project` (Read access to projects)
4. Set the token as an environment variable:
   ```bash
   export GITHUB_TOKEN=your_token_here
   ```

> **Note**: Without the `read:project` scope, you'll receive a `401 Unauthorized` error when attempting to use project board features.

## Quick Start

### Simplest Usage (Using Defaults)

```bash
# Fetch issues using defaults (Status field: "In Progress,Done,Blocked")
weekly-report-cli generate --project "org:my-org/5"
```

This is the easiest way to get started! The tool automatically filters by:
- **Field**: "Status" 
- **Values**: "In Progress", "Done", "Blocked"

### Basic Project Board Usage

```bash
# Fetch all issues from a project board with specific status
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --since-days 7
```

### Multiple Status Values

```bash
# Fetch issues with multiple status values (OR logic)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "In Progress,Blocked,At Risk" \
  --since-days 14
```

### Multiple Field Filters

```bash
# Filter by multiple fields (AND logic between fields)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "In Progress,Blocked" \
  --project-field "Priority" \
  --project-field-values "High,Critical" \
  --since-days 7
```

This fetches issues where:
- Status is "In Progress" **OR** "Blocked" **AND**
- Priority is "High" **OR** "Critical"

## Project URL Formats

The tool supports multiple project reference formats:

### Organization Projects

```bash
# Full URL format
--project "https://github.com/orgs/my-org/projects/5"

# Short format (recommended)
--project "org:my-org/5"
```

### User Projects

```bash
# Full URL format
--project "https://github.com/users/username/projects/3"

# Short format (recommended)
--project "user:username/3"
```

### Which Format to Use?

- **Short format** (`org:name/number` or `user:name/number`) is recommended for CLI usage
- **Full URL** is useful when copying directly from your browser
- Both formats work identically

## Filtering Options

### Command-Line Flags

| Flag | Description | Default | Required |
|------|-------------|---------|----------|
| `--project` | Project URL or reference | - | Yes (for project mode) |
| `--project-field` | Field name to filter on | `"Status"` | No |
| `--project-field-values` | Comma-separated values to match | `"In Progress,Done,Blocked"` | No |
| `--project-include-prs` | Include pull requests in results | `false` | No |
| `--project-max-items` | Maximum items to fetch | `100` | No |

### Field Types

The tool supports various GitHub Projects field types:

| Field Type | Matching Behavior | Example |
|------------|-------------------|---------|
| **Text** | Case-insensitive substring match | "Sprint" matches "Sprint 1", "Sprint 2" |
| **Single Select** | Exact match | "Done" matches only "Done" |
| **Date** | String comparison | "2025-01-15" |
| **Number** | String comparison | "3" |

### Special Filtering Rules

1. **Draft Issues**: Always excluded from results
2. **Pull Requests**: Excluded by default (use `--project-include-prs` to include)
3. **Empty Fields**: Items without the specified field are excluded
4. **Case Sensitivity**: Text field matching is case-insensitive

## Usage Examples

### Example 1: Default Status Report

Generate a report using defaults (Status: "In Progress,Done,Blocked"):

```bash
# Simplest usage - just specify the project!
weekly-report-cli generate --project "org:my-company/12"
```

### Example 2: Weekly Blocked Items Report

Generate a report for all blocked items in the past 7 days:

```bash
weekly-report-cli generate \
  --project "org:my-company/12" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --since-days 7
```

### Example 3: High Priority In-Progress Items

Fetch high-priority items currently in progress:

```bash
weekly-report-cli generate \
  --project "org:my-company/12" \
  --project-field "Status" \
  --project-field-values "In Progress" \
  --project-field "Priority" \
  --project-field-values "High,Critical" \
  --since-days 14
```

### Example 4: Sprint Report with Pull Requests

Include both issues and PRs for a specific sprint:

```bash
weekly-report-cli generate \
  --project "org:my-company/12" \
  --project-field "Sprint" \
  --project-field-values "Sprint 23" \
  --project-include-prs \
  --since-days 7
```

### Example 5: Completed Items Report

Generate a report of recently completed items:

```bash
weekly-report-cli generate \
  --project "org:my-company/12" \
  --project-field "Status" \
  --project-field-values "Done,Completed" \
  --since-days 30
```

### Example 6: Team-Specific Items

Filter by team or assignee using custom fields:

```bash
weekly-report-cli generate \
  --project "org:my-company/12" \
  --project-field "Team" \
  --project-field-values "Backend,Infrastructure" \
  --project-field "Status" \
  --project-field-values "In Progress,Blocked" \
  --since-days 7
```

## Filter Behavior

### OR Logic Within Field Values

When you specify multiple values for a single field, they are combined with **OR** logic:

```bash
--project-field "Status" \
--project-field-values "In Progress,Blocked,At Risk"
```

This matches items where Status is:
- "In Progress" **OR**
- "Blocked" **OR**
- "At Risk"

### AND Logic Between Different Fields

When you specify multiple fields, they are combined with **AND** logic:

```bash
--project-field "Status" \
--project-field-values "In Progress" \
--project-field "Priority" \
--project-field-values "High"
```

This matches items where:
- Status is "In Progress" **AND**
- Priority is "High"

### Complex Example

```bash
--project-field "Status" \
--project-field-values "In Progress,Blocked" \
--project-field "Priority" \
--project-field-values "High,Critical" \
--project-field "Team" \
--project-field-values "Backend"
```

This matches items where:
- (Status is "In Progress" **OR** "Blocked") **AND**
- (Priority is "High" **OR** "Critical") **AND**
- (Team is "Backend")

## Mixed Mode

You can combine project board filtering with manual URL lists:

### Use Case

- Automatically fetch items from your project board using defaults
- Add specific high-priority issues that might not be on the board
- Generate a single unified report

### Example

```bash
# Create a file with additional issues
cat > critical-issues.txt << EOF
https://github.com/my-org/repo/issues/123
https://github.com/my-org/repo/issues/456
EOF

# Generate report combining both sources (using defaults for project)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --input critical-issues.txt \
  --since-days 7
```

### Deduplication

Issues are automatically deduplicated across both sources. If an issue appears in both:
- The project board results
- Your manual URL list

It will only appear **once** in the final report.

## Performance Considerations

### Pagination

The tool uses cursor-based pagination to fetch project items:
- Default page size: 100 items
- Automatically handles pagination for large projects
- Use `--project-max-items` to limit results

### Large Projects

For projects with many items:

```bash
# Limit to first 50 items (faster)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --project-max-items 50 \
  --since-days 7
```

### Rate Limiting

The GraphQL client includes:
- Automatic retry with exponential backoff
- Jittered delays to avoid thundering herd
- Rate limit detection and handling

If you encounter rate limits:
- Reduce `--project-max-items`
- Add delays between runs
- Use more specific filters to reduce result set

## Troubleshooting

### Error: 401 Unauthorized

```
Error: failed to fetch project: 401 Unauthorized - Check token has 'read:project' scope
```

**Solution**: Your GitHub token is missing the `read:project` scope. Regenerate your token with the required scope.

### Error: 404 Not Found

```
Error: failed to fetch project: 404 Not Found - Project may not exist or token lacks access
```

**Possible causes**:
1. Project URL/reference is incorrect
2. Project doesn't exist
3. Token doesn't have access to the organization/user's projects
4. Project number is wrong

**Solutions**:
- Verify the project URL by visiting it in your browser
- Check the project number in the URL
- Ensure your token has access to the organization

### Error: No Items Match Filters

```
No qualifying reports found in the specified time window
```

**Possible causes**:
1. No items match your field filters
2. Matched items don't have report data in comments
3. `--since-days` window doesn't include any updates

**Solutions**:
- Verify field names and values are correct (case matters for single-select)
- Check that filtered issues have report comment data
- Increase `--since-days` to widen the time window
- Test without filters first: `--project "org:my-org/5"`

### Error: Field Not Found

If a field name is misspelled or doesn't exist, no items will match. To debug:

1. List all project fields using GitHub's web interface
2. Copy field names exactly as they appear
3. Remember: Field names are case-sensitive for single-select fields

### Debugging Tips

```bash
# Test project access (no filters)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --since-days 7

# Test with broad filters
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-max-items 10 \
  --since-days 30

# Add verbosity (if implemented)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --since-days 7 \
  -v
```

## API Details

### GraphQL API

The project board integration uses GitHub's GraphQL API v4:
- **Endpoint**: `https://api.github.com/graphql`
- **Authentication**: Bearer token (from `GITHUB_TOKEN`)
- **API Version**: GitHub Projects V2

### Query Structure

The tool constructs GraphQL queries to:
1. Fetch project metadata (title, owner)
2. Retrieve project items with pagination
3. Extract field values for each item
4. Resolve linked issues/PRs

### Data Fetched

For each project item, the tool retrieves:
- **Item ID**: Unique identifier
- **Content Type**: Issue or Pull Request
- **Issue/PR Number**: For URL construction
- **Repository**: Owner and name
- **Field Values**: All custom field values

### Field Value Types

The GraphQL API returns different structures for each field type:
- **Text**: Direct string value
- **Single Select**: Option name
- **Date**: ISO 8601 date string
- **Number**: Numeric value

The tool normalizes these into a consistent format for filtering.

## Advanced Usage

### Environment Variables

You can set defaults via environment variables:

```bash
# Set default project
export WEEKLY_REPORT_PROJECT="org:my-org/5"

# Use in command (if supported)
weekly-report-cli generate \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --since-days 7
```

> **Note**: Check `weekly-report-cli generate --help` for supported environment variables.

### Scripting

Create reusable scripts for common report types:

```bash
#!/bin/bash
# blocked-items.sh - Generate blocked items report

weekly-report-cli generate \
  --project "org:my-company/12" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --since-days "${DAYS:-7}" \
  "$@"
```

Usage:
```bash
# Default (7 days)
./blocked-items.sh

# Custom time window
DAYS=14 ./blocked-items.sh

# Additional flags
./blocked-items.sh --no-notes
```

## Best Practices

1. **Use Specific Filters**: More specific filters = faster queries
2. **Set Reasonable Limits**: Use `--project-max-items` for large boards
3. **Combine with Time Windows**: Use `--since-days` to limit comment fetching
4. **Monitor Rate Limits**: Be mindful of GraphQL API rate limits
5. **Cache Token**: Store `GITHUB_TOKEN` in your shell profile
6. **Use Short Format**: Prefer `org:name/number` over full URLs
7. **Test Incrementally**: Start without filters, then add one at a time
8. **Document Field Names**: Keep a list of your project's field names

## Migration from URL Lists

If you're currently using URL list mode and want to migrate:

### Step 1: Identify Your Use Case

- **Before**: Manually maintain `issues.txt` with URLs
- **After**: Automatically fetch from project board

### Step 2: Map to Project Fields

Identify which project board fields correspond to your current filtering:
- Status fields → `--project-field "Status"`
- Priority tags → `--project-field "Priority"`
- Sprint labels → `--project-field "Sprint"`

### Step 3: Test Side-by-Side

```bash
# Old method
cat issues.txt | weekly-report-cli generate --since-days 7

# New method (test)
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "In Progress,Blocked" \
  --since-days 7
```

### Step 4: Use Mixed Mode During Transition

Keep your URL list as a backup while testing:

```bash
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Status" \
  --project-field-values "Blocked" \
  --input issues.txt \
  --since-days 7
```

### Step 5: Full Migration

Once confident, remove URL list and use project board exclusively.

## Conclusion

The GitHub Projects board integration provides a powerful way to automate issue report generation based on your project board's structure. By leveraging field-based filtering, you can:

- Eliminate manual URL list maintenance
- Generate dynamic reports based on current project state
- Combine automated and manual issue selection
- Integrate with your existing project management workflow

For additional help, see:
- [README.md](../README.md) - Main documentation
- [CLAUDE.md](../CLAUDE.md) - Development guide
- GitHub Issues - Report bugs or request features
