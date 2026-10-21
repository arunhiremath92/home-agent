package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"
)

type Provider string

const (
	ProviderClaude   Provider = "claude"
	ProviderOllama   Provider = "ollama"
	ProviderChatGPT  Provider = "chatgpt"
	ProviderLocal    Provider = "local"
	anthropicURL              = "https://api.anthropic.com/v1"
	anthropicVersion          = "2023-06-01"
)

type Client interface {
	SendRequest(context.Context, []ModelMessage) (ModelResponse, error)
}

type Agent struct {
	httpClient *http.Client
	serverURL  string
	model      string
	apiKey     string
	provider   Provider
}

func NewAgent() *Agent {
	provider := providerFromEnvironment()
	serverURL, model, apiKey := settingsFromEnvironment(provider)
	return &Agent{
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		serverURL: serverURL,
		model:     model,
		apiKey:    apiKey,
		provider:  provider,
	}
}

func NewAgentFor(provider Provider) (*Agent, error) {
	switch provider {
	case ProviderClaude, ProviderOllama, ProviderChatGPT, ProviderLocal:
	default:
		return nil, fmt.Errorf("unsupported agent provider %q", provider)
	}
	serverURL, model, apiKey := settingsFromEnvironment(provider)
	if model == "" {
		return nil, fmt.Errorf("model is required for agent provider %q", provider)
	}
	if provider == ProviderClaude && apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY must be set for Claude")
	}
	if provider == ProviderChatGPT && apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY must be set for ChatGPT")
	}
	return &Agent{
		httpClient: &http.Client{Timeout: 5 * time.Minute},
		serverURL:  serverURL,
		model:      model,
		apiKey:     apiKey,
		provider:   provider,
	}, nil
}

func NewConfiguredAgent() (*Agent, error) {
	return NewAgentFor(providerFromEnvironment())
}

const (
	SERVER_URL         = "http://localhost:11434/v1"
	OPENAI_SERVER_URL  = "https://api.openai.com/v1"
	CHAT_ENDPOINT      = "chat/completions"
	serverURLEnv       = "MODEL_SERVER_URL"
	modelNameEnv       = "MODEL_NAME"
	openAIModelEnv     = "OPENAI_MODEL"
	openAIKeyEnv       = "OPENAI_API_KEY"
	legacyOpenAIKeyEnv = "OPEN_API_KEY"
	anthropicKeyEnv    = "ANTHROPIC_API_KEY"
	agentProviderEnv   = "AGENT_PROVIDER"
	anthropicModelEnv  = "ANTHROPIC_MODEL"
	defaultModelName   = "qwen3.5:0.8b"
	defaultOpenAIModel = "gpt-4o-mini"
	defaultClaudeModel = "claude-3-5-sonnet-latest"
)

func providerFromEnvironment() Provider {
	if provider := Provider(strings.ToLower(strings.TrimSpace(os.Getenv(agentProviderEnv)))); provider != "" {
		return provider
	}
	if strings.TrimSpace(os.Getenv(anthropicKeyEnv)) != "" {
		return ProviderClaude
	}
	if apiKeyFromEnvironment() != "" {
		return ProviderChatGPT
	}
	return ProviderOllama
}

func settingsFromEnvironment(provider Provider) (string, string, string) {
	switch provider {
	case ProviderClaude:
		model := strings.TrimSpace(os.Getenv(anthropicModelEnv))
		if model == "" {
			model = defaultClaudeModel
		}
		return anthropicURL, model, strings.TrimSpace(os.Getenv(anthropicKeyEnv))
	case ProviderChatGPT:
		model := strings.TrimSpace(os.Getenv(openAIModelEnv))
		if model == "" {
			model = defaultOpenAIModel
		}
		return OPENAI_SERVER_URL, model, apiKeyFromEnvironment()
	case ProviderLocal:
		serverURL := strings.TrimSpace(os.Getenv(serverURLEnv))
		if serverURL == "" {
			serverURL = SERVER_URL
		}
		model := strings.TrimSpace(os.Getenv(modelNameEnv))
		if model == "" {
			model = defaultModelName
		}
		return serverURL, model, ""
	default:
		serverURL := strings.TrimSpace(os.Getenv(serverURLEnv))
		if serverURL == "" {
			serverURL = SERVER_URL
		}
		model := strings.TrimSpace(os.Getenv(modelNameEnv))
		if model == "" {
			model = defaultModelName
		}
		return serverURL, model, ""
	}
}

func apiKeyFromEnvironment() string {
	if key := strings.TrimSpace(os.Getenv(openAIKeyEnv)); key != "" {
		return key
	}
	return strings.TrimSpace(os.Getenv(legacyOpenAIKeyEnv))
}

func serverURLFromEnvironment() string {
	if apiKeyFromEnvironment() != "" {
		return OPENAI_SERVER_URL
	}
	serverURL := strings.TrimSpace(os.Getenv(serverURLEnv))
	if serverURL == "" {
		return SERVER_URL
	}
	return serverURL
}

func modelFromEnvironment() string {
	if apiKeyFromEnvironment() != "" {
		if model := strings.TrimSpace(os.Getenv(openAIModelEnv)); model != "" {
			return model
		}
		return defaultOpenAIModel
	}
	modelName := strings.TrimSpace(os.Getenv(modelNameEnv))
	if modelName == "" {
		return defaultModelName
	}
	return modelName
}

type ModelMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ModelChoice struct {
	Index        int          `json:"index"`
	Message      ModelMessage `json:"message"`
	FinishReason string       `json:"finish_reason"`
}

type ModelResponse struct {
	ID                string        `json:"id"`
	Object            string        `json:"object"`
	Created           int64         `json:"created"`
	Model             string        `json:"model"`
	SystemFingerprint string        `json:"system_fingerprint"`
	Choices           []ModelChoice `json:"choices"`
	Usage             ModelUsage    `json:"usage"`
}

type ModelUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type chatRequest struct {
	Model    string         `json:"model"`
	Messages []ModelMessage `json:"messages"`
}

const chatPrompt = "You are a helpful assistant. Please provide a concise and informative response to the users question. If you don't know the answer, please say 'I don't know'."

func (a *Agent) SendChatRequest(userMessage string) (ModelResponse, error) {
	return a.SendRequest(context.Background(), []ModelMessage{
		{Role: "system", Content: chatPrompt},
		{Role: "user", Content: userMessage},
	})
}

func (a *Agent) SendRequest(ctx context.Context, messages []ModelMessage) (ModelResponse, error) {
	if a == nil {
		return ModelResponse{}, fmt.Errorf("agent is not initialized")
	}
	if a.httpClient == nil {
		return ModelResponse{}, fmt.Errorf("agent HTTP client is not initialized")
	}
	model := a.model
	if model == "" {
		model = modelFromEnvironment()
	}
	if len(messages) == 0 {
		return ModelResponse{}, fmt.Errorf("at least one message is required")
	}
	slog.Debug("sending agent request", "provider", a.provider, "model", model, "messages", len(messages))
	if a.provider == ProviderClaude {
		return a.sendClaudeRequest(ctx, model, messages)
	}

	requestBody, err := json.Marshal(chatRequest{
		Model:    model,
		Messages: messages,
	})
	if err != nil {
		return ModelResponse{}, fmt.Errorf("failed to encode chat request: %w", err)
	}

	url := strings.TrimRight(a.serverURL, "/") + "/" + strings.TrimLeft(CHAT_ENDPOINT, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(requestBody))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.apiKey)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("failed to send chat request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return ModelResponse{}, fmt.Errorf("chat request returned HTTP %d and response body could not be read: %w", resp.StatusCode, readErr)
		}
		return ModelResponse{}, fmt.Errorf("chat request returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}

	var modelResponse ModelResponse
	if err := json.NewDecoder(resp.Body).Decode(&modelResponse); err != nil {
		return ModelResponse{}, fmt.Errorf("failed to decode chat response: %w", err)
	}

	if len(modelResponse.Choices) == 0 {
		return ModelResponse{}, fmt.Errorf("chat response contains no choices")
	}

	return modelResponse, nil
}

func (a *Agent) sendClaudeRequest(ctx context.Context, model string, messages []ModelMessage) (ModelResponse, error) {
	system := ""
	claudeMessages := make([]ModelMessage, 0, len(messages))
	for _, message := range messages {
		if message.Role == "system" {
			system += message.Content + "\n"
			continue
		}
		claudeMessages = append(claudeMessages, message)
	}
	if len(claudeMessages) == 0 {
		return ModelResponse{}, fmt.Errorf("Claude request requires at least one non-system message")
	}
	requestBody, err := json.Marshal(struct {
		Model     string         `json:"model"`
		MaxTokens int            `json:"max_tokens"`
		System    string         `json:"system,omitempty"`
		Messages  []ModelMessage `json:"messages"`
	}{model, 1024, strings.TrimSpace(system), claudeMessages})
	if err != nil {
		return ModelResponse{}, fmt.Errorf("failed to encode Claude request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(a.serverURL, "/")+"/messages", bytes.NewReader(requestBody))
	if err != nil {
		return ModelResponse{}, fmt.Errorf("failed to create Claude request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("failed to send Claude request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		errorBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if readErr != nil {
			return ModelResponse{}, fmt.Errorf("Claude request returned HTTP %d and response body could not be read: %w", resp.StatusCode, readErr)
		}
		return ModelResponse{}, fmt.Errorf("Claude request returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(errorBody)))
	}
	var response struct {
		ID      string `json:"id"`
		Model   string `json:"model"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return ModelResponse{}, fmt.Errorf("failed to decode Claude response: %w", err)
	}
	var text strings.Builder
	for _, block := range response.Content {
		if block.Type == "text" {
			text.WriteString(block.Text)
		}
	}
	if text.Len() == 0 {
		return ModelResponse{}, fmt.Errorf("Claude response contains no text")
	}
	return ModelResponse{
		ID:      response.ID,
		Model:   response.Model,
		Choices: []ModelChoice{{Message: ModelMessage{Role: "assistant", Content: text.String()}}},
		Usage: ModelUsage{
			PromptTokens:     response.Usage.InputTokens,
			CompletionTokens: response.Usage.OutputTokens,
			TotalTokens:      response.Usage.InputTokens + response.Usage.OutputTokens,
		},
	}, nil
}
