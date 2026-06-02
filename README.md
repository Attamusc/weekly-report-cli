# Weekly Report CLI

A Go CLI tool that generates weekly status reports by parsing structured data from GitHub issue comments. The tool fetches GitHub issues, extracts status report data using HTML comment markers, and generates markdown tables with optional AI summarization via GitHub Models.

## Features

- **GitHub Integration**: Fetches issues and comments using OAuth2 authentication
- **GitHub Projects V2**: Direct integration with project boards and views via GraphQL
- **Structured Data Parsing**: Extracts reports from HTML comment markers in GitHub issues
- **Batch AI Summarization**: Single-request AI summarization for all updates (avoids rate limits)
- **Flexible Input**: Accepts GitHub issue URLs from stdin, files, or project boards
- **Concurrent Processing**: Parallel data fetching with configurable worker pools
- **Status Mapping**: Maps various status indicators to standardized emoji format
- **Markdown Output**: Generates clean markdown tables with status, epic info, and summaries

## Installation

### Prerequisites
- Go 1.24.2 or later
- GitHub Personal Access Token

### Build from Source
```bash
git clone https://github.com/Attamusc/weekly-report-cli
cd weekly-report-cli
make build
```

### Quick Start (Recommended)
```bash
# Complete build pipeline with all checks
make all

# Development build only
make build

# Production build (optimized)
make build-prod
```

### Cross-Platform Builds
```bash
# Build for multiple platforms (Linux, macOS, Windows)
make build-all

# Create release archives
make release
```

### Alternative: Manual Go Build
```bash
# If you prefer not to use Make
go build -o weekly-report-cli .

# Production build
CGO_ENABLED=0 go build -ldflags="-s -w" -o weekly-report-cli .
```

## GitHub Action

Use `weekly-report-cli` directly in your GitHub Actions workflows without installing anything.

### Basic Usage

```yaml
- uses: Attamusc/weekly-report-cli@v1
  with:
    github-token: ${{ secrets.GITHUB_TOKEN }}
    project: 'org:my-org/5'
    since-days: 7
```

### Full Example: Weekly Report to Slack

```yaml
name: Weekly Status Report

on:
  schedule:
    - cron: '0 9 * * 1'  # Every Monday at 9 AM UTC
  workflow_dispatch:

jobs:
  generate-report:
    runs-on: ubuntu-latest
    steps:
      - uses: Attamusc/weekly-report-cli@v1
        id: report
        with:
          github-token: ${{ secrets.PROJECT_TOKEN }}
          project: 'org:my-org/5'
          project-view: 'Current Sprint'
          since-days: 7

      - name: Post report to Slack
        uses: slackapi/slack-github-action@v1
        with:
          channel-id: 'C0123456789'
          payload: |
            {
              "text": "Weekly Status Report",
              "blocks": [
                {
                  "type": "section",
                  "text": {
                    "type": "mrkdwn",
                    "text": "${{ steps.report.outputs.report }}"
                  }
                }
              ]
            }
        env:
          SLACK_BOT_TOKEN: ${{ secrets.SLACK_BOT_TOKEN }}

      - name: Save report as artifact
        uses: actions/upload-artifact@v4
        with:
          name: weekly-report
          path: ${{ steps.report.outputs.report-file }}
```

### Action Inputs

| Input | Description | Default |
|-------|-------------|---------|
| `github-token` | GitHub token with `repo` and `read:project` scopes | **Required** |
| `version` | Version of weekly-report-cli to use | `latest` |
| `project` | GitHub Project board URL or identifier | - |
| `project-field` | Field name to filter by | - |
| `project-field-values` | Comma-separated values to match | - |
| `project-view` | GitHub project view name | - |
| `project-view-id` | GitHub project view ID | - |
| `project-include-prs` | Include pull requests | `false` |
| `project-max-items` | Maximum items to fetch | `100` |
| `input-file` | Path to file containing issue URLs | - |
| `since-days` | Number of days to look back | `7` |
| `concurrency` | Number of concurrent workers | `4` |
| `no-notes` | Disable notes section | `false` |
| `no-summary` | Disable AI summarization | `false` |
| `summary-prompt` | Custom AI summarization prompt | - |
| `verbose` | Enable verbose output | `false` |
| `quiet` | Suppress all progress output | `false` |

### Action Outputs

| Output | Description |
|--------|-------------|
| `report` | The generated markdown report content |
| `report-file` | Path to the report file |

### Version Pinning

```yaml
# Use latest v1.x.x (recommended)
uses: Attamusc/weekly-report-cli@v1

# Use specific minor version
uses: Attamusc/weekly-report-cli@v1.2

# Use exact version
uses: Attamusc/weekly-report-cli@v1.2.3
```

## Configuration

### Environment Variables

#### Required
- `GITHUB_TOKEN` - Personal Access Token for GitHub API access and GitHub Models

#### Optional
- `GITHUB_MODELS_BASE_URL` - Base URL for the AI completions endpoint (default: `https://models.github.ai`)
- `GITHUB_MODELS_MODEL` - AI model to use (default: `gpt-4o-mini`)
- `DISABLE_SUMMARY` - Set to any value to disable AI summarization

**Supported endpoint configurations:**

| Endpoint | Base URL | Example model |
|---|---|---|
| GitHub Models (default) | `https://models.github.ai` | `openai/gpt-4o-mini` |
| GitHub Copilot API | `https://api.githubcopilot.com` | `claude-haiku-4.5` |

When `GITHUB_MODELS_BASE_URL` is set to `https://api.githubcopilot.com`, the client automatically uses the correct path (`/chat/completions`) and sends the required `Copilot-Integration-Id: vscode-chat` header. This allows access to Anthropic (Haiku, Sonnet) and Google (Gemini) models available through your Copilot seat.

> **Note**: The Copilot completions endpoint is not officially documented for third-party CLI use. It works with a classic PAT on a Copilot-seated account, but may change without notice. GitHub Actions `GITHUB_TOKEN` does **not** work with this endpoint.

### Setting up GitHub Token
1. Go to GitHub Settings > Developer settings > Personal access tokens
2. Generate a new token with the following scopes:
   - `repo` (for private repositories)
   - `public_repo` (for public repositories)
   - `read:project` (for GitHub Projects V2 board access) **← NEW: Required for project board integration**
3. Set the token as an environment variable:
   ```bash
   export GITHUB_TOKEN=your_token_here
   ```

> **Note**: The `read:project` scope is only required if you plan to use the GitHub Projects board integration feature. It is not needed for the traditional URL list input mode.

## Usage

### Command Line Interface

```bash
# Basic usage with stdin (URL list mode)
cat links.txt | weekly-report-cli generate --since-days 7

# With file input (URL list mode)
weekly-report-cli generate --input links.txt --since-days 14

# Disable AI summarization and notes
weekly-report-cli generate --input links.txt --no-notes

# Custom concurrency
weekly-report-cli generate --input links.txt --concurrency 8

# GitHub Projects board integration (NEW) - uses defaults
weekly-report-cli generate --project "org:my-org/5"

# With custom field and values
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-field "Priority" \
  --project-field-values "High,Critical" \
  --since-days 7

# Using project views (NEW) - reference pre-configured views
weekly-report-cli generate \
  --project "org:my-org/5" \
  --project-view "Blocked Items" \
  --since-days 7

# Mixed mode: Combine project board + URL list
weekly-report-cli generate \
  --project "org:my-org/5" \
  --input critical-issues.txt \
  --since-days 14
```


## Architecture

### Core Pipeline
The application follows a 4-phase pipeline architecture:

1. **CLI & Input Processing** - Parse GitHub issue URLs from stdin/file
2. **GitHub Data Fetching** - Fetch issues and comments with retry logic
3. **Report Extraction & Selection** - Parse structured data from comments
4. **Summarization & Rendering** - Generate markdown output with optional AI summaries

### Project Structure

```
.
├── cmd/                    # CLI commands
│   ├── root.go            # Root command and global flags
│   └── generate.go        # Main generate command
├── internal/
│   ├── ai/                # AI summarization
│   │   ├── summarizer.go  # Interface definition
│   │   └── ghmodels.go    # GitHub Models implementation
│   ├── config/            # Configuration management
│   │   └── config.go      # Environment and CLI flag handling
│   ├── derive/            # Data transformation utilities
│   │   ├── date.go        # Date parsing and formatting
│   │   └── status.go      # Status mapping and normalization
│   ├── format/            # Output formatting
│   │   ├── markdown.go    # Markdown table generation
│   │   └── notes.go       # Notes section formatting
│   ├── github/            # GitHub API integration
│   │   ├── client.go      # OAuth2 client setup
│   │   └── issues.go      # Issue and comment fetching
│   ├── input/             # Input processing
│   │   ├── links.go       # URL parsing and validation
│   │   └── resolver.go    # Unified input resolution (URL list + projects)
│   ├── projects/          # GitHub Projects V2 integration (NEW)
│   │   ├── types.go       # Data structures for projects
│   │   ├── parser.go      # Project URL parsing
│   │   ├── query.go       # GraphQL query templates
│   │   ├── client.go      # GraphQL client with pagination
│   │   ├── filter.go      # Field-based filtering logic
│   │   └── view_filter.go # View filter parsing and merging (NEW)
│   └── report/            # Report extraction and processing
│       ├── extract.go     # HTML comment parsing
│       └── select.go      # Time window filtering
├── docs/                  # Documentation
│   ├── PROJECT_BOARDS.md  # Project board usage guide
│   └── PROJECT_VIEWS.md   # Project views usage guide (NEW)
├── go.mod                 # Go module definition
├── go.sum                 # Dependency checksums
└── main.go               # Application entry point
```

### Key Components

- **GitHub Client**: OAuth2-authenticated client with retry logic for rate limits
- **Projects Client**: GraphQL client for GitHub Projects V2 API with cursor-based pagination
- **Input Resolver**: Unified input resolution supporting URL lists, project boards, and mixed mode
- **Report Extractor**: Parses structured data from HTML comments using regex
- **Status Mapper**: Normalizes various status formats to standard emoji representations
- **AI Summarizer**: Interface-based design supporting multiple AI providers
- **Worker Pools**: Concurrent processing of GitHub API requests

## Development

This project includes a comprehensive Makefile for all development tasks.

### Quick Development Workflow
```bash
# Complete development pipeline (recommended)
make all

# Quick development cycle
make check build

# Development with file watching (requires entr)
make dev

# View all available commands
make help
```

### Running Tests
```bash
# Run all tests
make test

# Run tests with race detection
make test-race

# Generate coverage report (HTML)
make coverage

# Show coverage in terminal
make coverage-text

# Run benchmarks
make bench
```

### Code Quality
```bash
# Format code
make fmt

# Run linter (installs if missing)
make lint

# Run go vet
make vet

# Run all quality checks
make check

# Security scanning
make security

# Vulnerability checking
make vuln
```

### Dependency Management
```bash
# Install/update dependencies
make deps

# Install development tools
make install-lint
```

### Alternative: Manual Go Commands
```bash
# If you prefer direct Go commands
go test ./...
go test -race ./...
go test -cover ./...
go mod tidy
```

### Code Structure Guidelines
- Each internal package has a specific responsibility
- Interfaces are used for external dependencies (AI, GitHub API)
- Comprehensive test coverage for all parsing and transformation logic
- Error handling includes context and actionable information

## Error Handling

The tool handles various error conditions gracefully:

- **GitHub API Errors**: Automatic retry for 5xx errors and rate limits
- **AI API Errors**: Jittered backoff for 429 responses
- **Input Validation**: Clear error messages for malformed URLs
- **Missing Data**: Graceful handling of incomplete report data
- **Network Issues**: Timeout handling and connection retry logic

### Exit Codes
- `0` - Success
- `2` - No rows produced (valid but empty result)
- `>2` - Fatal errors (API failures, invalid configuration, etc.)

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes with tests
4. Run the complete test suite: `make check`
5. Submit a pull request

### Development Workflow
```bash
# Set up development environment
make deps
make install-lint

# During development
make dev  # File watching mode

# Before committing
make check  # Runs fmt, vet, lint, test
make coverage  # Verify test coverage

# Build and test everything
make all
```

### Code Style
- Follow standard Go formatting (use `make fmt`)
- Add comprehensive tests for new functionality
- Update documentation for API changes
- Use meaningful commit messages
- Ensure all quality checks pass (`make check`)

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Troubleshooting

### Common Issues

**Authentication Errors**
```bash
Error: GitHub API authentication failed
```
- Verify your `GITHUB_TOKEN` is set correctly
- Check that the token has appropriate repository access permissions
- If using project boards, ensure the token has the `read:project` scope

**Project Board Errors**
```bash
Error: failed to fetch project: 401 Unauthorized - Check token has 'read:project' scope
```
- Your GitHub token is missing the `read:project` scope
- Regenerate your token with the required scope and update the `GITHUB_TOKEN` environment variable

```bash
Error: failed to fetch project: 404 Not Found - Project may not exist or token lacks access
```
- Verify the project URL/reference is correct
- Ensure your token has access to the organization or user's projects
- Check that the project number is correct

**No Reports Found**
```bash
No qualifying reports found in the specified time window
```
- Verify that GitHub issues contain properly formatted HTML comment markers
- Check that the `--since-days` parameter includes the time period of your reports
- Ensure comments contain the `<!-- data key="isReport" value="true" -->` marker

**AI Summarization Failures**
```bash
Failed to generate AI summary: API rate limit exceeded
```
- The tool will continue without AI summaries if the service is unavailable
- Set `DISABLE_SUMMARY=true` to skip AI processing entirely
- Check GitHub Models API quota and rate limits

### Debug Mode
For debugging, you can examine the raw data extraction by looking at the test files or adding debug output to the extraction functions.