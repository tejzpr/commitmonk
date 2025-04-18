package llm

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/tejzpr/commitmonk/config"
)

// Client handles interactions with the LLM API
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
}

// NewClient creates a new LLM client from configuration
func NewClient(cfg config.LLMConfig) *Client {
	return &Client{
		BaseURL: cfg.BaseURL,
		APIKey:  cfg.APIKey,
		Model:   cfg.Model,
	}
}

// HasCredentials checks if the client has valid credentials
func (c *Client) HasCredentials() bool {
	return c.APIKey != ""
}

// Message represents a chat message in the API request
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest represents the request structure for chat models
type ChatRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// ChatResponse represents the response structure from chat models
type ChatResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// GenerateCommitMessage creates a commit message for the given diff
func (c *Client) GenerateCommitMessage(diff string) (string, error) {
	if !c.HasCredentials() {
		return "", fmt.Errorf("LLM API credentials not configured")
	}

	// Create prompt for the LLM
	prompt := fmt.Sprintf("You are a Git commit message generator. Your task is to write a clear, "+
		"concise commit message in the conventional commit format (type: description) based on the "+
		"following Git diff. Focus only on the most important changes, and keep the message under 72 characters. "+
		"Respond with ONLY the commit message, nothing else, do not add any other prefix or suffix.\n\nDiff:\n%s", diff)

	// Prepare the request
	chatReq := ChatRequest{
		Model: c.Model,
		Messages: []Message{
			{Role: "system", Content: "You generate concise git commit messages in conventional format."},
			{Role: "user", Content: prompt},
		},
		MaxTokens: 100,
	}

	reqBody, err := json.Marshal(chatReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Make HTTP request
	endpoint := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(c.BaseURL, "/"))
	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errorResponse struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&errorResponse); err == nil {
			return "", fmt.Errorf("API error: %s", errorResponse.Error.Message)
		}
		return "", fmt.Errorf("API returned non-200 status code: %d", resp.StatusCode)
	}

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from LLM")
	}

	// Trim any leading/trailing whitespace and quotes
	message := strings.TrimSpace(chatResp.Choices[0].Message.Content)
	message = strings.Trim(message, `"'`)

	return message, nil
}
