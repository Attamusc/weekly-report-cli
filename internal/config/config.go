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
	Models      struct {
		BaseURL      string
		Model        string
		Enabled      bool
		SystemPrompt string
		Sentiment    bool          // true by default when AI enabled, false with --no-sentiment
		Timeout      time.Duration // HTTP timeout for AI API requests
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

	// Set up AI models configuration
	config.Models.BaseURL = os.Getenv("GITHUB_MODELS_BASE_URL")
	if config.Models.BaseURL == "" {
		config.Models.BaseURL = "https://models.github.ai"
	}

	config.Models.Model = os.Getenv("GITHUB_MODELS_MODEL")
	if config.Models.Model == "" {
		config.Models.Model = "gpt-5-mini"
	}

	// Check if AI summarization is disabled
	config.Models.Enabled = os.Getenv("DISABLE_SUMMARY") == ""

	// Set custom system prompt if provided
	config.Models.SystemPrompt = in.SummaryPrompt

	// Sentiment analysis is on by default when AI is enabled
	config.Models.Sentiment = config.Models.Enabled && !in.NoSentiment

	// AI API timeout: configurable via AI_TIMEOUT env var (in seconds), default 120s
	config.Models.Timeout = 120 * time.Second
	if timeoutStr := os.Getenv("AI_TIMEOUT"); timeoutStr != "" {
		timeoutSec, err := strconv.Atoi(timeoutStr)
		if err != nil {
			return nil, errors.New("AI_TIMEOUT must be an integer (seconds)")
		}
		if timeoutSec > 0 {
			config.Models.Timeout = time.Duration(timeoutSec) * time.Second
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
