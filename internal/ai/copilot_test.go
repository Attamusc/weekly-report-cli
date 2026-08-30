package ai

import (
	"context"
	"errors"
	"strings"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
)

type fakeCopilotRuntime struct {
	session        copilotSession
	createErr      error
	createdOptions []sessionOptions
}

func (f *fakeCopilotRuntime) Start(context.Context) error { return nil }
func (f *fakeCopilotRuntime) CreateSession(_ context.Context, options sessionOptions) (copilotSession, error) {
	f.createdOptions = append(f.createdOptions, options)
	return f.session, f.createErr
}
func (f *fakeCopilotRuntime) Stop() error { return nil }
func (f *fakeCopilotRuntime) ForceStop()  {}

type fakeCopilotSession struct {
	response        string
	sendErr         error
	disconnectErr   error
	prompts         []string
	disconnectCalls int
}

func (f *fakeCopilotSession) SendAndWait(_ context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	return f.response, f.sendErr
}
func (f *fakeCopilotSession) Abort(context.Context) error { return nil }
func (f *fakeCopilotSession) Disconnect() error {
	f.disconnectCalls++
	return f.disconnectErr
}

func TestCopilotInterfacesAcceptProjectOwnedFakes(t *testing.T) {
	var session copilotSession = &fakeCopilotSession{response: "response"}
	var runtime copilotRuntime = &fakeCopilotRuntime{session: session}

	created, err := runtime.CreateSession(context.Background(), sessionOptions{})
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}
	got, err := created.SendAndWait(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("SendAndWait() error = %v", err)
	}
	if got != "response" {
		t.Errorf("SendAndWait() = %q, want response", got)
	}
}

func TestCopilotSummarizerSummarize(t *testing.T) {
	session := &fakeCopilotSession{response: "Completed OAuth2 integration."}
	runtime := &fakeCopilotRuntime{session: session}
	client := NewCopilotSummarizer(runtime, "test-model", "custom instructions")

	got, err := client.Summarize(context.Background(), "Implement authentication", "https://github.com/org/repo/issues/1", "OAuth2 is complete.")
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if got != "Completed OAuth2 integration." {
		t.Errorf("Summarize() = %q, want response text", got)
	}
	if len(runtime.createdOptions) != 1 {
		t.Fatalf("CreateSession() calls = %d, want 1", len(runtime.createdOptions))
	}
	options := runtime.createdOptions[0]
	if options.Model != "test-model" || options.SystemPrompt != "custom instructions" {
		t.Errorf("session options = %#v, want configured model and system prompt", options)
	}
	wantPrompt := "Issue: Implement authentication (https://github.com/org/repo/issues/1)\nUpdate:\nOAuth2 is complete."
	if len(session.prompts) != 1 || session.prompts[0] != wantPrompt {
		t.Errorf("prompts = %#v, want %q", session.prompts, wantPrompt)
	}
	if session.disconnectCalls != 1 {
		t.Errorf("Disconnect() calls = %d, want 1", session.disconnectCalls)
	}
}

func TestCopilotSummarizerSummarizeMany(t *testing.T) {
	session := &fakeCopilotSession{response: "Combined summary."}
	runtime := &fakeCopilotRuntime{session: session}
	client := NewCopilotSummarizer(runtime, "test-model", "")

	got, err := client.SummarizeMany(context.Background(), "Multiple updates", "https://github.com/org/repo/issues/2", []string{"Newest update", "Older update"})
	if err != nil {
		t.Fatalf("SummarizeMany() error = %v", err)
	}
	if got != "Combined summary." {
		t.Errorf("SummarizeMany() = %q, want response text", got)
	}
	wantPrompt := "Issue: Multiple updates (https://github.com/org/repo/issues/2)\nUpdates (newest first):\n1) Newest update\n2) Older update"
	if len(session.prompts) != 1 || session.prompts[0] != wantPrompt {
		t.Errorf("prompts = %#v, want %q", session.prompts, wantPrompt)
	}
	if runtime.createdOptions[0].SystemPrompt != defaultSystemPrompt {
		t.Error("SummarizeMany() did not use the default system prompt")
	}
	if session.disconnectCalls != 1 {
		t.Errorf("Disconnect() calls = %d, want 1", session.disconnectCalls)
	}
}

func TestCopilotSummarizerDisconnectsOnFailure(t *testing.T) {
	session := &fakeCopilotSession{sendErr: errors.New("inference failed")}
	client := NewCopilotSummarizer(&fakeCopilotRuntime{session: session}, "test-model", "")

	_, err := client.Summarize(context.Background(), "Issue", "url", "update")
	if err == nil || !strings.Contains(err.Error(), "inference failed") {
		t.Fatalf("Summarize() error = %v, want inference failure", err)
	}
	if session.disconnectCalls != 1 {
		t.Errorf("Disconnect() calls = %d, want 1", session.disconnectCalls)
	}
}

func TestCopilotSummarizerReturnsDisconnectFailure(t *testing.T) {
	session := &fakeCopilotSession{response: "summary", disconnectErr: errors.New("disconnect failed")}
	client := NewCopilotSummarizer(&fakeCopilotRuntime{session: session}, "test-model", "")

	_, err := client.Summarize(context.Background(), "Issue", "url", "update")
	if err == nil || !strings.Contains(err.Error(), "disconnect failed") {
		t.Fatalf("Summarize() error = %v, want disconnect failure", err)
	}
}

func TestSDKSessionConfigDisablesHostCapabilities(t *testing.T) {
	config := sdkSessionConfig(sessionOptions{Model: "test-model", SystemPrompt: "instructions"})

	if config.Model != "test-model" {
		t.Errorf("Model = %q, want test-model", config.Model)
	}
	if config.SystemMessage == nil || config.SystemMessage.Mode != "append" || config.SystemMessage.Content != "instructions" {
		t.Errorf("SystemMessage = %#v, want append-mode instructions", config.SystemMessage)
	}
	assertFalse := func(name string, value *bool) {
		t.Helper()
		if value == nil || *value {
			t.Errorf("%s = %v, want non-nil false", name, value)
		}
	}
	assertFalse("EnableConfigDiscovery", config.EnableConfigDiscovery)
	assertFalse("EnableSkills", config.EnableSkills)
	assertFalse("EnableFileHooks", config.EnableFileHooks)
	assertFalse("EnableHostGitOperations", config.EnableHostGitOperations)
	assertFalse("EnableSessionStore", config.EnableSessionStore)
	if config.AvailableTools == nil || len(config.AvailableTools) != 0 {
		t.Errorf("AvailableTools = %#v, want non-nil empty list", config.AvailableTools)
	}
	if config.Tools == nil || len(config.Tools) != 0 {
		t.Errorf("Tools = %#v, want non-nil empty list", config.Tools)
	}
}

func TestAssistantContent(t *testing.T) {
	tests := []struct {
		name    string
		event   *copilot.SessionEvent
		want    string
		wantErr bool
	}{
		{name: "assistant message", event: &copilot.SessionEvent{Data: &copilot.AssistantMessageData{Content: "summary"}}, want: "summary"},
		{name: "nil event", event: nil, wantErr: true},
		{name: "unexpected data", event: &copilot.SessionEvent{Data: &copilot.SessionIdleData{}}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := assistantContent(tc.event)
			if (err != nil) != tc.wantErr {
				t.Fatalf("assistantContent() error = %v, wantErr %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Errorf("assistantContent() = %q, want %q", got, tc.want)
			}
		})
	}
}
