package ai

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
}

// NewCopilotSummarizer creates a Copilot SDK-backed summarizer.
func NewCopilotSummarizer(runtime copilotRuntime, model, systemPrompt string) *CopilotSummarizer {
	return &CopilotSummarizer{
		runtime:      runtime,
		model:        model,
		systemPrompt: systemPrompt,
	}
}

// Summarize generates a summary for a single update.
func (c *CopilotSummarizer) Summarize(ctx context.Context, title, url, update string) (string, error) {
	return c.complete(ctx, c.getSystemPrompt(), buildSummaryPrompt(title, url, update))
}

// SummarizeMany generates a summary for multiple updates ordered newest first.
func (c *CopilotSummarizer) SummarizeMany(ctx context.Context, title, url string, updates []string) (string, error) {
	return c.complete(ctx, c.getSystemPrompt(), buildManySummaryPrompt(title, url, updates))
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

	result, err = session.SendAndWait(ctx, prompt)
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
