package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/Attamusc/weekly-report-cli/internal/input"
)

const (
	defaultBaseURL    = "https://api.github.com/graphql"
	userAgent         = "weekly-report-cli/1.0"
	maxRetries        = 3
	baseBackoffMs     = 1000 // 1 second
	requestTimeoutSec = 30   // 30 seconds
)

// Client is a GraphQL client for GitHub Projects API
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
}

// NewClient creates a new GitHub Projects GraphQL client
func NewClient(token string) *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: requestTimeoutSec * time.Second,
		},
		baseURL: defaultBaseURL,
		token:   token,
	}
}

// FetchProjectItems fetches all items from a project with field values
// Handles pagination automatically and returns all items up to maxItems limit
func (c *Client) FetchProjectItems(ctx context.Context, config ProjectConfig) ([]ProjectItem, error) {
	// Get logger from context if available
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	logger.Debug("Fetching project items", "project", config.Ref.String(), "maxItems", config.MaxItems)

	var allItems []ProjectItem
	var cursor *string
	hasMore := true
	totalFetched := 0

	// Build the query once
	query := buildProjectQuery(config.Ref.Type)

	for hasMore && totalFetched < config.MaxItems {
		// Calculate batch size (don't exceed maxItems)
		batchSize := min(config.MaxItems-totalFetched, 100)

		logger.Debug("Fetching project page", "cursor", cursor, "batchSize", batchSize)

		// Fetch page
		response, err := c.fetchProjectPage(ctx, query, config.Ref, batchSize, cursor)
		if err != nil {
			return nil, err
		}

		// Extract project data
		project := response.Data.GetProject()
		if project == nil {
			return nil, fmt.Errorf("project not found: %s", config.Ref.String())
		}

		// Convert items to ProjectItem structs
		pageItems, err := c.convertProjectItems(project.Items.Nodes, config.Ref)
		if err != nil {
			return nil, fmt.Errorf("failed to convert project items: %w", err)
		}

		allItems = append(allItems, pageItems...)
		totalFetched += len(pageItems)

		// Update pagination
		hasMore = project.Items.PageInfo.HasNextPage
		cursor = project.Items.PageInfo.EndCursor

		logger.Debug("Project page fetched", "items", len(pageItems), "totalFetched", totalFetched, "hasMore", hasMore)
	}

	logger.Info("Project items fetched", "project", config.Ref.String(), "total", len(allItems))

	return allItems, nil
}

// fetchProjectPage fetches a single page of project items
func (c *Client) fetchProjectPage(ctx context.Context, query string, ref ProjectRef, batchSize int, cursor *string) (*graphQLResponse, error) {
	// Build variables
	variables := map[string]interface{}{
		"owner":  ref.Owner,
		"number": ref.Number,
		"first":  batchSize,
	}
	if cursor != nil {
		variables["cursor"] = *cursor
	}

	// Build request
	request := graphQLRequest{
		Query:     query,
		Variables: variables,
	}

	// Execute with retries
	return c.executeGraphQLWithRetry(ctx, request, ref)
}

// executeGraphQLWithRetry executes a GraphQL request with retry logic
func (c *Client) executeGraphQLWithRetry(ctx context.Context, request graphQLRequest, ref ProjectRef) (*graphQLResponse, error) {
	// Get logger from context
	logger, ok := ctx.Value("logger").(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Calculate exponential backoff with jitter
			backoff := calculateBackoff(attempt - 1)
			logger.Debug("Retrying GraphQL request", "attempt", attempt, "backoff", backoff)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		response, err := c.executeGraphQL(ctx, request)
		if err != nil {
			lastErr = err

			// Check if it's a rate limit error
			if isRateLimitError(err) {
				logger.Debug("GraphQL rate limit hit", "attempt", attempt)
				if attempt < maxRetries {
					continue
				}
			}

			// Check if it's a retryable error
			if isRetryableError(err) {
				logger.Debug("Retryable GraphQL error", "attempt", attempt, "error", err)
				if attempt < maxRetries {
					continue
				}
			}

			// Non-retryable error, return immediately
			return nil, enhanceGraphQLError(err, ref)
		}

		// Check for GraphQL errors in response
		if len(response.Errors) > 0 {
			err := formatGraphQLErrors(response.Errors, ref)
			logger.Debug("GraphQL errors in response", "errors", len(response.Errors))
			return nil, err
		}

		// Success
		return response, nil
	}

	// All retries exhausted
	return nil, fmt.Errorf("GraphQL request failed after %d retries: %w", maxRetries+1, lastErr)
}

// executeGraphQL executes a single GraphQL request
func (c *Client) executeGraphQL(ctx context.Context, request graphQLRequest) (*graphQLResponse, error) {
	// Marshal request body
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("User-Agent", userAgent)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check HTTP status
	if resp.StatusCode != http.StatusOK {
		return nil, &httpError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
			Headers:    resp.Header,
		}
	}

	// Parse GraphQL response
	var response graphQLResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GraphQL response: %w", err)
	}

	return &response, nil
}

// convertProjectItems converts GraphQL response items to ProjectItem structs
func (c *Client) convertProjectItems(nodes []projectItemNode, ref ProjectRef) ([]ProjectItem, error) {
	var items []ProjectItem

	for _, node := range nodes {
		item := ProjectItem{
			FieldValues: make(map[string]FieldValue),
		}

		// Determine content type
		switch node.Type {
		case "ISSUE":
			item.ContentType = ContentTypeIssue
		case "PULL_REQUEST":
			item.ContentType = ContentTypePullRequest
		case "DRAFT_ISSUE":
			item.ContentType = ContentTypeDraftIssue
		default:
			// Unknown type, skip
			continue
		}

		// Extract issue reference for issues and PRs
		if node.Content != nil && node.Content.Number != nil && node.Content.Repository != nil {
			item.IssueRef = &input.IssueRef{
				Owner:  node.Content.Repository.Owner.Login,
				Repo:   node.Content.Repository.Name,
				Number: *node.Content.Number,
				URL:    node.Content.URL,
			}
		}

		// Extract field values
		for _, fv := range node.FieldValues.Nodes {
			if fv.Field == nil {
				continue
			}

			fieldName := fv.Field.Name
			var fieldValue FieldValue

			// Determine field type and value
			if fv.Text != nil {
				fieldValue = FieldValue{
					Type: FieldTypeText,
					Text: *fv.Text,
				}
			} else if fv.Name != nil {
				fieldValue = FieldValue{
					Type: FieldTypeSingleSelect,
					Text: *fv.Name,
				}
			} else if fv.Date != nil {
				// Parse date string
				parsedDate, err := time.Parse("2006-01-02", *fv.Date)
				if err == nil {
					fieldValue = FieldValue{
						Type: FieldTypeDate,
						Date: &parsedDate,
					}
				} else {
					// If date parsing fails, store as text
					fieldValue = FieldValue{
						Type: FieldTypeText,
						Text: *fv.Date,
					}
				}
			} else if fv.Number != nil {
				fieldValue = FieldValue{
					Type:   FieldTypeNumber,
					Number: *fv.Number,
				}
			} else {
				// Unknown field type, skip
				continue
			}

			item.FieldValues[fieldName] = fieldValue
		}

		items = append(items, item)
	}

	return items, nil
}

// httpError represents an HTTP error response
type httpError struct {
	StatusCode int
	Body       string
	Headers    http.Header
}

func (e *httpError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

// isRateLimitError checks if an error is a rate limit error
func isRateLimitError(err error) bool {
	if httpErr, ok := err.(*httpError); ok {
		return httpErr.StatusCode == 429 || httpErr.StatusCode == 403
	}
	return false
}

// isRetryableError checks if an error is retryable
func isRetryableError(err error) bool {
	if httpErr, ok := err.(*httpError); ok {
		// Retry on 5xx errors
		return httpErr.StatusCode >= 500
	}
	return false
}

// calculateBackoff calculates exponential backoff with jitter
func calculateBackoff(attempt int) time.Duration {
	// Exponential backoff: base * 2^attempt
	backoffMs := baseBackoffMs * int(math.Pow(2, float64(attempt)))

	// Add jitter (±25%)
	jitterMs := backoffMs / 4
	jitter := rand.Intn(jitterMs*2) - jitterMs

	return time.Duration(backoffMs+jitter) * time.Millisecond
}

// enhanceGraphQLError enhances a GraphQL error with helpful context
func enhanceGraphQLError(err error, ref ProjectRef) error {
	if httpErr, ok := err.(*httpError); ok {
		switch httpErr.StatusCode {
		case 401:
			return fmt.Errorf("GitHub API authentication failed for project '%s'.\nYour GITHUB_TOKEN may be invalid.\nVisit https://github.com/settings/tokens to create or update your token", ref.String())

		case 403:
			// Check if it's rate limit or permission issue
			if strings.Contains(httpErr.Body, "rate limit") {
				return fmt.Errorf("GitHub GraphQL API rate limit exceeded.\nTip: Use --project-max-items to reduce query cost")
			}
			return fmt.Errorf("GitHub API access denied for project '%s'.\nYour token may require the 'read:project' scope.\nVisit https://github.com/settings/tokens to update your token", ref.String())

		case 404:
			return fmt.Errorf("Project not found: %s\nThis could mean:\n  - The project doesn't exist\n  - The project is private and your token lacks access\n  - The organization/user name is incorrect", ref.String())

		case 429:
			return fmt.Errorf("GitHub GraphQL API rate limit exceeded.\nRetry after a few minutes.\nTip: Use --project-max-items to reduce query cost")
		}
	}

	return err
}

// formatGraphQLErrors formats GraphQL errors into a user-friendly error message
func formatGraphQLErrors(errors []graphQLError, ref ProjectRef) error {
	if len(errors) == 0 {
		return nil
	}

	var messages []string
	for _, err := range errors {
		messages = append(messages, err.Message)
	}

	return fmt.Errorf("GraphQL errors for project '%s':\n  - %s", ref.String(), strings.Join(messages, "\n  - "))
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
