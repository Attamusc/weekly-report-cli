# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go CLI tool called `weekly-report-cli` that generates weekly status reports by parsing structured data from GitHub issue comments. The tool fetches GitHub issues, extracts status report data using HTML comment markers, and generates markdown tables with optional AI summarization through the GitHub Copilot SDK and CLI.

## Architecture

### Core Pipeline

The application follows a 3-phase pipeline architecture:

1. **Phase A: Data Collection (Parallel)** - Resolve issues, fetch GitHub data, and extract reports without AI
2. **Phase B: Batch Summarization (Batched Requests)** - Summarize eligible updates in chunks of up to 25 items
3. **Phase C: Result Assembly** - Match summaries to issues and render markdown output

This batched approach groups eligible updates into requests of up to 25 items while preserving per-issue fallback behavior.

### Key Components

**Command Structure:**

- `cmd/root.go` - CLI flags and environment configuration
- `cmd/generate.go` - Main pipeline orchestration with worker pools

**Core Modules:**

- `internal/config/` - Configuration management (env vars + CLI flags)
- `internal/input/` - GitHub URL parsing, validation, and unified input resolution
- `internal/projects/` - GitHub Projects V2 integration with GraphQL client, view support, and filtering
- `internal/github/` - GitHub API client with OAuth2 and retry logic
- `internal/report/` - Report extraction from HTML comments and selection logic
- `internal/ai/` - Provider-neutral summarization interface with a Copilot SDK implementation
- `internal/derive/` - Status mapping and date parsing utilities
- `internal/format/` - Markdown table and notes rendering

### Data Flow

**Phase A: Data Collection (Parallel)**

1. **Input Resolution** - Resolve issue references from one of three modes:
   - URL List Mode: Parse GitHub issue URLs from stdin/file
   - Project Board Mode: Fetch issues from GitHub Projects V2 via GraphQL with field filtering
   - Mixed Mode: Combine project board results with manual URL list
2. **Deduplication** - Remove duplicate issue references across all sources
3. **GitHub Fetching** - Fetch issue metadata and comments since specified window (parallel workers)
4. **Report Extraction** - Extract reports using HTML comment markers: `<!-- data key="isReport" value="true" -->`
5. **Report Selection** - Select reports within time window (newest-first)
6. **Status & Date Mapping** - Map trending status and parse target dates

**Phase B: Batch Summarization (Batched Requests)**

7. **Batch AI Summarization** - Collect update texts and send chunked requests through isolated Copilot SDK sessions

- Supports up to 25 items per batch (chunked if larger)
- JSON response format with URL-to-summary mapping
- Markdown fallback parsing if JSON fails
- Falls back to raw text if API fails

**Phase C: Result Assembly**

8. **Summary Application** - Match AI summaries to issues by URL
9. **Rendering** - Render markdown table with status, epic info, target date, and summary

### Input Resolution Architecture

```
CLI Input (--project or --input/stdin)
    ↓
input.ResolveIssueRefs (auto-detects mode)
    ↓
┌─────────────────────────────────────┐
│ Unknown → detect from CLI flags     │
│ URLList → parse from stdin/file     │
│ Project → fetch via GraphQL         │
│ Mixed → combine both sources         │
└─────────────────────────────────────┘
    ↓
If Project Mode:
    projectClientAdapter → projects.Client → GitHub GraphQL API
    → If view specified:
        → projects.FetchProjectViews (fetch all views)
        → Find view by name/ID
        → projects.ParseViewFilter (parse view JSON to FieldFilter)
        → projects.MergeFilters (merge with manual filters if any)
    → projects.FetchProjectItems (paginated, with merged filters)
    → projects.FilterProjectItems (apply filters client-side)
    → []IssueRef
    ↓
Deduplicate & merge with URL list (if mixed mode)
    ↓
Existing pipeline (unchanged)
```

## Development Commands

This project includes a comprehensive Makefile for development tasks:

### Quick Development Cycle

```bash
# Complete development pipeline (recommended)
make all

# Build binary
make build

# Run all tests
make test

# Format, lint, and test code
make check

# Install dependencies
make deps
```

### Build Variants

```bash
# Development build
make build

# Production build (optimized)
make build-prod

# Cross-platform builds
make build-all

# Create release archives
make release
```

### Testing & Quality

```bash
# Run tests with race detection
make test-race

# Generate coverage report
make coverage

# Run security scan
make security

# Check for vulnerabilities
make vuln

# Run benchmarks
make bench
```

### Development Workflow

```bash
# File watching for development
make dev

# Run with sample data
make run-example

# Install development tools
make install-lint

# View all available targets
make help
```

### Legacy Go Commands (still available)

```bash
# Manual Go commands if needed
go mod tidy
go test ./...
go test -race ./...
go build -o weekly-report-cli .
```

## Configuration

### Required Environment Variables

- `GITHUB_TOKEN` - Personal Access Token with scopes:
  - `repo` - For private repository access
  - `read:project` - For GitHub Projects V2 board access (required for project board integration)

### Optional Environment Variables

- `COPILOT_GITHUB_TOKEN` - Separate fine-grained PAT with account-level Copilot Requests permission; classic PATs are unsupported
- `COPILOT_MODEL` - Copilot model (default: `claude-haiku-4.5`); availability depends on account and organization policy
- `COPILOT_CLI_PATH` - SDK-supported executable override when `copilot` is not on `PATH`
- `DISABLE_SUMMARY` - Set to skip Copilot startup and AI summarization
- `AI_TIMEOUT` - Per-request timeout in seconds (default: `120`)

### CLI Usage

```bash
# URL List Mode (traditional)
cat links.txt | weekly-report-cli generate --since-days 7
weekly-report-cli generate --input links.txt --since-days 14 --no-notes

# Project Board Mode (NEW) - simple with defaults
weekly-report-cli generate --project "org:my-org/5"

# Project Board Mode - custom field/values
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Priority" \
  --project-field-values "High,Critical" \
  --since-days 7

# Project View Mode (NEW) - reference pre-configured views
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Blocked Items" \
  --since-days 7

# Project View + Manual Filter
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Current Sprint" \
  --project-field "Priority" \
  --project-field-values "High"

# Mixed Mode
weekly-report-cli generate \
  --project "org:my-org/5" \
  --input critical-issues.txt \
  --since-days 7
```

**Project Board Defaults:**

- Field: "Status"
- Values: "In Progress,Done,Blocked"

## Key Implementation Details

### Report Data Format

Reports are identified by HTML comment marker and use structured data extraction:

```html
<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->🟣 done<!-- data end -->
<!-- data key="target_date" start -->2025-08-06<!-- data end -->
<!-- data key="update" start -->Completed feature implementation<!-- data end -->
```

### Status Mapping

- `🟢/green/on track` → `:green_circle: On Track`
- `🟡/yellow/at risk` → `:yellow_circle: At Risk`
- `🔴/red/blocked/off track` → `:red_circle: Off Track`
- `⚪/white/not started` → `:white_circle: Not Started`
- `🟣/purple/done/complete` → `:purple_circle: Done`

### Concurrency Model

Uses bounded worker pools for parallel GitHub API requests with configurable concurrency limits.

### Error Handling

- GitHub API: Retry logic for 5xx errors and rate limits
- Copilot: Actionable stderr diagnostics and non-AI fallback when startup or inference fails
- Input: Graceful handling of malformed URLs and missing data

## Testing Strategy

Each module should have comprehensive unit tests:

- URL parsing with edge cases and deduplication
- GitHub API mocking with httptest.Server
- Report extraction with exact sample validation
- Copilot adapter with fake runtime and session responses
- End-to-end integration tests with mocked dependencies

## Exit Codes

- `0` - Success
- `2` - No rows produced (valid but empty result)
- `>2` - Fatal errors
