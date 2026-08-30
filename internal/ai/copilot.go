package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

type copilotRuntime interface {
	Start(context.Context) error
	CreateSession(context.Context, sessionOptions) (copilotSession, error)
	Stop() error
	ForceStop()
}

type copilotSession interface {
	SendAndWait(context.Context, string) (string, error)
	Abort(context.Context) error
	Disconnect() error
}

type sessionOptions struct {
	Model        string
	SystemPrompt string
}

// CopilotSummarizer implements summarization through a command-scoped Copilot runtime.
type CopilotSummarizer struct {
	runtime      copilotRuntime
	model        string
	systemPrompt string
	timeout      time.Duration
	cleanupOnce  sync.Once
	cleanupErr   error
}

const abortTimeout = 5 * time.Second

// NewCopilotSummarizer creates a Copilot SDK-backed summarizer.
func NewCopilotSummarizer(runtime copilotRuntime, model, systemPrompt string, timeout time.Duration) *CopilotSummarizer {
	return &CopilotSummarizer{
		runtime:      runtime,
		model:        model,
		systemPrompt: systemPrompt,
		timeout:      timeout,
	}
}

// NewSDKCopilotSummarizer creates a summarizer backed by the Copilot SDK runtime.
func NewSDKCopilotSummarizer(token, model, systemPrompt string, timeout time.Duration, env []string) *CopilotSummarizer {
	options := &copilot.ClientOptions{Env: env, GitHubToken: token}
	if token == "" {
		options.UseLoggedInUser = copilot.Bool(true)
	}
	return NewCopilotSummarizer(newSDKCopilotRuntime(copilot.NewClient(options)), model, systemPrompt, timeout)
}

// Start starts the command-scoped Copilot runtime.
func (c *CopilotSummarizer) Start(ctx context.Context) error {
	return c.runtime.Start(ctx)
}

// Cleanup stops the command-scoped Copilot runtime. Repeated calls are safe.
func (c *CopilotSummarizer) Cleanup() error {
	c.cleanupOnce.Do(func() {
		c.cleanupErr = c.runtime.Stop()
		if c.cleanupErr != nil {
			c.runtime.ForceStop()
		}
	})
	return c.cleanupErr
}

// Summarize generates a summary for a single update.
func (c *CopilotSummarizer) Summarize(ctx context.Context, title, url, update string) (string, error) {
	return c.complete(ctx, c.getSystemPrompt(), buildSummaryPrompt(title, url, update))
}

// SummarizeMany generates a summary for multiple updates ordered newest first.
func (c *CopilotSummarizer) SummarizeMany(ctx context.Context, title, url string, updates []string) (string, error) {
	return c.complete(ctx, c.getSystemPrompt(), buildManySummaryPrompt(title, url, updates))
}

// SummarizeBatch generates URL-keyed summaries for multiple issues.
//
//nolint:dupl // Summary and description batches intentionally retain distinct types and prompts.
func (c *CopilotSummarizer) SummarizeBatch(ctx context.Context, items []BatchItem) (map[string]BatchResult, error) {
	if len(items) == 0 {
		return make(map[string]BatchResult), nil
	}
	if len(items) > maxBatchSize {
		return chunkedBatch(ctx, items, getContextLogger(ctx), "summarize", c.SummarizeBatch)
	}

	userPrompt, err := c.buildBatchPrompt(items)
	if err != nil {
		return nil, fmt.Errorf("failed to build summarize prompt: %w", err)
	}
	text, err := c.complete(ctx, batchSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("summarize batch with Copilot: %w", err)
	}
	return c.parseBatchResponse(text, items)
}

// DescribeBatch generates URL-keyed project descriptions.
//
//nolint:dupl // Summary and description batches intentionally retain distinct types and prompts.
func (c *CopilotSummarizer) DescribeBatch(ctx context.Context, items []DescribeBatchItem) (map[string]string, error) {
	if len(items) == 0 {
		return make(map[string]string), nil
	}
	if len(items) > maxBatchSize {
		return chunkedBatch(ctx, items, getContextLogger(ctx), "describe", c.DescribeBatch)
	}

	userPrompt, err := c.buildDescribePrompt(items)
	if err != nil {
		return nil, fmt.Errorf("failed to build describe prompt: %w", err)
	}
	text, err := c.complete(ctx, describeSystemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("describe batch with Copilot: %w", err)
	}
	return c.parseDescribeResponse(text, items)
}

// GenerateHeader produces an executive summary paragraph from assembled report data.
func (c *CopilotSummarizer) GenerateHeader(ctx context.Context, items []HeaderItem) (string, error) {
	if len(items) == 0 {
		return "", nil
	}

	type headerRequestItem struct {
		Status     string  `json:"status"`
		Transition *string `json:"transition"`
		New        bool    `json:"new"`
		Title      string  `json:"title"`
		Summary    string  `json:"summary"`
	}
	request := make([]headerRequestItem, len(items))
	for i, item := range items {
		request[i] = headerRequestItem{
			Status: item.StatusCaption, Transition: item.StatusTransition, New: item.NewItem,
			Title: item.Title, Summary: item.Summary,
		}
	}
	userPrompt, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal header items: %w", err)
	}
	text, err := c.complete(ctx, headerSystemPrompt, string(userPrompt))
	if err != nil {
		return "", fmt.Errorf("generate header with Copilot: %w", err)
	}
	return text, nil
}

func (c *CopilotSummarizer) buildBatchPrompt(items []BatchItem) (string, error) {
	request := batchRequest{Items: make([]batchRequestItem, len(items))}
	for i, item := range items {
		request.Items[i] = batchRequestItem{ID: item.IssueURL, Issue: item.IssueTitle, Updates: item.UpdateTexts, ReportedStatus: item.ReportedStatus}
	}
	text, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal batch request: %w", err)
	}
	return string(text), nil
}

func (c *CopilotSummarizer) parseBatchResponse(response string, items []BatchItem) (map[string]BatchResult, error) {
	var nested map[string]sentimentResponseItem
	if err := json.Unmarshal([]byte(response), &nested); err == nil && len(nested) > 0 {
		for _, item := range nested {
			if item.Summary != "" {
				return convertNestedResponse(nested), nil
			}
			break
		}
	}

	var flat map[string]string
	if err := json.Unmarshal([]byte(response), &flat); err == nil && len(flat) > 0 {
		results := make(map[string]BatchResult, len(flat))
		for url, summary := range flat {
			results[url] = BatchResult{Summary: summary}
		}
		return results, nil
	}
	return parseMarkdownBatchResponse(response, items)
}

func parseMarkdownBatchResponse(response string, items []BatchItem) (map[string]BatchResult, error) {
	results := make(map[string]BatchResult)
	for _, part := range strings.Split(response, "## SUMMARY") {
		lines := strings.SplitN(strings.TrimSpace(part), "\n", 2)
		if len(lines) < 2 {
			continue
		}
		for _, item := range items {
			if strings.Contains(strings.TrimSpace(lines[0]), item.IssueURL) {
				results[item.IssueURL] = BatchResult{Summary: strings.TrimSpace(lines[1])}
				break
			}
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("failed to parse batch response in both JSON and markdown formats")
	}
	return results, nil
}

func (c *CopilotSummarizer) buildDescribePrompt(items []DescribeBatchItem) (string, error) {
	request := describeRequest{Items: make([]describeRequestItem, len(items))}
	for i, item := range items {
		request.Items[i] = describeRequestItem{ID: item.IssueURL, Issue: item.IssueTitle, Body: item.IssueBody}
	}
	text, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal describe request: %w", err)
	}
	return string(text), nil
}

func (c *CopilotSummarizer) parseDescribeResponse(response string, items []DescribeBatchItem) (map[string]string, error) {
	var results map[string]string
	if err := json.Unmarshal([]byte(response), &results); err == nil {
		return results, nil
	}

	results = make(map[string]string)
	for _, part := range strings.Split(response, "## ") {
		lines := strings.SplitN(strings.TrimSpace(part), "\n", 2)
		if len(lines) < 2 {
			continue
		}
		for _, item := range items {
			if strings.Contains(strings.TrimSpace(lines[0]), item.IssueURL) {
				results[item.IssueURL] = strings.TrimSpace(lines[1])
				break
			}
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("failed to parse describe response in both JSON and markdown formats")
	}
	return results, nil
}

func (c *CopilotSummarizer) getSystemPrompt() string {
	if c.systemPrompt != "" {
		return c.systemPrompt
	}
	return defaultSystemPrompt
}

func (c *CopilotSummarizer) complete(ctx context.Context, systemPrompt, prompt string) (result string, err error) {
	session, err := c.runtime.CreateSession(ctx, sessionOptions{
		Model:        c.model,
		SystemPrompt: systemPrompt,
	})
	if err != nil {
		return "", fmt.Errorf("create Copilot session: %w", err)
	}
	defer func() {
		if disconnectErr := session.Disconnect(); disconnectErr != nil {
			err = errors.Join(err, fmt.Errorf("disconnect Copilot session: %w", disconnectErr))
		}
	}()

	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result, err = session.SendAndWait(requestCtx, prompt)
	if err != nil && requestCtx.Err() != nil {
		abortCtx, abortCancel := context.WithTimeout(context.Background(), abortTimeout)
		_ = session.Abort(abortCtx)
		abortCancel()
	}
	if err != nil {
		return "", fmt.Errorf("complete Copilot request: %w", err)
	}
	return result, nil
}

func buildSummaryPrompt(title, url, update string) string {
	return fmt.Sprintf("Issue: %s (%s)\nUpdate:\n%s", title, url, update)
}

func buildManySummaryPrompt(title, url string, updates []string) string {
	var prompt strings.Builder
	_, _ = fmt.Fprintf(&prompt, "Issue: %s (%s)\nUpdates (newest first):", title, url)
	for i, update := range updates {
		_, _ = fmt.Fprintf(&prompt, "\n%d) %s", i+1, update)
	}
	return prompt.String()
}

type sdkCopilotRuntime struct {
	client *copilot.Client
}

func newSDKCopilotRuntime(client *copilot.Client) copilotRuntime {
	return &sdkCopilotRuntime{client: client}
}

func (r *sdkCopilotRuntime) Start(ctx context.Context) error {
	return r.client.Start(ctx)
}

func (r *sdkCopilotRuntime) CreateSession(ctx context.Context, options sessionOptions) (copilotSession, error) {
	session, err := r.client.CreateSession(ctx, sdkSessionConfig(options))
	if err != nil {
		return nil, fmt.Errorf("create Copilot session: %w", err)
	}
	return &sdkCopilotSession{session: session}, nil
}

func (r *sdkCopilotRuntime) Stop() error {
	return r.client.Stop()
}

func (r *sdkCopilotRuntime) ForceStop() {
	r.client.ForceStop()
}

type sdkCopilotSession struct {
	session *copilot.Session
}

func (s *sdkCopilotSession) SendAndWait(ctx context.Context, prompt string) (string, error) {
	event, err := s.session.SendAndWait(ctx, copilot.MessageOptions{Prompt: prompt})
	if err != nil {
		return "", fmt.Errorf("send Copilot message: %w", err)
	}
	return assistantContent(event)
}

func (s *sdkCopilotSession) Abort(ctx context.Context) error {
	return s.session.Abort(ctx)
}

func (s *sdkCopilotSession) Disconnect() error {
	return s.session.Disconnect()
}

func sdkSessionConfig(options sessionOptions) *copilot.SessionConfig {
	return &copilot.SessionConfig{
		Model:                   options.Model,
		EnableConfigDiscovery:   copilot.Bool(false),
		EnableSkills:            copilot.Bool(false),
		EnableFileHooks:         copilot.Bool(false),
		EnableHostGitOperations: copilot.Bool(false),
		EnableSessionStore:      copilot.Bool(false),
		Tools:                   []copilot.Tool{},
		AvailableTools:          []string{},
		SystemMessage: &copilot.SystemMessageConfig{
			Mode:    "append",
			Content: options.SystemPrompt,
		},
	}
}

func assistantContent(event *copilot.SessionEvent) (string, error) {
	if event == nil {
		return "", fmt.Errorf("Copilot session returned no final event")
	}
	message, ok := event.Data.(*copilot.AssistantMessageData)
	if !ok {
		return "", fmt.Errorf("Copilot session returned unexpected final event data %T", event.Data)
	}
	return message.Content, nil
}
