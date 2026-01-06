package input

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// InputMode represents the detected input mode
type InputMode int

const (
	// InputModeUnknown indicates no valid input detected
	InputModeUnknown InputMode = iota
	// InputModeURLList indicates URL list input (stdin or file)
	InputModeURLList
	// InputModeProject indicates project board input
	InputModeProject
	// InputModeMixed indicates both project and URL list input
	InputModeMixed
)

// String returns the string representation of InputMode
func (m InputMode) String() string {
	switch m {
	case InputModeURLList:
		return "URL List"
	case InputModeProject:
		return "Project"
	case InputModeMixed:
		return "Mixed (Project + URL List)"
	case InputModeUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}

// ResolverConfig holds configuration for input resolution
type ResolverConfig struct {
	// Project board settings
	ProjectURL         string
	ProjectFieldName   string
	ProjectFieldValues []string
	ProjectIncludePRs  bool
	ProjectMaxItems    int
	ProjectView        string // View name to filter by
	ProjectViewID      string // View ID (takes precedence over ProjectView)

	// URL list settings
	URLListPath string // File path or empty for stdin
	UseStdin    bool   // Whether to read from stdin
}

// ProjectClient is an interface for fetching project items
// This allows us to avoid circular dependencies and makes testing easier
type ProjectClient interface {
	FetchProjectItems(ctx context.Context, config interface{}) ([]IssueRef, error)
}

// ResolveIssueRefs determines input mode and returns deduplicated issue refs
// This is the main entry point for getting issues from any source
func ResolveIssueRefs(ctx context.Context, cfg ResolverConfig, projectClient ProjectClient) ([]IssueRef, error) {
	// Get logger from context
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	// Validate configuration
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	// Detect input mode
	mode := detectInputMode(cfg)
	logger.Info("Input mode detected", "mode", mode.String())

	if mode == InputModeUnknown {
		return nil, fmt.Errorf("no valid input provided: specify --project or provide issue URLs via stdin/--input")
	}

	var allRefs []IssueRef

	// Fetch from project if specified
	if mode == InputModeProject || mode == InputModeMixed {
		logger.Debug("Fetching issues from project board")
		projectRefs, err := fetchFromProject(ctx, cfg, projectClient)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch from project: %w", err)
		}
		logger.Info("Issues fetched from project", "count", len(projectRefs))
		allRefs = append(allRefs, projectRefs...)
	}

	// Fetch from URL list if specified
	if mode == InputModeURLList || mode == InputModeMixed {
		logger.Debug("Fetching issues from URL list")
		urlRefs, err := fetchFromURLList(cfg)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch from URL list: %w", err)
		}
		logger.Info("Issues fetched from URL list", "count", len(urlRefs))
		allRefs = append(allRefs, urlRefs...)
	}

	// Deduplicate
	logger.Debug("Deduplicating issue references", "total", len(allRefs))
	unique := deduplicateRefs(allRefs)
	logger.Info("Input resolution complete", "uniqueIssues", len(unique), "mode", mode.String())

	return unique, nil
}

// detectInputMode determines which input mode to use based on configuration
func detectInputMode(cfg ResolverConfig) InputMode {
	hasProject := cfg.ProjectURL != ""
	hasURLList := cfg.UseStdin || cfg.URLListPath != ""

	if hasProject && hasURLList {
		return InputModeMixed
	} else if hasProject {
		return InputModeProject
	} else if hasURLList {
		return InputModeURLList
	}

	return InputModeUnknown
}

// validateConfig validates the resolver configuration
func validateConfig(cfg ResolverConfig) error {
	// If project URL is provided, validate project-specific settings
	if cfg.ProjectURL != "" {
		// Field name and values are now optional (have defaults)
		// But if provided, they should be valid
		if cfg.ProjectMaxItems < 1 || cfg.ProjectMaxItems > 1000 {
			return fmt.Errorf("--project-max-items must be between 1 and 1000, got %d", cfg.ProjectMaxItems)
		}
	}

	return nil
}

// fetchFromProject fetches issue references from a project board
func fetchFromProject(ctx context.Context, cfg ResolverConfig, projectClient ProjectClient) ([]IssueRef, error) {
	// Get logger from context
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Fetching from project board", "url", cfg.ProjectURL)

	// Delegate to the project client
	// The client will handle parsing, fetching, and filtering
	refs, err := projectClient.FetchProjectItems(ctx, cfg)
	if err != nil {
		return nil, err
	}

	return refs, nil
}

// fetchFromURLList fetches issue references from URL list (stdin or file)
func fetchFromURLList(cfg ResolverConfig) ([]IssueRef, error) {
	var reader io.Reader

	if cfg.UseStdin {
		// Read from stdin
		reader = os.Stdin
	} else if cfg.URLListPath != "" {
		// Read from file
		file, err := os.Open(cfg.URLListPath)
		if err != nil {
			return nil, fmt.Errorf("failed to open input file %s: %w", cfg.URLListPath, err)
		}
		defer file.Close()
		reader = file
	} else {
		return nil, fmt.Errorf("no URL list source specified")
	}

	// Use existing ParseIssueLinks function
	refs, err := ParseIssueLinks(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to parse issue links: %w", err)
	}

	return refs, nil
}

// deduplicateRefs removes duplicate issue references while preserving order
func deduplicateRefs(refs []IssueRef) []IssueRef {
	seen := make(map[string]bool)
	var unique []IssueRef

	for _, ref := range refs {
		// Use canonical URL as the key for deduplication
		if !seen[ref.URL] {
			seen[ref.URL] = true
			unique = append(unique, ref)
		}
	}

	return unique
}

// ParseFieldValues splits a comma-separated string into field values
// Trims whitespace and filters empty values
func ParseFieldValues(raw string) []string {
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, ",")
	var values []string

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			values = append(values, trimmed)
		}
	}

	return values
}
