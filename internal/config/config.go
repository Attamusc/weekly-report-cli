package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

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

// FromEnvAndFlags creates a Config from environment variables and CLI flags
func FromEnvAndFlags(sinceDays int, concurrency int, noNotes bool, verbose bool, quiet bool, inputPath string, summaryPrompt string, projectURL string, projectField string, projectFieldValues []string, projectIncludePRs bool, projectMaxItems int, projectView string, projectViewID string, noSentiment bool) (*Config, error) {
	// Load environment variables from .env file if it exists
	_ = godotenv.Load() // Silently ignore if .env file doesn't exist
	config := &Config{
		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		SinceDays:   sinceDays,
		Concurrency: concurrency,
		Notes:       !noNotes,          // --no-notes inverts the boolean
		Verbose:     verbose && !quiet, // verbose is disabled if quiet is set
		Quiet:       quiet,
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
	config.Models.SystemPrompt = summaryPrompt

	// Sentiment analysis is on by default when AI is enabled
	config.Models.Sentiment = config.Models.Enabled && !noSentiment

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
	config.Project.URL = projectURL
	config.Project.FieldName = projectField
	config.Project.FieldValues = projectFieldValues
	config.Project.IncludePRs = projectIncludePRs
	config.Project.MaxItems = projectMaxItems
	config.Project.ViewName = projectView
	config.Project.ViewID = projectViewID

	return config, nil
}
