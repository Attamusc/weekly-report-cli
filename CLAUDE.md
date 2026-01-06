# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go CLI tool called `weekly-report-cli` that generates weekly status reports by parsing structured data from GitHub issue comments. The tool fetches GitHub issues, extracts status report data using HTML comment markers, and generates markdown tables with optional AI summarization via GitHub Models.

## Architecture

### Core Pipeline
The application follows a 4-phase pipeline architecture:
1. **CLI & Input Processing** - Resolve GitHub issues from URL lists, project boards, or mixed mode
2. **GitHub Data Fetching** - Fetch issues and comments with retry logic  
3. **Report Extraction & Selection** - Parse structured data from comments
4. **Summarization & Rendering** - Generate markdown output with optional AI summaries

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
- `internal/ai/` - AI summarization interface with GitHub Models implementation
- `internal/derive/` - Status mapping and date parsing utilities
- `internal/format/` - Markdown table and notes rendering

### Data Flow

1. **Input Resolution** - Resolve issue references from one of three modes:
   - URL List Mode: Parse GitHub issue URLs from stdin/file
   - Project Board Mode: Fetch issues from GitHub Projects V2 via GraphQL with field filtering
   - Mixed Mode: Combine project board results with manual URL list
2. **Deduplication** - Remove duplicate issue references across all sources
3. **GitHub Fetching** - Fetch issue metadata and comments since specified window
4. **Report Extraction** - Extract reports using HTML comment markers: `<!-- data key="isReport" value="true" -->`
5. **Report Selection** - Select reports within time window (newest-first)
6. **AI Summarization** - Summarize updates using AI (optional)
7. **Status Mapping** - Map trending status to canonical emoji/caption format
8. **Rendering** - Render markdown table with status, epic info, target date, and summary

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
- `GITHUB_MODELS_BASE_URL` - Default: `https://models.github.ai`
- `GITHUB_MODELS_MODEL` - Default: `gpt-4o-mini`
- `DISABLE_SUMMARY` - Set to disable AI summarization

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
- AI API: Jittered backoff for 429 responses  
- Input: Graceful handling of malformed URLs and missing data

## Testing Strategy

Each module should have comprehensive unit tests:
- URL parsing with edge cases and deduplication
- GitHub API mocking with httptest.Server
- Report extraction with exact sample validation
- AI client with fake server responses
- End-to-end integration tests with mocked dependencies

## Exit Codes
- `0` - Success
- `2` - No rows produced (valid but empty result)
- `>2` - Fatal errors