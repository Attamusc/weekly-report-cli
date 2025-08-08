package config

import (
	"errors"
	"os"
)

// Config holds all configuration for the application
type Config struct {
	GitHubToken string
	SinceDays   int
	Concurrency int
	Notes       bool
	Models      struct {
		BaseURL string
		Model   string
		Enabled bool
	}
}

// FromEnvAndFlags creates a Config from environment variables and CLI flags
func FromEnvAndFlags(sinceDays int, concurrency int, noNotes bool, inputPath string) (*Config, error) {
	config := &Config{
		GitHubToken: os.Getenv("GITHUB_TOKEN"),
		SinceDays:   sinceDays,
		Concurrency: concurrency,
		Notes:       !noNotes, // --no-notes inverts the boolean
	}

	// Validate required GitHub token
	if config.GitHubToken == "" {
		return nil, errors.New("GITHUB_TOKEN environment variable is required")
	}

	// Set up AI models configuration
	config.Models.BaseURL = os.Getenv("GITHUB_MODELS_BASE_URL")
	if config.Models.BaseURL == "" {
		config.Models.BaseURL = "https://models.github.ai/v1"
	}

	config.Models.Model = os.Getenv("GITHUB_MODELS_MODEL")
	if config.Models.Model == "" {
		config.Models.Model = "gpt-4o-mini"
	}

	// Check if AI summarization is disabled
	config.Models.Enabled = os.Getenv("DISABLE_SUMMARY") == ""

	return config, nil
}