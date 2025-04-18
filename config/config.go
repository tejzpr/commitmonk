package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/ini.v1"
)

// Config represents application configuration
type Config struct {
	DefaultInterval string
	LLM             LLMConfig
}

// LLMConfig holds LLM API configuration
type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		DefaultInterval: "5m",
		LLM: LLMConfig{
			BaseURL: "https://api.openai.com/v1",
			Model:   "gpt-4",
		},
	}
}

// GetConfigDir returns the platform-specific config directory
func GetConfigDir() (string, error) {
	var configDir string

	switch runtime.GOOS {
	case "windows":
		configDir = filepath.Join(os.Getenv("APPDATA"), "commitmonk")
	case "darwin", "linux":
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		configDir = filepath.Join(homeDir, ".config", "commitmonk")
	default:
		return "", fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}

	return configDir, nil
}

// GetConfigFilePath returns the full path to the config file
func GetConfigFilePath() (string, error) {
	configDir, err := GetConfigDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(configDir, "config.ini"), nil
}

// LoadConfig loads the application configuration from the config file
func LoadConfig() (*Config, error) {
	configPath, err := GetConfigFilePath()
	if err != nil {
		return nil, err
	}

	// Check if config file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		// Create default config
		config := DefaultConfig()
		if err := config.Save(); err != nil {
			return nil, fmt.Errorf("failed to create default config: %w", err)
		}
		return config, nil
	}

	// Load existing config
	iniFile, err := ini.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()

	// Load default section
	defaultSection := iniFile.Section("default")
	if defaultSection != nil {
		config.DefaultInterval = defaultSection.Key("interval").MustString(config.DefaultInterval)
	}

	// Load LLM section
	llmSection := iniFile.Section("llm")
	if llmSection != nil {
		config.LLM.BaseURL = llmSection.Key("base_url").MustString(config.LLM.BaseURL)
		config.LLM.APIKey = llmSection.Key("api_key").String()
		config.LLM.Model = llmSection.Key("model").MustString(config.LLM.Model)
	}

	return config, nil
}

// Save writes the configuration to the config file
func (c *Config) Save() error {
	configPath, err := GetConfigFilePath()
	if err != nil {
		return err
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	iniFile := ini.Empty()

	// Save default section
	defaultSection, err := iniFile.NewSection("default")
	if err != nil {
		return fmt.Errorf("failed to create default section: %w", err)
	}
	_, err = defaultSection.NewKey("interval", c.DefaultInterval)
	if err != nil {
		return fmt.Errorf("failed to write interval key: %w", err)
	}

	// Save LLM section
	llmSection, err := iniFile.NewSection("llm")
	if err != nil {
		return fmt.Errorf("failed to create llm section: %w", err)
	}
	_, err = llmSection.NewKey("base_url", c.LLM.BaseURL)
	if err != nil {
		return fmt.Errorf("failed to write base_url key: %w", err)
	}
	_, err = llmSection.NewKey("api_key", c.LLM.APIKey)
	if err != nil {
		return fmt.Errorf("failed to write api_key key: %w", err)
	}
	_, err = llmSection.NewKey("model", c.LLM.Model)
	if err != nil {
		return fmt.Errorf("failed to write model key: %w", err)
	}

	// Write to file with restricted permissions
	if err := iniFile.SaveTo(configPath); err != nil {
		return fmt.Errorf("failed to save config file: %w", err)
	}

	// Set permission to 600 (user read/write only)
	if err := os.Chmod(configPath, 0600); err != nil {
		return fmt.Errorf("failed to set config file permissions: %w", err)
	}

	return nil
}
