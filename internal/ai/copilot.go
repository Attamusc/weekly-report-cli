package ai

import (
	"context"
	"fmt"

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
