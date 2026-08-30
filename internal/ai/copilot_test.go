package ai

import (
	"context"
	"testing"

	copilot "github.com/github/copilot-sdk/go"
)

type fakeCopilotRuntime struct {
	session copilotSession
}

func (f *fakeCopilotRuntime) Start(context.Context) error { return nil }
func (f *fakeCopilotRuntime) CreateSession(context.Context, sessionOptions) (copilotSession, error) {
	return f.session, nil
}
func (f *fakeCopilotRuntime) Stop() error { return nil }
func (f *fakeCopilotRuntime) ForceStop()  {}

type fakeCopilotSession struct{}

func (f *fakeCopilotSession) SendAndWait(context.Context, string) (string, error) {
	return "response", nil
}
func (f *fakeCopilotSession) Abort(context.Context) error { return nil }
func (f *fakeCopilotSession) Disconnect() error           { return nil }

func TestCopilotInterfacesAcceptProjectOwnedFakes(t *testing.T) {
	var session copilotSession = &fakeCopilotSession{}
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
