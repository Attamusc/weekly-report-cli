package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// ErrNoRows indicates no report rows were produced.
var ErrNoRows = errors.New("no rows produced")

// Config holds all configuration for the application
type Config struct {
	GitHubToken string
	SinceDays   int
	Concurrency int
	Notes       bool
	Verbose     bool
	Quiet       bool
	Copilot     struct {
		Token        string
		Model        string
		Enabled      bool
		SystemPrompt string
		Sentiment    bool
		Timeout      time.Duration
	}
	Project struct {
		URL         string
		FieldName   string
		FieldValues []string
		IncludePRs  bool
		MaxItems    int
		ViewName    string
		ViewID      string
	}
}

// ConfigInput holds the CLI flags and input parameters for creating a Config.
type ConfigInput struct {
	SinceDays          int
	Concurrency        int
	NoNotes            bool
	Verbose            bool
	Quiet              bool
	InputPath          string
	SummaryPrompt      string
	ProjectURL         string
	ProjectField       string
	ProjectFieldValues []string
	ProjectIncludePRs  bool
	ProjectMaxItems    int
	ProjectView        string
	ProjectViewID      string
	NoSentiment        bool
	DisableSummary     bool
}

// FromEnvAndFlags creates a Config from environment variables and CLI flags
func FromEnvAndFlags(in ConfigInput) (*Config, error) {
	// Load environment variables from .env file if it exists
	_ = godotenv.Load() // Silently ignore if .env file doesn't exist
	config := &Config{
		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		SinceDays:   in.SinceDays,
		Concurrency: in.Concurrency,
		Notes:       !in.NoNotes,             // --no-notes inverts the boolean
		Verbose:     in.Verbose && !in.Quiet, // verbose is disabled if quiet is set
		Quiet:       in.Quiet,
	}

	// Validate required GitHub token
	if config.GitHubToken == "" {
		return nil, errors.New("GITHUB_TOKEN environment variable is required")
	}

	config.Copilot.Token = os.Getenv("COPILOT_GITHUB_TOKEN")
	config.Copilot.Model = os.Getenv("COPILOT_MODEL")
	if config.Copilot.Model == "" {
		config.Copilot.Model = "claude-haiku-4.5"
	}
	config.Copilot.Enabled = os.Getenv("DISABLE_SUMMARY") == "" && !in.DisableSummary
	config.Copilot.SystemPrompt = in.SummaryPrompt
	config.Copilot.Sentiment = config.Copilot.Enabled && !in.NoSentiment
	config.Copilot.Timeout = 120 * time.Second
	if timeoutStr := os.Getenv("AI_TIMEOUT"); timeoutStr != "" {
		timeoutSec, err := strconv.Atoi(timeoutStr)
		if err != nil {
			return nil, errors.New("AI_TIMEOUT must be an integer (seconds)")
		}
		if timeoutSec > 0 {
			config.Copilot.Timeout = time.Duration(timeoutSec) * time.Second
		}
	}

	// Set up project configuration
	config.Project.URL = in.ProjectURL
	config.Project.FieldName = in.ProjectField
	config.Project.FieldValues = in.ProjectFieldValues
	config.Project.IncludePRs = in.ProjectIncludePRs
	config.Project.MaxItems = in.ProjectMaxItems
	config.Project.ViewName = in.ProjectView
	config.Project.ViewID = in.ProjectViewID

	return config, nil
}
