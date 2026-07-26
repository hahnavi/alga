package setup

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"alga-agent/internal/config"
)

func TestFetchModels_ParsesAndFiltersTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("Authorization = %q", got)
		}
		w.Write([]byte(`{"data":[
			{"id":"b/tool-model","supported_parameters":["tools","temperature"]},
			{"id":"a/tool-model","supported_parameters":["tools"]},
			{"id":"c/chat-only","supported_parameters":["temperature"]},
			{"id":"d/no-params-field"},
			{"id":""}
		]}`))
	}))
	defer srv.Close()

	ids, err := fetchModels(srv.URL, "sk-test")
	if err != nil {
		t.Fatalf("fetchModels: %v", err)
	}
	want := []string{"a/tool-model", "b/tool-model", "d/no-params-field"}
	if !slices.Equal(ids, want) {
		t.Errorf("ids = %v, want %v", ids, want)
	}
}

func TestFetchModels_ErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	if _, err := fetchModels(srv.URL, ""); err == nil {
		t.Fatal("expected error for 401 response")
	}
}

func TestMergeModels(t *testing.T) {
	curated := []string{"openrouter/free", "openai/gpt-4o"}
	live := []string{"openai/gpt-4o", "z/live-model", "a/live-model"}

	got := mergeModels(curated, live, 10)
	want := []string{"openrouter/free", "openai/gpt-4o", "z/live-model", "a/live-model"}
	if !slices.Equal(got, want) {
		t.Errorf("merged = %v, want %v", got, want)
	}

	if got := mergeModels(curated, live, 3); len(got) != 3 {
		t.Errorf("cap ignored: %v", got)
	}
	if got := mergeModels(nil, nil, 5); len(got) != 0 {
		t.Errorf("expected empty merge, got %v", got)
	}
}

func TestCuratedListDefaultFirst(t *testing.T) {
	if curatedOpenRouterModels[0] != "openrouter/free" {
		t.Errorf("curated[0] = %q, want openrouter/free", curatedOpenRouterModels[0])
	}
}

func TestProviderChoicesHavePresets(t *testing.T) {
	if providerChoices[0] != "openrouter" {
		t.Errorf("providerChoices[0] = %q, want openrouter", providerChoices[0])
	}
	if last := providerChoices[len(providerChoices)-1]; last != "custom" {
		t.Errorf("last provider choice = %q, want custom", last)
	}
	for _, id := range providerChoices {
		if id == "custom" {
			continue
		}
		p, ok := providerPresets[id]
		if !ok {
			t.Errorf("provider %q has no preset", id)
			continue
		}
		if p.keyURL == "" {
			t.Errorf("provider %q preset missing keyURL", id)
		}
		if got := config.BaseURLForProvider(id); got == "" || (id != "openrouter" && got == config.BaseURLForProvider("openrouter")) {
			t.Errorf("provider %q has no canonical base URL (got %q)", id, got)
		}
	}
	for id := range providerPresets {
		if !slices.Contains(providerChoices, id) {
			t.Errorf("preset %q not offered in providerChoices", id)
		}
	}
}

func TestDefaultModelForProvider(t *testing.T) {
	if got := defaultModelForProvider("openrouter"); got != "openrouter/free" {
		t.Errorf("openrouter default = %q, want openrouter/free", got)
	}
	if got := defaultModelForProvider("zai"); got != providerPresets["zai"].models[0] {
		t.Errorf("zai default = %q, want first curated model", got)
	}
	if got := defaultModelForProvider("custom"); got != "openrouter/free" {
		t.Errorf("custom default = %q, want openrouter/free fallback", got)
	}
}
