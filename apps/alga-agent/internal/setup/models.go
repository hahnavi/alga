// Model list helpers for the setup wizard: a curated OpenRouter list merged
// with a live fetch of the provider's /models endpoint (hermes convention).
package setup

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"alga-agent/internal/version"
)

// curatedOpenRouterModels is the offline fallback shown when the live fetch
// fails. The first entry is the wizard default. Mirrors the hermes-agent
// curated list, filtered to tool-capable models verified on OpenRouter.
var curatedOpenRouterModels = []string{
	"openrouter/free",
	"z-ai/glm-5.2",
	"moonshotai/kimi-k3",
	"anthropic/claude-sonnet-5",
	"anthropic/claude-opus-5",
	"anthropic/claude-haiku-4.5",
	"openai/gpt-5.6-sol",
	"openai/gpt-5.5",
	"openai/gpt-5.4-mini",
	"google/gemini-3.1-pro-preview",
	"google/gemini-3.5-flash",
	"x-ai/grok-4.5",
	"deepseek/deepseek-v4-pro",
	"deepseek/deepseek-v4-flash",
	"qwen/qwen3.7-max",
	"minimax/minimax-m3",
}

// maxModelChoices caps the merged list so the picker stays scannable.
const maxModelChoices = 30

// providerChoices is the wizard's provider menu order; the first entry is the
// default for fresh configs and "custom" is always last.
var providerChoices = []string{
	"openrouter",
	"openai",
	"opencode-zen",
	"opencode-go",
	"zai",
	"zai-coding-plan",
	"alibaba",
	"alibaba-coding-plan",
	"custom",
}

// providerPreset carries the wizard hints for a known provider: where to get
// an API key and a curated model list (first entry = default). Base URLs live
// in config.BaseURLForProvider. Mirrors the hermes-agent provider registry.
type providerPreset struct {
	keyURL string
	models []string
}

var providerPresets = map[string]providerPreset{
	"openrouter": {
		keyURL: "https://openrouter.ai/keys",
		models: curatedOpenRouterModels,
	},
	"openai": {
		// No curated list: the OpenAI /models endpoint is fetched live.
		keyURL: "https://platform.openai.com/api-keys",
	},
	"opencode-zen": {
		keyURL: "https://opencode.ai/auth",
		models: []string{
			"kimi-k2.5", "kimi-k2.6", "gpt-5.5", "gpt-5.5-pro", "gpt-5.4-pro",
			"gpt-5.4", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex",
			"gpt-5.3-codex-spark", "gpt-5.2", "gpt-5.2-codex",
		},
	},
	"opencode-go": {
		keyURL: "https://opencode.ai/auth",
		models: []string{
			"kimi-k3", "kimi-k2.7-code", "kimi-k2.6", "kimi-k2.5",
			"glm-5.2", "glm-5.1", "glm-5", "mimo-v2.5-pro", "mimo-v2.5",
			"mimo-v2-pro", "mimo-v2-omni", "minimax-m3",
		},
	},
	"zai": {
		keyURL: "https://z.ai/",
		models: []string{
			"glm-5.2", "glm-5.1", "glm-5", "glm-5v-turbo", "glm-5-turbo",
			"glm-4.7", "glm-4.5", "glm-4.5-flash",
		},
	},
	"zai-coding-plan": {
		keyURL: "https://z.ai/",
		models: []string{"glm-5.2", "glm-5.1", "glm-5v-turbo", "glm-4.7"},
	},
	"alibaba": {
		keyURL: "https://modelstudio.console.alibabacloud.com/",
		models: []string{
			"qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus", "kimi-k2.5",
			"qwen3.5-plus", "qwen3-coder-plus", "qwen3-coder-next",
			"glm-5", "glm-4.7", "MiniMax-M2.5",
		},
	},
	"alibaba-coding-plan": {
		keyURL: "https://modelstudio.console.alibabacloud.com/",
		models: []string{
			"qwen3.7-plus", "qwen3.6-plus", "qwen3.5-plus",
			"qwen3-max-2026-01-23", "qwen3-coder-plus", "qwen3-coder-next",
			"kimi-k2.5", "glm-5", "glm-4.7", "MiniMax-M2.5",
		},
	},
}

// defaultModelForProvider returns the first curated model for a provider,
// falling back to the global wizard default.
func defaultModelForProvider(provider string) string {
	if p, ok := providerPresets[provider]; ok && len(p.models) > 0 {
		return p.models[0]
	}
	return "openrouter/free"
}

// fetchModels retrieves model ids from an OpenAI-compatible {base_url}/models
// endpoint. When the response advertises supported_parameters (OpenRouter),
// models without tool support are dropped — the agent requires tool calling.
// A function var so tests can stub the network.
var fetchModels = func(baseURL, apiKey string) ([]string, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	// Identify explicitly: some providers (e.g. OpenCode Zen) sit behind a WAF
	// that rejects Go's default User-Agent.
	req.Header.Set("User-Agent", version.UserAgent())
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}

	var body struct {
		Data []struct {
			ID                  string   `json:"id"`
			SupportedParameters []string `json:"supported_parameters"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode %s: %w", url, err)
	}

	var ids []string
	for _, m := range body.Data {
		if m.ID == "" {
			continue
		}
		if m.SupportedParameters != nil && !slices.Contains(m.SupportedParameters, "tools") {
			continue
		}
		ids = append(ids, m.ID)
	}
	slices.Sort(ids)
	return ids, nil
}

// mergeModels combines curated (kept first, in order) with live ids, dropping
// duplicates and capping the result at limit.
func mergeModels(curated, live []string, limit int) []string {
	seen := make(map[string]bool, len(curated)+len(live))
	out := make([]string, 0, limit)
	for _, lst := range [][]string{curated, live} {
		for _, id := range lst {
			if len(out) >= limit {
				return out
			}
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
