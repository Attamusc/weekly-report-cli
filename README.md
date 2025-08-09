# Weekly Report CLI

A Go CLI tool that generates weekly status reports by parsing structured data from GitHub issue comments. The tool fetches GitHub issues, extracts status report data using HTML comment markers, and generates markdown tables with optional AI summarization via GitHub Models.

## Features

- **GitHub Integration**: Fetches issues and comments using OAuth2 authentication
- **Structured Data Parsing**: Extracts reports from HTML comment markers in GitHub issues
- **AI Summarization**: Optional AI-powered summaries using GitHub Models API
- **Flexible Input**: Accepts GitHub issue URLs from stdin or file input
- **Concurrent Processing**: Parallel API requests with configurable worker pools
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

## Configuration

### Environment Variables

#### Required
- `GITHUB_TOKEN` - Personal Access Token for GitHub API access and GitHub Models

#### Optional
- `GITHUB_MODELS_BASE_URL` - Base URL for GitHub Models API (default: `https://models.github.ai/v1`)
- `GITHUB_MODELS_MODEL` - AI model to use (default: `gpt-4o-mini`)
- `DISABLE_SUMMARY` - Set to any value to disable AI summarization

### Setting up GitHub Token
1. Go to GitHub Settings > Developer settings > Personal access tokens
2. Generate a new token with the following scopes:
   - `repo` (for private repositories)
   - `public_repo` (for public repositories)
3. Set the token as an environment variable:
   ```bash
   export GITHUB_TOKEN=your_token_here
   ```

## Usage

### Command Line Interface

```bash
# Basic usage with stdin
cat links.txt | weekly-report-cli generate --since-days 7

# With file input
weekly-report-cli generate --input links.txt --since-days 14

# Disable AI summarization and notes
weekly-report-cli generate --input links.txt --no-notes

# Custom concurrency
weekly-report-cli generate --input links.txt --concurrency 8
```

### Input Format

The tool accepts GitHub issue URLs, one per line:
```
https://github.com/owner/repo/issues/123
https://github.com/owner/repo/issues/456
https://github.com/another-owner/another-repo/issues/789
```

### Report Data Format

Reports are embedded in GitHub issue comments using HTML comment markers:

```html
<!-- data key="isReport" value="true" -->
<!-- data key="trending" start -->🟢 on track<!-- data end -->
<!-- data key="target_date" start -->2025-08-06<!-- data end -->
<!-- data key="update" start -->
Completed the user authentication module.
- Added OAuth2 integration
- Implemented session management
- Added security headers
<!-- data end -->
```

#### Status Values
The following status indicators are automatically mapped to standardized emojis:

- `🟢`, `green`, `on track` → `:green_circle: On Track`
- `🟡`, `yellow`, `at risk` → `:yellow_circle: At Risk`
- `🔴`, `red`, `blocked`, `off track` → `:red_circle: Off Track`
- `⚪`, `white`, `not started` → `:white_circle: Not Started`
- `🟣`, `purple`, `done`, `complete` → `:purple_circle: Done`

### Example Output

```markdown
| Status | Epic | Target Date | Summary |
|--------|------|-------------|---------|
| :green_circle: On Track | Authentication System (#123) | 2025-08-06 | Completed OAuth2 integration and session management |
| :yellow_circle: At Risk | User Dashboard (#456) | 2025-08-10 | Frontend components completed, backend API in progress |
| :purple_circle: Done | Payment Gateway (#789) | 2025-08-01 | Successfully integrated Stripe payment processing |

## Notes
- 3 issues processed
- 2 issues had recent updates
- AI summaries generated using gpt-4o-mini
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
│   │   └── links.go       # URL parsing and validation
│   └── report/            # Report extraction and processing
│       ├── extract.go     # HTML comment parsing
│       └── select.go      # Time window filtering
├── go.mod                 # Go module definition
├── go.sum                 # Dependency checksums
└── main.go               # Application entry point
```

### Key Components

- **GitHub Client**: OAuth2-authenticated client with retry logic for rate limits
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