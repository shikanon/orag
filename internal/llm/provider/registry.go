package provider

import (
	"fmt"
	"sort"
	"strings"
)

type Name string

const (
	OpenAI         Name = "openai"
	AzureOpenAI    Name = "azure-openai"
	Anthropic      Name = "anthropic"
	Gemini         Name = "gemini"
	GoogleCloud    Name = "google-cloud"
	XAI            Name = "xai"
	Mistral        Name = "mistral"
	Cohere         Name = "cohere"
	DeepSeek       Name = "deepseek"
	Moonshot       Name = "moonshot"
	MiniMax        Name = "minimax"
	BaiChuan       Name = "baichuan"
	ZhipuAI        Name = "zhipu-ai"
	TongyiQianwen  Name = "tongyi-qianwen"
	VolcEngine     Name = "volcengine"
	TencentHunyuan Name = "tencent-hunyuan"
	XunFeiSpark    Name = "xunfei-spark"
	BaiduYiyan     Name = "baiduyiyan"
	Xiaomi         Name = "xiaomi"
	Perplexity     Name = "perplexity"
	VoyageAI       Name = "voyage-ai"
	Jina           Name = "jina"
	Mock           Name = "mock"
)

type Capability string

const (
	CapabilityChat        Capability = "chat"
	CapabilityEmbedding   Capability = "embedding"
	CapabilityRerank      Capability = "rerank"
	CapabilityImage2Text  Capability = "image2text"
	CapabilitySpeech2Text Capability = "speech2text"
	CapabilityTTS         Capability = "tts"
	CapabilityModeration  Capability = "moderation"
)

type Protocol string

const (
	ProtocolOpenAICompatible Protocol = "openai-compatible"
	ProtocolAzureOpenAI      Protocol = "azure-openai"
	ProtocolAnthropic        Protocol = "anthropic"
	ProtocolGemini           Protocol = "gemini"
	ProtocolGoogleCloud      Protocol = "google-cloud"
	ProtocolCohere           Protocol = "cohere"
	ProtocolJina             Protocol = "jina"
	ProtocolVoyage           Protocol = "voyage"
	ProtocolMock             Protocol = "mock"
)

type Info struct {
	Name          Name
	DisplayName   string
	Protocol      Protocol
	Capabilities  []Capability
	DefaultModels map[Capability]string
	RequiredEnv   []string
}

func (i Info) Supports(capability Capability) bool {
	for _, item := range i.Capabilities {
		if item == capability {
			return true
		}
	}
	return false
}

type Registry struct {
	providers map[Name]Info
	aliases   map[Name]Name
}

func BuiltinRegistry() Registry {
	registry := Registry{
		providers: map[Name]Info{},
		aliases: map[Name]Name{
			"azure":       AzureOpenAI,
			"azureopenai": AzureOpenAI,
			"ark":         VolcEngine,
			"volc":        VolcEngine,
			"volc-engine": VolcEngine,
			"zhipu":       ZhipuAI,
			"zhipuai":     ZhipuAI,
			"qwen":        TongyiQianwen,
			"dashscope":   TongyiQianwen,
			"tongyi":      TongyiQianwen,
			"aliyun":      TongyiQianwen,
			"hunyuan":     TencentHunyuan,
			"xunfei":      XunFeiSpark,
			"spark":       XunFeiSpark,
			"baichuan":    BaiChuan,
			"baidu":       BaiduYiyan,
			"voyage":      VoyageAI,
		},
	}
	for _, info := range []Info{
		provider(OpenAI, "OpenAI", ProtocolOpenAICompatible, env("OPENAI_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityRerank, CapabilityImage2Text, CapabilitySpeech2Text, CapabilityTTS, CapabilityModeration), models(CapabilityChat, "gpt-4o-mini", CapabilityEmbedding, "text-embedding-3-large", CapabilityImage2Text, "gpt-4o-mini")),
		provider(AzureOpenAI, "Azure OpenAI", ProtocolAzureOpenAI, env("AZURE_OPENAI_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityImage2Text, CapabilitySpeech2Text, CapabilityModeration), models(CapabilityChat, "gpt-4o-mini", CapabilityEmbedding, "text-embedding-3-large")),
		provider(Anthropic, "Anthropic", ProtocolAnthropic, env("ANTHROPIC_API_KEY"), caps(CapabilityChat, CapabilityImage2Text), models(CapabilityChat, "claude-sonnet-4-20250514")),
		provider(Gemini, "Gemini", ProtocolGemini, env("GEMINI_API_KEY", "GOOGLE_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityImage2Text), models(CapabilityChat, "gemini-2.5-flash", CapabilityEmbedding, "gemini-embedding-001")),
		provider(GoogleCloud, "Google Cloud", ProtocolGoogleCloud, env("GOOGLE_CLOUD_API_KEY", "GOOGLE_APPLICATION_CREDENTIALS"), caps(CapabilityChat, CapabilityEmbedding, CapabilityImage2Text), models(CapabilityChat, "gemini-2.5-flash")),
		provider(XAI, "xAI", ProtocolOpenAICompatible, env("XAI_API_KEY"), caps(CapabilityChat, CapabilityImage2Text), models(CapabilityChat, "grok-3-mini")),
		provider(Mistral, "Mistral", ProtocolOpenAICompatible, env("MISTRAL_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityImage2Text, CapabilityModeration), models(CapabilityChat, "mistral-large-latest", CapabilityEmbedding, "mistral-embed")),
		provider(Cohere, "Cohere", ProtocolCohere, env("COHERE_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityRerank, CapabilitySpeech2Text), models(CapabilityChat, "command-r-plus", CapabilityEmbedding, "embed-v4.0", CapabilityRerank, "rerank-v3.5")),
		provider(DeepSeek, "DeepSeek", ProtocolOpenAICompatible, env("DEEPSEEK_API_KEY"), caps(CapabilityChat), models(CapabilityChat, "deepseek-chat")),
		provider(Moonshot, "Moonshot", ProtocolOpenAICompatible, env("MOONSHOT_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityImage2Text), models(CapabilityChat, "kimi-k2-0711-preview")),
		provider(MiniMax, "MiniMax", ProtocolOpenAICompatible, env("MINIMAX_API_KEY"), caps(CapabilityChat, CapabilityTTS), models(CapabilityChat, "MiniMax-M2.5")),
		provider(BaiChuan, "BaiChuan", ProtocolOpenAICompatible, env("BAICHUAN_API_KEY"), caps(CapabilityChat, CapabilityEmbedding), models(CapabilityChat, "Baichuan4")),
		provider(ZhipuAI, "ZHIPU-AI", ProtocolOpenAICompatible, env("ZHIPU_API_KEY", "ZHIPUAI_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityImage2Text, CapabilitySpeech2Text, CapabilityModeration), models(CapabilityChat, "glm-4.5", CapabilityEmbedding, "embedding-3")),
		provider(TongyiQianwen, "Tongyi-Qianwen", ProtocolOpenAICompatible, env("DASHSCOPE_API_KEY", "TONGYI_QIANWEN_API_KEY", "ALIYUN_RERANK_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityRerank, CapabilityImage2Text, CapabilitySpeech2Text, CapabilityTTS, CapabilityModeration), models(CapabilityChat, "qwen-plus", CapabilityEmbedding, "text-embedding-v4", CapabilityRerank, "gte-rerank-v2")),
		provider(VolcEngine, "VolcEngine", ProtocolOpenAICompatible, env("ARK_API_KEY", "VOLCENGINE_API_KEY", "LLM_API_KEY"), caps(CapabilityChat, CapabilityEmbedding, CapabilityRerank, CapabilityImage2Text), models(CapabilityChat, "doubao-seed-2-1-pro-260628", CapabilityEmbedding, "doubao-embedding-vision-251215", CapabilityRerank, "m3-v2-rerank", CapabilityImage2Text, "doubao-seed-2-1-pro-260628")),
		provider(TencentHunyuan, "Tencent Hunyuan", ProtocolOpenAICompatible, env("TENCENT_HUNYUAN_API_KEY", "HUNYUAN_API_KEY"), caps(CapabilityChat, CapabilityImage2Text), models(CapabilityChat, "hunyuan-standard")),
		provider(XunFeiSpark, "XunFei Spark", ProtocolOpenAICompatible, env("XUNFEI_SPARK_API_KEY", "SPARK_API_KEY"), caps(CapabilityChat, CapabilityTTS), models(CapabilityChat, "Spark-Max")),
		provider(BaiduYiyan, "BaiduYiyan", ProtocolOpenAICompatible, env("BAIDU_YIYAN_API_KEY", "WENXIN_API_KEY"), caps(CapabilityChat), models(CapabilityChat, "ernie-4.0-turbo")),
		provider(Xiaomi, "Xiaomi", ProtocolOpenAICompatible, env("XIAOMI_API_KEY"), caps(CapabilityChat, CapabilityImage2Text), models(CapabilityChat, "mimo-v2.5")),
		provider(Perplexity, "Perplexity", ProtocolOpenAICompatible, env("PERPLEXITY_API_KEY"), caps(CapabilityEmbedding), models(CapabilityEmbedding, "pplx-embed-v1-0.6b")),
		provider(VoyageAI, "Voyage AI", ProtocolVoyage, env("VOYAGE_API_KEY"), caps(CapabilityEmbedding, CapabilityRerank), models(CapabilityEmbedding, "voyage-3.5", CapabilityRerank, "rerank-2")),
		provider(Jina, "Jina", ProtocolJina, env("JINA_API_KEY"), caps(CapabilityEmbedding, CapabilityRerank), models(CapabilityEmbedding, "jina-embeddings-v3", CapabilityRerank, "jina-reranker-v2-base-multilingual")),
		provider(Mock, "Deterministic Mock", ProtocolMock, env("ALLOW_DETERMINISTIC_MOCK"), caps(CapabilityChat, CapabilityEmbedding, CapabilityRerank, CapabilityImage2Text), models(CapabilityChat, "mock", CapabilityEmbedding, "mock", CapabilityRerank, "mock", CapabilityImage2Text, "mock")),
	} {
		registry.providers[info.Name] = info
	}
	return registry
}

func (r Registry) Get(name Name) (Info, bool) {
	normalized := NormalizeName(string(name))
	if canonical, ok := r.aliases[normalized]; ok {
		normalized = canonical
	}
	info, ok := r.providers[normalized]
	return info, ok
}

func (r Registry) MustGet(name Name) Info {
	info, ok := r.Get(name)
	if !ok {
		panic(fmt.Sprintf("unknown model provider %q", name))
	}
	return info
}

func (r Registry) Names() []Name {
	names := make([]Name, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
	return names
}

func NormalizeName(name string) Name {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, "_", "-")
	name = strings.ReplaceAll(name, " ", "-")
	return Name(name)
}

func provider(name Name, displayName string, protocol Protocol, requiredEnv []string, capabilities []Capability, defaultModels map[Capability]string) Info {
	return Info{
		Name:          name,
		DisplayName:   displayName,
		Protocol:      protocol,
		Capabilities:  capabilities,
		DefaultModels: defaultModels,
		RequiredEnv:   requiredEnv,
	}
}

func env(keys ...string) []string {
	return keys
}

func caps(capabilities ...Capability) []Capability {
	return capabilities
}

func models(items ...any) map[Capability]string {
	out := map[Capability]string{}
	for i := 0; i+1 < len(items); i += 2 {
		capability, ok := items[i].(Capability)
		if !ok {
			continue
		}
		model, ok := items[i+1].(string)
		if !ok || strings.TrimSpace(model) == "" {
			continue
		}
		out[capability] = model
	}
	return out
}
