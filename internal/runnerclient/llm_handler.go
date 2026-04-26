package runnerclient

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"xbot/internal/runnerproto"
	"xbot/llm"
)

// LLMProxyRequest mirrors the server-side LLM proxy request.
type LLMProxyRequest struct {
	Model        string            `json:"model"`
	Messages     []llm.ChatMessage `json:"messages"`
	Tools        []llm.ToolDefJSON `json:"tools,omitempty"`
	ThinkingMode string            `json:"thinking_mode,omitempty"`
}

// handleLLMGenerate handles the "llm_generate" request.
func handleLLMGenerate(msg runnerproto.RunnerMessage, llmClient llm.LLM, logf LogFunc) *runnerproto.RunnerMessage {
	if llmClient == nil {
		return runnerproto.MakeError(msg.ID, "ENOTSUP", "local LLM not configured on this runner")
	}

	var req LLMProxyRequest
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return runnerproto.MakeError(msg.ID, "EINVAL", "invalid llm_generate request: "+err.Error())
	}

	// Convert ToolDefJSON back to ChatMessage format for LLM call
	var tools []llm.ToolDefinition
	if len(req.Tools) > 0 {
		tools = make([]llm.ToolDefinition, len(req.Tools))
		for i, t := range req.Tools {
			tools[i] = &toolDefAdapter{name: t.Name, desc: t.Description, params: t.Parameters}
		}
	}

	// Use generous timeout — LLM calls can be slow
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	resp, err := llmClient.Generate(ctx, req.Model, req.Messages, tools, req.ThinkingMode)
	if err != nil {
		return runnerproto.MakeError(msg.ID, "EIO", "LLM generate failed: "+err.Error())
	}

	return runnerproto.MakeResponse(msg.ID, "llm_response", resp)
}

// handleLLMModels handles the "llm_models" request.
func handleLLMModels(msg runnerproto.RunnerMessage, llmClient llm.LLM, llmModels []string, logf LogFunc) *runnerproto.RunnerMessage {
	if llmClient == nil {
		return runnerproto.MakeError(msg.ID, "ENOTSUP", "local LLM not configured on this runner")
	}

	return runnerproto.MakeResponse(msg.ID, "llm_models_response", llm.LLMListModelsResponse{
		Models: llmModels,
	})
}

// toolDefAdapter adapts ToolDefJSON to the llm.ToolDefinition interface.
type toolDefAdapter struct {
	name   string
	desc   string
	params []llm.ToolParam
}

func (t *toolDefAdapter) Name() string                { return t.name }
func (t *toolDefAdapter) Description() string         { return t.desc }
func (t *toolDefAdapter) Parameters() []llm.ToolParam { return t.params }

// InitLLMClient initializes the LLM client from provider/baseURL/apiKey/model.
// Empty provider means sandbox-only mode, no LLM configured.
func InitLLMClient(provider, baseURL, apiKey, model string, logf LogFunc) (llm.LLM, []string, error) {
	if provider == "" || apiKey == "" {
		callLogf(logf, "  Local LLM: not configured (pure sandbox mode)")
		return nil, nil, nil
	}

	var client llm.LLM
	var models []string

	switch provider {
	case "openai":
		cfg := llm.OpenAIConfig{
			APIKey:       apiKey,
			BaseURL:      baseURL,
			DefaultModel: model,
		}
		client = llm.NewOpenAILLM(cfg)
		models = client.ListModels()

	case "anthropic":
		cfg := llm.AnthropicConfig{
			APIKey:       apiKey,
			BaseURL:      baseURL,
			DefaultModel: model,
		}
		client = llm.NewAnthropicLLM(cfg)
		models = client.ListModels()

	default:
		return nil, nil, fmt.Errorf("unsupported LLM provider: %s", provider)
	}

	callLogf(logf, "  Local LLM: configured provider=%s model=%s (%d models available)",
		provider, model, len(models))
	return client, models, nil
}
