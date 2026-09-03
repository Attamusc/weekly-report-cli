# Weekly Report CLI

A Go CLI tool that generates weekly status reports by parsing structured data from GitHub issue comments. The tool fetches GitHub issues, extracts status report data using HTML comment markers, and generates markdown tables with optional AI summarization through the GitHub Copilot SDK.

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
- A GitHub token for the repositories and projects included in the report
- [GitHub Copilot CLI](https://docs.github.com/en/copilot/how-tos/copilot-cli/set-up-copilot-cli/install-copilot-cli) for AI summaries when running the binary directly

Install Copilot CLI and ensure `copilot` is on `PATH`. If it is installed elsewhere, set the SDK-supported `COPILOT_CLI_PATH` to the executable. You can omit this prerequisite when using `DISABLE_SUMMARY`; the composite action installs Copilot CLI automatically.

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

The preferred configuration uses the workflow's built-in token for Copilot and a separate token for report data. The calling workflow must grant the permission because a composite action cannot grant caller permissions. Organization policy must also allow Copilot CLI requests billed through the built-in token.

```yaml
permissions:
  contents: read
  copilot-requests: write

steps:
  - uses: Attamusc/weekly-report-cli@<sha>
    with:
      github-token: ${{ secrets.REPORT_DATA_TOKEN }}
      project: 'org:my-org/5'
      since-days: 7
```

If organization policy does not allow the built-in token, pass a separate fine-grained PAT with account-level **Copilot Requests** permission:

```yaml
- uses: Attamusc/weekly-report-cli@<sha>
  with:
    github-token: ${{ secrets.REPORT_DATA_TOKEN }}
    copilot-token: ${{ secrets.COPILOT_REQUESTS_TOKEN }}
```

Keep `github-token` as the data-access credential. Classic PATs are unsupported for Copilot requests.

### Full Example: Weekly Report to Slack

```yaml
name: Weekly Status Report

on:
  schedule:
    - cron: '0 9 * * 1'  # Every Monday at 9 AM UTC
  workflow_dispatch:

permissions:
  contents: read
  copilot-requests: write

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
| `github-token` | Token used only for GitHub issue and Projects data access | **Required** |
| `copilot-token` | Optional fine-grained PAT with Copilot Requests permission; otherwise uses the calling workflow token | - |
| `copilot-cli-version` | Exact `@github/copilot` version installed by the action | `1.0.79` |
| `copilot-model` | Copilot model used for summarization; availability depends on account and organization policy | `claude-haiku-4.5` |
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
- `GITHUB_TOKEN` - Token used for GitHub issue and Projects data access

#### Optional
- `COPILOT_GITHUB_TOKEN` - Fine-grained PAT with account-level Copilot Requests permission. Classic PATs are unsupported.
- `COPILOT_MODEL` - Copilot model to use (default: `claude-haiku-4.5`). Model availability depends on account and organization policy.
- `COPILOT_CLI_PATH` - Path to the Copilot CLI executable when `copilot` is not on `PATH`.
- `DISABLE_SUMMARY` - Set to any value to disable Copilot startup and AI summarization.
- `AI_TIMEOUT` - Per-request timeout in seconds (default: `120`).

Keep data and inference credentials separate for direct use:

```bash
export GITHUB_TOKEN='<data token>'
export COPILOT_GITHUB_TOKEN='<fine-grained Copilot Requests PAT>'
weekly-report-cli generate ...
```

If `COPILOT_GITHUB_TOKEN` is omitted, the Copilot SDK can use stored Copilot CLI or `gh` authentication.

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
│   │   └── copilot.go     # GitHub Copilot SDK implementation
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
- **Copilot Errors**: Visible stderr diagnostics followed by non-AI fallback content
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

**Copilot Summarization Failures**
```text
Copilot summarization unavailable (missing executable); using non-AI fallback content. Install GitHub Copilot CLI and ensure copilot is on PATH.
```
- Copilot startup, authorization, model-availability, and inference failures produce one actionable diagnostic on stderr, then report generation continues with non-AI fallback content.
- These fallback diagnostics remain visible with `--quiet`; report stdout stays clean.
- For Actions authorization failures, verify `copilot-requests: write` on the calling workflow and organization policy, or provide `copilot-token` as a fine-grained Copilot Requests PAT.
- For model failures, choose an available model with `COPILOT_MODEL` or the action's `copilot-model` input. Availability is policy-dependent; the tool does not silently switch models.
- Set `DISABLE_SUMMARY=true` for `generate`, use `describe --no-summary`, or set action input `no-summary: true` to skip Copilot entirely.

### Debug Mode
For debugging, you can examine the raw data extraction by looking at the test files or adding debug output to the extraction functions.