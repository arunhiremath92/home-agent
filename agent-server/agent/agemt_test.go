package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSendChatRequest(t *testing.T) {
	const (
		userMessage   = "Should the task run?"
		modelResponse = "Yes, there is enough time."
	)
	t.Setenv(serverURLEnv, SERVER_URL)
	t.Setenv(modelNameEnv, defaultModelName)
	t.Setenv(openAIKeyEnv, "")
	t.Setenv(legacyOpenAIKeyEnv, "")

	agent := NewAgent()
	agent.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.Method != http.MethodPost {
				t.Errorf("request method = %q, want %q", req.Method, http.MethodPost)
			}

			if req.URL.String() != SERVER_URL+"/"+CHAT_ENDPOINT {
				t.Errorf("request URL = %q, want %q", req.URL, SERVER_URL+"/"+CHAT_ENDPOINT)
			}
			if got := req.Header.Get("Content-Type"); got != "application/json" {
				t.Errorf("Content-Type = %q, want %q", got, "application/json")
			}
			if got := req.Header.Get("Authorization"); got != "" {
				t.Errorf("Authorization = %q, want no authorization header for local model", got)
			}

			var request struct {
				Model    string `json:"model"`
				Messages []struct {
					Role    string `json:"role"`
					Content string `json:"content"`
				} `json:"messages"`
			}
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Errorf("decode request body: %v", err)
			} else {
				if request.Model == "" {
					t.Error("request model is empty")
				}
				if len(request.Messages) != 2 {
					t.Errorf("len(messages) = %d, want 2", len(request.Messages))
				} else {
					if request.Messages[0].Role != "system" {
						t.Errorf("first message role = %q, want %q", request.Messages[0].Role, "system")
					}
					if request.Messages[1].Role != "user" || request.Messages[1].Content != userMessage {
						t.Errorf("user message = %#v, want role %q and content %q", request.Messages[1], "user", userMessage)
					}
				}
			}

			expectedResponse := ModelResponse{
				ID:    "test-response",
				Model: defaultModelName,
				Usage: ModelUsage{PromptTokens: 10, CompletionTokens: 7, TotalTokens: 17},
				Choices: []ModelChoice{{Index: 0, Message: ModelMessage{
					Role: "assistant", Content: modelResponse,
				}, FinishReason: "stop"}},
			}
			body, err := json.Marshal(expectedResponse)
			if err != nil {
				t.Fatalf("marshal mock response: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Request:    req,
			}, nil
		}),
	}

	got, err := agent.SendChatRequest(userMessage)
	if err != nil {
		t.Fatalf("SendChatRequest() error = %v", err)
	}
	if len(got.Choices) != 1 {
		t.Fatalf("len(choices) = %d, want 1", len(got.Choices))
	}
	if got.Choices[0].Message.Content != modelResponse {
		t.Errorf("response content = %q, want %q", got.Choices[0].Message.Content, modelResponse)
	}
	if got.ID != "test-response" {
		t.Errorf("response ID = %q, want %q", got.ID, "test-response")
	}
	if got.Usage != (ModelUsage{PromptTokens: 10, CompletionTokens: 7, TotalTokens: 17}) {
		t.Errorf("response usage = %#v, want token counts 10/7/17", got.Usage)
	}
}

func TestNewAgentUsesConfiguredModelServerURL(t *testing.T) {
	const configuredURL = "http://model-server:9931/v1/"
	t.Setenv(serverURLEnv, configuredURL)
	t.Setenv(modelNameEnv, "test-model")
	t.Setenv(openAIKeyEnv, "")
	t.Setenv(legacyOpenAIKeyEnv, "")

	agent := NewAgent()
	agent.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got, want := req.URL.String(), configuredURL+"chat/completions"; got != want {
				t.Errorf("request URL = %q, want %q", got, want)
			}
			var request chatRequest
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Errorf("decode request body: %v", err)
			} else if request.Model != "test-model" {
				t.Errorf("request model = %q, want %q", request.Model, "test-model")
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
				Request:    req,
			}, nil
		}),
	}

	if _, err := agent.SendChatRequest("test"); err != nil {
		t.Fatalf("SendChatRequest() error = %v", err)
	}
}

func TestNewAgentDefaultsModelServerURL(t *testing.T) {
	t.Setenv(serverURLEnv, "")
	t.Setenv(openAIKeyEnv, "")
	t.Setenv(legacyOpenAIKeyEnv, "")
	if got := NewAgent().serverURL; got != SERVER_URL {
		t.Errorf("server URL = %q, want default %q", got, SERVER_URL)
	}
}

func TestNewAgentDefaultsModelName(t *testing.T) {
	t.Setenv(modelNameEnv, "")
	t.Setenv(openAIKeyEnv, "")
	t.Setenv(legacyOpenAIKeyEnv, "")
	if got := NewAgent().model; got != defaultModelName {
		t.Errorf("model = %q, want default %q", got, defaultModelName)
	}
}

func TestNewAgentUsesOpenAIWhenAPIKeyIsSet(t *testing.T) {
	t.Setenv(openAIKeyEnv, "test-openai-key")
	t.Setenv(legacyOpenAIKeyEnv, "")
	t.Setenv(serverURLEnv, SERVER_URL)
	t.Setenv(modelNameEnv, defaultModelName)
	t.Setenv(openAIModelEnv, "")

	agent := NewAgent()
	agent.httpClient = &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if got, want := req.URL.String(), OPENAI_SERVER_URL+"/"+CHAT_ENDPOINT; got != want {
				t.Errorf("request URL = %q, want %q", got, want)
			}
			if got, want := req.Header.Get("Authorization"), "Bearer test-openai-key"; got != want {
				t.Errorf("Authorization = %q, want %q", got, want)
			}
			var request chatRequest
			if err := json.NewDecoder(req.Body).Decode(&request); err != nil {
				t.Errorf("decode request body: %v", err)
			} else if got, want := request.Model, defaultOpenAIModel; got != want {
				t.Errorf("request model = %q, want %q", got, want)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"ok"}}]}`)),
				Request:    req,
			}, nil
		}),
	}

	if _, err := agent.SendChatRequest("test"); err != nil {
		t.Fatalf("SendChatRequest() error = %v", err)
	}
}

func TestNewAgentUsesLegacyOpenAIKey(t *testing.T) {
	t.Setenv(openAIKeyEnv, "")
	t.Setenv(legacyOpenAIKeyEnv, "legacy-test-key")
	agent := NewAgent()
	if got, want := agent.serverURL, OPENAI_SERVER_URL; got != want {
		t.Errorf("server URL = %q, want %q", got, want)
	}
	if got, want := agent.apiKey, "legacy-test-key"; got != want {
		t.Errorf("API key = %q, want %q", got, want)
	}
}

func TestNewAgentUsesConfiguredOpenAIModel(t *testing.T) {
	t.Setenv(openAIKeyEnv, "test-openai-key")
	t.Setenv(legacyOpenAIKeyEnv, "")
	t.Setenv(openAIModelEnv, "gpt-test-model")

	if got, want := NewAgent().model, "gpt-test-model"; got != want {
		t.Errorf("model = %q, want %q", got, want)
	}
}

func TestNewAgentConfiguresHTTPTimeout(t *testing.T) {
	if got, want := NewAgent().httpClient.Timeout, 5*time.Minute; got != want {
		t.Errorf("HTTP client timeout = %s, want %s", got, want)
	}
}

func TestSendChatRequestRejectsNilAgent(t *testing.T) {
	var agent *Agent
	if _, err := agent.SendChatRequest("test"); err == nil {
		t.Fatal("SendChatRequest() error = nil, want an error for a nil agent")
	}
}

func TestSendChatRequestRejectsNilHTTPClient(t *testing.T) {
	agent := &Agent{}
	if _, err := agent.SendChatRequest("test"); err == nil {
		t.Fatal("SendChatRequest() error = nil, want an error for a nil HTTP client")
	}
}

func TestSendChatRequestRejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		body       string
	}{
		{
			name:       "non-200 status",
			statusCode: http.StatusInternalServerError,
			body:       `{"error":"unavailable"}`,
		},
		{
			name:       "invalid JSON",
			statusCode: http.StatusOK,
			body:       `not-json`,
		},
		{
			name:       "empty choices",
			statusCode: http.StatusOK,
			body:       `{"choices":[]}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := NewAgent()
			agent.httpClient = &http.Client{
				Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: tt.statusCode,
						Header:     make(http.Header),
						Body:       io.NopCloser(strings.NewReader(tt.body)),
						Request:    req,
					}, nil
				}),
			}

			if _, err := agent.SendChatRequest("test"); err == nil {
				t.Fatal("SendChatRequest() error = nil, want an error")
			}
		})
	}
}

func TestClaudeProviderImplementsSendRequest(t *testing.T) {
	t.Setenv(anthropicKeyEnv, "test-key")
	claude, err := NewAgentFor(ProviderClaude)
	if err != nil {
		t.Fatalf("NewAgentFor(claude) error = %v", err)
	}
	claude.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != anthropicURL+"/messages" {
			t.Errorf("request URL = %q, want %q", req.URL, anthropicURL+"/messages")
		}
		if got := req.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key = %q, want test key", got)
		}
		if got := req.Header.Get("anthropic-version"); got != anthropicVersion {
			t.Errorf("anthropic-version = %q, want %q", got, anthropicVersion)
		}
		var body struct {
			MaxTokens int            `json:"max_tokens"`
			System    string         `json:"system"`
			Messages  []ModelMessage `json:"messages"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		if body.System != "Be concise." || body.MaxTokens <= 0 || len(body.Messages) != 1 {
			t.Errorf("request body = %#v, want system prompt and one user message", body)
		}
		response := `{"id":"claude-1","model":"claude-test","content":[{"type":"text","text":"hello"}],"usage":{"input_tokens":2,"output_tokens":3}}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(response)),
			Request:    req,
		}, nil
	})}
	response, err := claude.SendRequest(context.Background(), []ModelMessage{
		{Role: "system", Content: "Be concise."},
		{Role: "user", Content: "hi"},
	})
	if err != nil {
		t.Fatalf("SendRequest() error = %v", err)
	}
	if len(response.Choices) != 1 || response.Choices[0].Message.Content != "hello" {
		t.Fatalf("response = %#v, want assistant text hello", response)
	}
	if response.Usage.TotalTokens != 5 {
		t.Errorf("total tokens = %d, want 5", response.Usage.TotalTokens)
	}
}

func TestNewAgentForRejectsUnknownProvider(t *testing.T) {
	if _, err := NewAgentFor(Provider("unknown")); err == nil {
		t.Fatal("NewAgentFor(unknown) error = nil, want unsupported-provider error")
	}
}
