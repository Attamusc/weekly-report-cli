package ai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	copilot "github.com/github/copilot-sdk/go"
)

type fakeCopilotRuntime struct {
	session        copilotSession
	createErr      error
	stopErr        error
	createdOptions []sessionOptions
	stopCalls      int
	forceStopCalls int
}

func (f *fakeCopilotRuntime) Start(context.Context) error { return nil }
func (f *fakeCopilotRuntime) CreateSession(_ context.Context, options sessionOptions) (copilotSession, error) {
	f.createdOptions = append(f.createdOptions, options)
	return f.session, f.createErr
}
func (f *fakeCopilotRuntime) Stop() error {
	f.stopCalls++
	return f.stopErr
}
func (f *fakeCopilotRuntime) ForceStop() { f.forceStopCalls++ }

type fakeCopilotSession struct {
	response        string
	sendErr         error
	disconnectErr   error
	prompts         []string
	disconnectCalls int
	send            func(context.Context) (string, error)
	abortCalls      int
	abortContextErr error
}

func (f *fakeCopilotSession) SendAndWait(ctx context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	if f.send != nil {
		return f.send(ctx)
	}
	return f.response, f.sendErr
}
func (f *fakeCopilotSession) Abort(ctx context.Context) error {
	f.abortCalls++
	f.abortContextErr = ctx.Err()
	return nil
}
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
	client := NewCopilotSummarizer(runtime, "test-model", "custom instructions", time.Second)

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
	client := NewCopilotSummarizer(runtime, "test-model", "", time.Second)

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

func TestCopilotSummarizerBatchDescriptionAndHeader(t *testing.T) {
	t.Run("summary batch preserves nested URL keyed results", func(t *testing.T) {
		session := &fakeCopilotSession{response: `{"https://github.com/org/repo/issues/1":{"summary":"Shipped.","sentiment":{"status":"on_track","explanation":"Work completed."}}}`}
		runtime := &fakeCopilotRuntime{session: session}
		client := NewCopilotSummarizer(runtime, "test-model", "", time.Second)
		items := []BatchItem{{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "Feature", UpdateTexts: []string{"Shipped"}, ReportedStatus: "On Track"}}

		got, err := client.SummarizeBatch(context.Background(), items)
		if err != nil {
			t.Fatalf("SummarizeBatch() error = %v", err)
		}
		if got[items[0].IssueURL].Summary != "Shipped." || got[items[0].IssueURL].Sentiment == nil {
			t.Errorf("SummarizeBatch() = %#v, want nested result", got)
		}
		if len(runtime.createdOptions) != 1 || runtime.createdOptions[0].SystemPrompt != batchSystemPrompt {
			t.Errorf("created options = %#v, want one batch session", runtime.createdOptions)
		}
	})

	t.Run("description batch preserves partial results", func(t *testing.T) {
		session := &fakeCopilotSession{response: `{"https://github.com/org/repo/issues/1":"Project description."}`}
		runtime := &fakeCopilotRuntime{session: session}
		client := NewCopilotSummarizer(runtime, "test-model", "", time.Second)
		items := []DescribeBatchItem{
			{IssueURL: "https://github.com/org/repo/issues/1", IssueTitle: "One", IssueBody: "Body one"},
			{IssueURL: "https://github.com/org/repo/issues/2", IssueTitle: "Two", IssueBody: "Body two"},
		}

		got, err := client.DescribeBatch(context.Background(), items)
		if err != nil {
			t.Fatalf("DescribeBatch() error = %v", err)
		}
		if len(got) != 1 || got[items[0].IssueURL] != "Project description." {
			t.Errorf("DescribeBatch() = %#v, want one partial result", got)
		}
		if runtime.createdOptions[0].SystemPrompt != describeSystemPrompt {
			t.Error("DescribeBatch() did not use description system prompt")
		}
	})

	t.Run("header uses header contract", func(t *testing.T) {
		session := &fakeCopilotSession{response: "Good progress this week."}
		runtime := &fakeCopilotRuntime{session: session}
		client := NewCopilotSummarizer(runtime, "test-model", "", time.Second)

		got, err := client.GenerateHeader(context.Background(), []HeaderItem{{StatusCaption: "Done", Title: "Feature", Summary: "Shipped."}})
		if err != nil {
			t.Fatalf("GenerateHeader() error = %v", err)
		}
		if got != "Good progress this week." {
			t.Errorf("GenerateHeader() = %q", got)
		}
		if runtime.createdOptions[0].SystemPrompt != headerSystemPrompt || !strings.Contains(session.prompts[0], `"title":"Feature"`) {
			t.Errorf("header session options/prompts = %#v / %#v", runtime.createdOptions, session.prompts)
		}
	})
}

func TestCopilotSummarizerBatchMalformedResponse(t *testing.T) {
	client := NewCopilotSummarizer(&fakeCopilotRuntime{session: &fakeCopilotSession{response: "not structured"}}, "test-model", "", time.Second)
	_, err := client.SummarizeBatch(context.Background(), []BatchItem{{IssueURL: "url"}})
	if err == nil || !strings.Contains(err.Error(), "failed to parse batch response") {
		t.Fatalf("SummarizeBatch() error = %v, want parse error", err)
	}
}

func TestCopilotSummarizerBatchChunksIntoIsolatedSessions(t *testing.T) {
	session := &fakeCopilotSession{response: `{"url":"summary"}`}
	runtime := &fakeCopilotRuntime{session: session}
	client := NewCopilotSummarizer(runtime, "test-model", "", time.Second)
	items := make([]BatchItem, maxBatchSize+1)
	for i := range items {
		items[i].IssueURL = "url"
	}

	if _, err := client.SummarizeBatch(context.Background(), items); err != nil {
		t.Fatalf("SummarizeBatch() error = %v", err)
	}
	if len(runtime.createdOptions) != 2 || session.disconnectCalls != 2 {
		t.Errorf("sessions created/disconnected = %d/%d, want 2/2", len(runtime.createdOptions), session.disconnectCalls)
	}
}

func TestCopilotSummarizerDisconnectsOnFailure(t *testing.T) {
	session := &fakeCopilotSession{sendErr: errors.New("inference failed")}
	client := NewCopilotSummarizer(&fakeCopilotRuntime{session: session}, "test-model", "", time.Second)

	_, err := client.Summarize(context.Background(), "Issue", "url", "update")
	if err == nil || !strings.Contains(err.Error(), "inference failed") {
		t.Fatalf("Summarize() error = %v, want inference failure", err)
	}
	if session.disconnectCalls != 1 {
		t.Errorf("Disconnect() calls = %d, want 1", session.disconnectCalls)
	}
}

func TestCopilotSummarizerTimeoutAbortsWithFreshContextAndDisconnects(t *testing.T) {
	session := &fakeCopilotSession{send: func(ctx context.Context) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}
	client := NewCopilotSummarizer(&fakeCopilotRuntime{session: session}, "test-model", "", time.Nanosecond)

	_, err := client.Summarize(context.Background(), "Issue", "url", "update")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Summarize() error = %v, want deadline exceeded", err)
	}
	if session.abortCalls != 1 {
		t.Errorf("Abort() calls = %d, want 1", session.abortCalls)
	}
	if session.abortContextErr != nil {
		t.Errorf("Abort() context error = %v, want live cleanup context", session.abortContextErr)
	}
	if session.disconnectCalls != 1 {
		t.Errorf("Disconnect() calls = %d, want 1", session.disconnectCalls)
	}
}

func TestCopilotSummarizerCleanupIsIdempotent(t *testing.T) {
	t.Run("graceful stop", func(t *testing.T) {
		runtime := &fakeCopilotRuntime{}
		client := NewCopilotSummarizer(runtime, "test-model", "", time.Second)

		if err := client.Cleanup(); err != nil {
			t.Fatalf("Cleanup() error = %v", err)
		}
		if err := client.Cleanup(); err != nil {
			t.Fatalf("second Cleanup() error = %v", err)
		}
		if runtime.stopCalls != 1 || runtime.forceStopCalls != 0 {
			t.Errorf("Stop()/ForceStop() calls = %d/%d, want 1/0", runtime.stopCalls, runtime.forceStopCalls)
		}
	})

	t.Run("failed stop forces shutdown", func(t *testing.T) {
		runtime := &fakeCopilotRuntime{stopErr: errors.New("stop failed")}
		client := NewCopilotSummarizer(runtime, "test-model", "", time.Second)

		firstErr := client.Cleanup()
		secondErr := client.Cleanup()
		if firstErr == nil || secondErr == nil || firstErr.Error() != secondErr.Error() {
			t.Fatalf("Cleanup() errors = %v, %v, want same stop failure", firstErr, secondErr)
		}
		if runtime.stopCalls != 1 || runtime.forceStopCalls != 1 {
			t.Errorf("Stop()/ForceStop() calls = %d/%d, want 1/1", runtime.stopCalls, runtime.forceStopCalls)
		}
	})
}

func TestCopilotSummarizerReturnsDisconnectFailure(t *testing.T) {
	session := &fakeCopilotSession{response: "summary", disconnectErr: errors.New("disconnect failed")}
	client := NewCopilotSummarizer(&fakeCopilotRuntime{session: session}, "test-model", "", time.Second)

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
