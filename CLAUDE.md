# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Go CLI tool called `gh-epic-updates` that generates weekly status reports by parsing structured data from GitHub issue comments. The tool fetches GitHub issues, extracts status report data using HTML comment markers, and generates markdown tables with optional AI summarization via GitHub Models.

## Architecture

### Core Pipeline
The application follows a 4-phase pipeline architecture:
1. **CLI & Input Processing** - Parse GitHub issue URLs from stdin/file
2. **GitHub Data Fetching** - Fetch issues and comments with retry logic  
3. **Report Extraction & Selection** - Parse structured data from comments
4. **Summarization & Rendering** - Generate markdown output with optional AI summaries

### Key Components

**Command Structure:**
- `cmd/root.go` - CLI flags and environment configuration
- `cmd/generate.go` - Main pipeline orchestration with worker pools

**Core Modules:**
- `internal/config/` - Configuration management (env vars + CLI flags)
- `internal/input/` - GitHub URL parsing and validation
- `internal/github/` - GitHub API client with OAuth2 and retry logic
- `internal/report/` - Report extraction from HTML comments and selection logic
- `internal/ai/` - AI summarization interface with GitHub Models implementation
- `internal/derive/` - Status mapping and date parsing utilities
- `internal/format/` - Markdown table and notes rendering

### Data Flow

1. Parse GitHub issue URLs from input
2. Fetch issue metadata and comments since specified window
3. Extract reports using HTML comment markers: `<!-- data key="isReport" value="true" -->`
4. Select reports within time window (newest-first)
5. Summarize updates using AI (optional)
6. Map trending status to canonical emoji/caption format
7. Render markdown table with status, epic info, target date, and summary

## Development Commands

Since this is a new Go project, standard Go commands will be used:

```bash
# Initialize Go module (when creating)
go mod init github.com/Attamusc/weekly-report-cli

# Add dependencies
go mod tidy

# Build the CLI
go build -o gh-epic-updates .

# Run tests
go test ./...

# Run specific test package
go test ./internal/report

# Run with race detection
go test -race ./...

# Build for production
CGO_ENABLED=0 go build -ldflags="-s -w" -o gh-epic-updates .
```

## Configuration

### Required Environment Variables
- `GITHUB_TOKEN` - Personal Access Token for private repos and GitHub Models API

### Optional Environment Variables  
- `GITHUB_MODELS_BASE_URL` - Default: `https://models.github.ai/v1`
- `GITHUB_MODELS_MODEL` - Default: `gpt-4o-mini`
- `DISABLE_SUMMARY` - Set to disable AI summarization

### CLI Usage
```bash
# Basic usage with stdin
cat links.txt | gh-epic-updates generate --since-days 7

# With file input
gh-epic-updates generate --input links.txt --since-days 14 --no-notes
```

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