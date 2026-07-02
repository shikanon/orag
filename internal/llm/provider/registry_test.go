package provider

import "testing"

func TestBuiltinRegistryContainsIssueProviders(t *testing.T) {
	registry := BuiltinRegistry()
	required := []Name{
		OpenAI,
		AzureOpenAI,
		Anthropic,
		Gemini,
		GoogleCloud,
		XAI,
		Mistral,
		Cohere,
		DeepSeek,
		Moonshot,
		MiniMax,
		BaiChuan,
		ZhipuAI,
		TongyiQianwen,
		VolcEngine,
		TencentHunyuan,
		XunFeiSpark,
		BaiduYiyan,
		Xiaomi,
		Perplexity,
		VoyageAI,
		Jina,
	}
	for _, name := range required {
		info, ok := registry.Get(name)
		if !ok {
			t.Fatalf("provider %q is missing", name)
		}
		if len(info.RequiredEnv) == 0 {
			t.Fatalf("provider %q should declare required env vars", name)
		}
	}
}

func TestBuiltinRegistryCapturesProviderCapabilities(t *testing.T) {
	registry := BuiltinRegistry()

	volcengine := registry.MustGet(VolcEngine)
	for _, capability := range []Capability{CapabilityChat, CapabilityEmbedding, CapabilityImage2Text, CapabilityRerank} {
		if !volcengine.Supports(capability) {
			t.Fatalf("VolcEngine should support %s", capability)
		}
	}
	if volcengine.DefaultModels[CapabilityChat] != "doubao-seed-2-1-pro-260628" {
		t.Fatalf("VolcEngine default chat model = %q", volcengine.DefaultModels[CapabilityChat])
	}
	if volcengine.DefaultModels[CapabilityEmbedding] != "doubao-embedding-vision-251215" {
		t.Fatalf("VolcEngine default embedding model = %q", volcengine.DefaultModels[CapabilityEmbedding])
	}

	jina := registry.MustGet(Jina)
	if !jina.Supports(CapabilityEmbedding) || !jina.Supports(CapabilityRerank) {
		t.Fatalf("Jina should support embedding and rerank: %#v", jina.Capabilities)
	}
	if jina.Supports(CapabilityChat) {
		t.Fatal("Jina must not be marked as chat-capable")
	}

	voyage := registry.MustGet(VoyageAI)
	if !voyage.Supports(CapabilityEmbedding) || !voyage.Supports(CapabilityRerank) {
		t.Fatalf("Voyage AI should support embedding and rerank: %#v", voyage.Capabilities)
	}

	perplexity := registry.MustGet(Perplexity)
	if !perplexity.Supports(CapabilityEmbedding) {
		t.Fatal("Perplexity should be registered for embedding capability")
	}
	if perplexity.Supports(CapabilityChat) {
		t.Fatal("Perplexity should not be marked as chat-capable until the adapter supports it")
	}
}
