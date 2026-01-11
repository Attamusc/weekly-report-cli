# AGENTS.md

Guidance for AI coding agents working in this repository.

## Project Overview

Go CLI tool (`weekly-report-cli`) that generates weekly status reports from GitHub issue comments. Uses a 3-phase pipeline: parallel data collection, batch AI summarization, and result assembly. Built with Cobra (CLI), go-github (API), and OAuth2.

## Build & Test Commands

```bash
# Full pipeline (clean, deps, fmt, vet, lint, test, build)
make all

# Build
make build                    # Dev build (CGO_ENABLED=0)
make build-prod               # Production (linux/amd64, optimized)

# Test
make test                     # go test -v ./...
make test-race                # go test -race -v ./...
go test -v -run TestFuncName ./internal/pkg/   # Single test
go test -v -run TestFuncName/subtest ./internal/pkg/  # Single subtest
make coverage                 # Generate HTML coverage report

# Lint & Format
make fmt                      # gofmt -w .
make vet                      # go vet ./...
make lint                     # golangci-lint run ./...
make check                    # fmt + vet + lint + test combined

# Dependencies
make deps                     # download + tidy + verify
```

## Code Style

### Imports

Three groups separated by blank lines: (1) stdlib, (2) external, (3) internal. Use `goimports` ordering. Aliases only when package names collide (e.g., `githubapi "github.com/google/go-github/v66/github"`).

```go
import (
    "context"
    "fmt"
    "log/slog"

    "github.com/google/go-github/v66/github"
    "github.com/spf13/cobra"

    "github.com/Attamusc/weekly-report-cli/internal/ai"
    "github.com/Attamusc/weekly-report-cli/internal/config"
)
```

### Formatting

- `gofmt` is the authoritative formatter -- enforced by CI and `golangci-lint`
- `goimports` for import ordering (enabled in `.golangci.yml`)
- No manual alignment beyond what `gofmt` produces

### Naming Conventions

- **Packages:** lowercase, single-word preferred (`derive`, `format`, `report`, `input`)
- **Exported types:** PascalCase with full words (`IssueRef`, `ProjectItem`, `BatchItem`)
- **Unexported helpers:** camelCase (`detectInputMode`, `deduplicateRefs`, `calculateBackoff`)
- **Acronyms:** uppercase in type names (`URL`, `HTML`, `ID`, `API`), but `htmlURL` in field access
- **Constants:** PascalCase for exported (`StateClosed`, `MarkerIsReport`), camelCase for unexported (`maxRetries`, `baseBackoffMs`)
- **Enums:** iota-based `int` types with `TypePrefix` naming (`ProjectTypeOrg`, `ContentTypeIssue`, `InputModeURLList`, `FieldTypeSingleSelect`)
- **Interfaces:** verb/noun names, not `-er` suffix necessarily (`Summarizer`, `ProjectClient`)
- **Context keys:** empty struct types (`LoggerContextKey struct{}`)

### Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain
- Use `errors.New()` for simple sentinel errors (see `config.go`)
- Return `(value, error)` pairs; never panic in library code
- Check type assertions explicitly: `val, ok := x.(Type); if !ok { ... }`
- Provide user-friendly error messages for known API failure modes (see `enhanceGitHubError`)
- Discard intentionally unused errors with `_ =` (e.g., `_ = resp.Body.Close()`, `_ = godotenv.Load()`)
- `errcheck` linter is enabled with `check-type-assertions: true`

### Struct Definitions

- Group related fields logically; add inline comments for field semantics
- Use pointer types for optional/nullable fields (`*time.Time`, `*format.Note`)
- Nested anonymous structs for config groupings (see `Config.Models`, `Config.Project`)
- No JSON/YAML struct tags used in this codebase (data flows through API clients, not serialization)

### Function Signatures

- Accept `context.Context` as first parameter for functions that do I/O
- Return `(T, error)` for fallible operations
- Return `(T, bool)` for parse/lookup operations (`ParseReport` returns `(Report, bool)`)
- Use concrete config structs rather than option funcs for complex configuration
- Constructors follow `NewXxx` pattern (`NewClient`, `NewNoopSummarizer`, `NewGHModelsClient`)

### Comments

- All exported types and functions must have doc comments starting with the name
- Comment format: `// FunctionName does X` (single-line) or block for complex behavior
- Use `// Case N:` inline comments to document branching logic (see `collectIssueData`)
- Phase markers use `// ========== PHASE X: Description ==========` in orchestration code

### Logging

- Use `log/slog` structured logging throughout (not `log` or `fmt.Println`)
- Retrieve logger from context: `logger, ok := ctx.Value(LoggerContextKey{}).(*slog.Logger)`
- Fall back to `slog.Default()` if not in context
- Write progress/diagnostic output to `os.Stderr`; reserve `os.Stdout` for report output
- Use `slog.LevelDebug` for internal details, `slog.LevelInfo` for progress, `slog.LevelWarn` for recoverable issues

### Concurrency

- Bounded worker pools with channel-based semaphores (`make(chan struct{}, concurrency)`)
- `sync.WaitGroup` for goroutine coordination
- `sync/atomic` for progress counters
- Close result channels from a dedicated goroutine after `wg.Wait()`

## Testing Patterns

- **Standard library only** -- no testify or assertion libraries
- **Table-driven tests** with `t.Run(tc.name, ...)` for parameterized cases
- **Assertions:** `if got != want { t.Errorf(...) }` and `t.Fatalf` for fatal preconditions
- **HTTP mocking:** `net/http/httptest.NewServer` with custom handlers for GitHub API tests
- **Test naming:** `TestFunctionName_Scenario` (e.g., `TestParseReport_ValidReport`, `TestFetchIssue_NotFound`)
- **No test fixtures on disk** -- test data is inline in test functions
- Linters relaxed in test files: `dupl`, `gosec`, `goconst`, `errcheck`, `gocyclo` are excluded

## Linter Configuration

Enforced via `.golangci.yml` with these notable settings:

- **errcheck:** type assertion checks enabled
- **govet:** shadow detection enabled
- **gocyclo:** max complexity 23
- **dupl:** threshold 100 lines
- **goconst:** min 3 occurrences, min length 3
- **revive:** confidence 0.8

## Project Structure

```
cmd/                    CLI commands (Cobra)
  root.go               Root command, Execute()
  generate.go           Main pipeline orchestration
  describe.go           Describe command
internal/
  ai/                   AI summarization (Summarizer interface + GitHub Models impl)
  config/               Configuration from env vars + CLI flags
  derive/               Status mapping and date parsing utilities
  format/               Markdown table and notes rendering
  github/               GitHub REST API client with retry logic
  input/                URL parsing, validation, input resolution
  projects/             GitHub Projects V2 GraphQL client, views, filtering
  report/               Report extraction from HTML comment markers
main.go                 Entry point
```

## Key Architectural Rules

1. **No circular imports** -- the `cmd` package uses adapter patterns to bridge `input` and `projects` packages
2. **Phases are distinct** -- data collection (parallel), AI summarization (batched), result assembly (sequential)
3. **AI is optional** -- `NoopSummarizer` provides transparent fallback when AI is disabled
4. **stdout is sacred** -- only final report output goes to stdout; everything else goes to stderr
5. **Exit codes matter** -- 0 success, 2 no rows produced, >2 fatal errors

## Version Control

This repo uses Jujutsu (`jj`) colocated with Git. Both `jj` and `git` commands work.
