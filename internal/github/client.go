package github

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/go-github/v66/github"
	"golang.org/x/oauth2"

	"github.com/Attamusc/weekly-report-cli/internal/retry"
)

const (
	userAgent         = "weekly-report-cli/1.0"
	maxRetries        = 3
	baseBackoffMs     = 1000 // 1 second base backoff
	requestTimeoutSec = 30   // 30 second timeout per request
)

// New creates a new GitHub client with OAuth2 authentication and retry logic
func New(ctx context.Context, token string) *github.Client {
	// Create OAuth2 token source
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})

	// Create HTTP client with OAuth2 transport, retry logic, and timeout
	httpClient := &http.Client{
		Timeout: requestTimeoutSec * time.Second,
		Transport: &retryTransport{
			base: &oauth2.Transport{
				Source: ts,
				Base:   http.DefaultTransport,
			},
		},
	}

	// Create GitHub client with custom HTTP client
	client := github.NewClient(httpClient)
	client.UserAgent = userAgent

	return client
}

// retryTransport wraps http.RoundTripper with retry logic for GitHub API
type retryTransport struct {
	base http.RoundTripper
}

// RoundTrip implements http.RoundTripper with intelligent retry logic
func (rt *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Clone request for retry attempts
		reqClone := req.Clone(req.Context())

		// Make the request
		resp, err := rt.base.RoundTrip(reqClone)
		if err != nil {
			lastErr = err
			if attempt < maxRetries {
				backoffDuration := retry.CalculateBackoff(attempt, baseBackoffMs)
				time.Sleep(backoffDuration)
			}
			continue
		}

		// Check if this is a non-retryable authorization error
		if isAuthorizationError(resp) {
			// Don't retry authorization errors, return immediately with descriptive error
			return resp, nil
		}

		// Check if we should retry based on response status
		if shouldRetry(resp) {
			// Handle rate limit with specific retry-after logic
			if resp.StatusCode == http.StatusForbidden {
				if retryAfter := getRateLimitRetryAfter(resp); retryAfter > 0 {
					// Close response body to prevent resource leak
					_ = resp.Body.Close()

					if attempt < maxRetries {
						time.Sleep(retryAfter)
						continue
					}
				}
			}

			// Handle other 5xx errors with exponential backoff
			if resp.StatusCode >= 500 {
				_ = resp.Body.Close()
				if attempt < maxRetries {
					backoffDuration := retry.CalculateBackoff(attempt, baseBackoffMs)
					time.Sleep(backoffDuration)
					continue
				}
			}
		}

		// Success or non-retryable error
		return resp, nil
	}

	// All retries exhausted
	return nil, fmt.Errorf("GitHub API request failed after %d attempts: %w", maxRetries+1, lastErr)
}

// shouldRetry determines if a response should be retried
func shouldRetry(resp *http.Response) bool {
	// Retry on 5xx server errors
	if resp.StatusCode >= 500 {
		return true
	}

	// Retry on 403 rate limit errors only when actually rate-limited.
	// GitHub sends X-RateLimit-* headers on ALL responses (including SSO/auth 403s),
	// so we check for Retry-After or X-RateLimit-Remaining: 0 specifically.
	if resp.StatusCode == http.StatusForbidden {
		if resp.Header.Get("Retry-After") != "" {
			return true
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return true
		}
	}

	return false
}

// getRateLimitRetryAfter calculates retry delay for rate limit responses
func getRateLimitRetryAfter(resp *http.Response) time.Duration {
	// First check for Retry-After header
	if retryAfterStr := resp.Header.Get("Retry-After"); retryAfterStr != "" {
		if retryAfterSec, err := strconv.Atoi(retryAfterStr); err == nil {
			return time.Duration(retryAfterSec) * time.Second
		}
	}

	// Check for X-RateLimit-Reset header
	if resetTimeStr := resp.Header.Get("X-RateLimit-Reset"); resetTimeStr != "" {
		if resetTime, err := strconv.ParseInt(resetTimeStr, 10, 64); err == nil {
			resetDuration := time.Until(time.Unix(resetTime, 0))
			if resetDuration > 0 {
				// Add small buffer to avoid racing with reset
				return resetDuration + (5 * time.Second)
			}
		}
	}

	// Default fallback for rate limits
	return 60 * time.Second
}

// isAuthorizationError checks if the response indicates an authorization error that should not be retried
func isAuthorizationError(resp *http.Response) bool {
	// 401 Unauthorized - invalid token
	if resp.StatusCode == http.StatusUnauthorized {
		return true
	}

	// 403 Forbidden — only treat as retryable rate limit when actually rate-limited.
	// GitHub sends X-RateLimit-* headers on ALL responses including SSO/auth 403s.
	if resp.StatusCode == http.StatusForbidden {
		// Only a rate limit error when Retry-After is set or remaining is 0
		if resp.Header.Get("Retry-After") != "" {
			return false // Rate limit, not an authorization error
		}
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return false // Rate limit, not an authorization error
		}

		// Any other 403 (SSO, permissions, etc.) is an authorization error
		return true
	}

	// 404 Not Found on private repos can indicate insufficient permissions
	if resp.StatusCode == http.StatusNotFound {
		// This could be a real 404 or a permission issue, but we should not retry
		return true
	}

	return false
}
