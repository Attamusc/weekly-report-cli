# Milestone 1: Code Quality Fixes

**Date:** 2026-04-21
**Status:** Draft
**Directory:** /Users/attamusc/projects/github.com/Attamusc/weekly-report-cli

## Overview

Refactor the weekly-report-cli codebase to improve testability, type safety, and maintainability. The primary goal is moving untested business logic out of the `cmd` package into testable `internal/` packages, fixing concurrency bugs, and cleaning up API surfaces.

## Approach

**Bottom-up refactoring** — start with leaf fixes (dead code, type safety) that don't change interfaces, then work up to structural changes (extract pipeline, shared boilerplate). Each todo is independently shippable and passes `make check`.

### Execution Order

1. **Remove dead code** (P3 #11) — clean slate, no risk
2. **Fix logger context key** (P2 #5) — small, isolated
3. **Replace `interface{}` in ProjectClient** (P2 #7) — type safety, small blast radius
4. **Replace 16 positional params in config** (P1 #2) — prerequisite for shared boilerplate
5. **Fix shared state mutation in executeBatchCall** (P1 #4) — concurrency bug fix
6. **Replace `os.Exit(2)` with sentinel errors** (P2 #6) — prerequisite for pipeline extraction
7. **Extract pipeline package** (P1 #1) — the big one, moves business logic to testable package
8. **Extract shared command boilerplate** (P1 #3) — depends on pipeline + config refactor
9. **Deduplicate flag variables** (P2 #10) — depends on shared boilerplate
10. **Consolidate retry/backoff** (P2 #9) — independent, lower priority
11. **Add config package tests** (P2 #8) — after config refactor

## Design

### Dead Code Removal (#11)
- Delete `projects/client.go` `min()` function (Go 1.21+ builtin)
- Delete `projects/filter.go` `DeduplicateIssueRefs` (duplicate of `input/resolver.go` `deduplicateRefs`)
- Delete unused `projectFields` types in `projects/query.go` — **actually these ARE used** (`query.go:157`), so skip this

### Logger Context Key (#5)
- `ai/ghmodels.go` defines `loggerContextKey struct{}` (unexported, line 177)
- Rest of codebase uses `input.LoggerContextKey{}` 
- Fix: replace `loggerContextKey` usage in `ai/` with `input.LoggerContextKey{}`
- This means `ai` imports `input` — check for circular deps. Currently `ai` has no internal imports, `input` has no `ai` imports. Safe.

### ProjectClient interface{} (#7)
- `input.ProjectClient.FetchProjectItems` accepts `interface{}`
- Only caller is `fetchFromProject` in `resolver.go` which passes `ResolverConfig`
- Only implementation is `projectClientAdapter` in `cmd/common.go` which type-asserts to `ResolverConfig`
- Fix: change signature to `FetchProjectItems(ctx context.Context, config ResolverConfig) ([]IssueRef, error)`
- Remove type assertion in adapter

### Config struct input (#2)
- Replace 16 positional params with `ConfigInput` struct
- Keep `FromEnvAndFlags` but accept struct instead
- Update both call sites (`generate.go`, `describe.go`)

### executeBatchCall shared state (#4)
- `executeBatchCall` swaps `c.SystemPrompt` and restores via defer — not goroutine-safe
- Fix: pass system prompt to `callAPI` as parameter, don't mutate struct
- Change `callAPI` to accept optional system prompt override
- `getSystemPrompt()` stays for default behavior

### os.Exit sentinel errors (#6)
- Define `var ErrNoRows = errors.New("no rows produced")` in a shared location
- Return `ErrNoRows` instead of calling `os.Exit(2)`
- Handle in `RunE` wrapper or root command's `Execute()`
- Root command checks for `ErrNoRows` and calls `os.Exit(2)` there

### Pipeline extraction (#1)
- Create `internal/pipeline/` package
- Move: `IssueData`, `IssueDataResult`, `IssueResult`, `DescribeIssueData`, `DescribeIssueDataResult`
- Move: `collectIssueData`, `assembleGenerateResults`, `createResultFromData`, `applyNoCommentFallback`, `applyLabelFallback`
- Move: `collectDescribeIssueData`, `assembleDescribeResults`
- Move: `batchSummarize`, `batchDescribe`
- Pipeline functions take interfaces for GitHub client operations (fetch issue, fetch comments)
- `cmd/generate.go` and `cmd/describe.go` become thin orchestrators

### Shared boilerplate (#3)
- After pipeline extraction, `runGenerate` and `runDescribe` share: config loading, logger setup, project client init, resolver config building, issue resolution, GitHub client init, summarizer init
- Extract `setupCommand(flags) -> (ctx, cfg, githubClient, summarizer, issueRefs, error)` helper
- Keep in `cmd/` package as internal helper

### Deduplicate flags (#10)
- `generate.go` and `describe.go` have duplicate flag vars for project-related flags
- Use Cobra persistent flags on root or a shared `addProjectFlags(cmd)` helper
- Shared flag struct returned from helper

### Retry consolidation (#9)
- Three retry implementations: `github/client.go` (HTTP transport), `projects/client.go` (GraphQL), `ai/ghmodels.go` (AI API)
- Create `internal/retry/` with generic retry function
- Each caller wraps with domain-specific config

## Dependencies
- No new external dependencies
- All changes are internal refactoring

## Risks & Open Questions
1. **Pipeline extraction blast radius** — Moving types/functions changes import paths everywhere. Mitigated by doing it in one focused todo with clear file lists.
2. **Retry consolidation may over-abstract** — The three retry implementations have genuinely different needs (HTTP transport vs direct call, rate limit headers, etc.). May be better to just extract `calculateBackoff` into a shared util rather than full retry abstraction.
3. **Flag deduplication** — Cobra's persistent flags might not work cleanly if `generate` and `describe` need different defaults. May need shared `addFlags` helper instead.
