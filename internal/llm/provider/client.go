package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/llm/ark"
)

type Config struct {
	ChatProvider       Name
	EmbeddingProvider  Name
	RerankProvider     Name
	MultimodalProvider Name

	APIKeys  map[Name]string
	BaseURLs map[Name]string

	ChatModel           string
	EmbeddingModel      string
	EmbeddingDimensions int
	RerankModel         string
	RerankInstruct      string
	MultimodalModel     string

	AllowDeterministicMock bool
	Timeout                time.Duration
	RetryTimes             int
}

const defaultAzureAPIVersion = "2024-02-15-preview"

type Client struct {
	chat       ark.ChatGenerator
	stream     chatStreamer
	embedding  ark.Embedder
	rerank     ark.Reranker
	multimodal ark.MultimodalParser
}

type chatStreamer interface {
	ChatStream(context.Context, []ark.ChatMessage) (<-chan ark.StreamChunk, <-chan error)
}

func NewClient(cfg Config, httpClient *http.Client) (*Client, error) {
	if cfg.EmbeddingDimensions <= 0 {
		cfg.EmbeddingDimensions = 1024
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	chat, stream, err := buildChatAdapter(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	embedding, err := buildEmbeddingAdapter(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	rerank, err := buildRerankAdapter(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	multimodal, err := buildMultimodalAdapter(cfg, httpClient)
	if err != nil {
		return nil, err
	}
	client := &Client{
		chat:       chat,
		stream:     stream,
		embedding:  embedding,
		rerank:     rerank,
		multimodal: multimodal,
	}
	return client, nil
}

func (c *Client) Chat(ctx context.Context, messages []ark.ChatMessage) (string, error) {
	return c.chat.Chat(ctx, messages)
}

func (c *Client) ChatStream(ctx context.Context, messages []ark.ChatMessage) (<-chan ark.StreamChunk, <-chan error) {
	if c.stream != nil {
		return c.stream.ChatStream(ctx, messages)
	}
	chunks := make(chan ark.StreamChunk, 2)
	errs := make(chan error, 1)
	go func() {
		defer close(chunks)
		defer close(errs)
		answer, err := c.Chat(ctx, messages)
		if err != nil {
			errs <- err
			return
		}
		chunks <- ark.StreamChunk{Content: answer}
		chunks <- ark.StreamChunk{Done: true}
		errs <- io.EOF
	}()
	return chunks, errs
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	return c.embedding.Embed(ctx, texts)
}

func (c *Client) Rerank(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
	return c.rerank.Rerank(ctx, query, docs, topN)
}

func (c *Client) MultimodalParse(ctx context.Context, name string, content []byte) (string, error) {
	return c.multimodal.MultimodalParse(ctx, name, content)
}

func buildChatAdapter(cfg Config, httpClient *http.Client) (ark.ChatGenerator, chatStreamer, error) {
	info, apiKey, baseURL, err := resolveAdapter(cfg, CapabilityChat, cfg.ChatProvider)
	if err != nil {
		return nil, nil, err
	}
	if info.Name == Cohere {
		adapter := newCohereAdapter(apiKey, baseURL, cfg, httpClient)
		return adapter, nil, nil
	}
	if info.Name == AzureOpenAI {
		return newAzureOpenAIAdapter(apiKey, baseURL, cfg, httpClient), nil, nil
	}
	if info.Name == Anthropic {
		return newAnthropicAdapter(apiKey, baseURL, cfg, httpClient), nil, nil
	}
	if info.Name == Gemini || info.Name == GoogleCloud {
		return newGeminiAdapter(apiKey, baseURL, cfg, httpClient), nil, nil
	}
	client, err := buildOpenAICompatibleAdapter(cfg, info, apiKey, baseURL, CapabilityChat, cfg.ChatModel, httpClient)
	if err != nil {
		return nil, nil, err
	}
	return client, client, nil
}

func buildEmbeddingAdapter(cfg Config, httpClient *http.Client) (ark.Embedder, error) {
	info, apiKey, baseURL, err := resolveAdapter(cfg, CapabilityEmbedding, cfg.EmbeddingProvider)
	if err != nil {
		return nil, err
	}
	if info.Name == Cohere {
		return newCohereAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	if info.Name == Gemini || info.Name == GoogleCloud {
		return newGeminiAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	if info.Name == AzureOpenAI {
		return newAzureOpenAIAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	if info.Name == VoyageAI {
		return newVoyageAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	return buildOpenAICompatibleAdapter(cfg, info, apiKey, baseURL, CapabilityEmbedding, cfg.EmbeddingModel, httpClient)
}

func buildRerankAdapter(cfg Config, httpClient *http.Client) (ark.Reranker, error) {
	info, apiKey, baseURL, err := resolveAdapter(cfg, CapabilityRerank, cfg.RerankProvider)
	if err != nil {
		return nil, err
	}
	if info.Name == Cohere {
		return newCohereAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	if info.Name == VoyageAI {
		return newVoyageAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	return buildOpenAICompatibleAdapter(cfg, info, apiKey, baseURL, CapabilityRerank, cfg.RerankModel, httpClient)
}

func buildMultimodalAdapter(cfg Config, httpClient *http.Client) (ark.MultimodalParser, error) {
	info, apiKey, baseURL, err := resolveAdapter(cfg, CapabilityImage2Text, cfg.MultimodalProvider)
	if err != nil {
		return nil, err
	}
	if info.Name == AzureOpenAI {
		return newAzureOpenAIAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	if info.Name == Anthropic {
		return newAnthropicAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	if info.Name == Gemini || info.Name == GoogleCloud {
		return newGeminiAdapter(apiKey, baseURL, cfg, httpClient), nil
	}
	return buildOpenAICompatibleAdapter(cfg, info, apiKey, baseURL, CapabilityImage2Text, cfg.MultimodalModel, httpClient)
}

func resolveAdapter(cfg Config, capability Capability, providerName Name) (Info, string, string, error) {
	registry := BuiltinRegistry()
	if providerName == "" {
		providerName = VolcEngine
	}
	info, ok := registry.Get(providerName)
	if !ok {
		return Info{}, "", "", fmt.Errorf("model provider %q is not supported", providerName)
	}
	if !info.Supports(capability) {
		return Info{}, "", "", fmt.Errorf("model provider %q does not support %s", info.Name, capability)
	}
	if info.Name == Mock {
		if !cfg.AllowDeterministicMock {
			return Info{}, "", "", fmt.Errorf("ALLOW_DETERMINISTIC_MOCK=true is required for mock provider")
		}
		return info, "", "", nil
	}
	apiKey := apiKeyFor(cfg.APIKeys, info.Name)
	if apiKey == "" {
		return Info{}, "", "", fmt.Errorf("missing API key for model provider %q", info.Name)
	}
	baseURL := baseURLFor(cfg.BaseURLs, info.Name)
	if baseURL == "" {
		baseURL = DefaultBaseURL(info.Name)
	}
	if baseURL == "" {
		return Info{}, "", "", fmt.Errorf("missing base URL for model provider %q", info.Name)
	}
	return info, apiKey, baseURL, nil
}

func buildAdapter(cfg Config, capability Capability, providerName Name, model string, httpClient *http.Client) (*ark.Client, error) {
	info, apiKey, baseURL, err := resolveAdapter(cfg, capability, providerName)
	if err != nil {
		return nil, err
	}
	return buildOpenAICompatibleAdapter(cfg, info, apiKey, baseURL, capability, model, httpClient)
}

func buildOpenAICompatibleAdapter(cfg Config, info Info, apiKey string, baseURL string, capability Capability, model string, httpClient *http.Client) (*ark.Client, error) {
	if info.Name == Mock {
		return ark.NewClient(ark.Config{
			EmbeddingDimensions: cfg.EmbeddingDimensions,
			Timeout:             cfg.Timeout,
			RetryTimes:          cfg.RetryTimes,
		}, httpClient), nil
	}
	if strings.TrimSpace(model) == "" {
		model = info.DefaultModels[capability]
	}
	arkCfg := ark.Config{
		APIKey:              apiKey,
		BaseURL:             baseURL,
		ChatModel:           modelFor(capability, CapabilityChat, model, info),
		EmbeddingModel:      modelFor(capability, CapabilityEmbedding, model, info),
		EmbeddingDimensions: cfg.EmbeddingDimensions,
		RerankProvider:      string(info.Name),
		RerankBaseURL:       baseURL,
		RerankModel:         modelFor(capability, CapabilityRerank, model, info),
		RerankAPIKey:        apiKey,
		RerankInstruct:      cfg.RerankInstruct,
		MultimodalModel:     modelFor(capability, CapabilityImage2Text, model, info),
		Timeout:             cfg.Timeout,
		RetryTimes:          cfg.RetryTimes,
	}
	if capability == CapabilityRerank && info.Name == TongyiQianwen {
		arkCfg.RerankProvider = "aliyun"
	}
	return ark.NewClient(arkCfg, httpClient), nil
}

func modelFor(actual Capability, desired Capability, model string, info Info) string {
	if actual == desired {
		return model
	}
	return info.DefaultModels[desired]
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func openAIContentPart(name string, content []byte) map[string]any {
	dataURL := "data:" + mediaType(name) + ";base64," + base64.StdEncoding.EncodeToString(content)
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".png") || strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") || strings.HasSuffix(lower, ".webp") {
		return map[string]any{"type": "image_url", "image_url": map[string]string{"url": dataURL}}
	}
	return map[string]any{"type": "file_url", "file_url": map[string]string{"url": dataURL}}
}

func mediaType(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".png"):
		return "image/png"
	case strings.HasSuffix(lower, ".jpg"), strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	case strings.HasSuffix(lower, ".pdf"):
		return "application/pdf"
	case strings.HasSuffix(lower, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

type azureOpenAIAdapter struct {
	apiKey               string
	baseURL              string
	chatDeployment       string
	embeddingDeployment  string
	multimodalDeployment string
	httpClient           *http.Client
}

func newAzureOpenAIAdapter(apiKey string, baseURL string, cfg Config, httpClient *http.Client) *azureOpenAIAdapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	info := BuiltinRegistry().MustGet(AzureOpenAI)
	chatDeployment := firstNonEmpty(cfg.ChatModel, info.DefaultModels[CapabilityChat])
	embeddingDeployment := firstNonEmpty(cfg.EmbeddingModel, info.DefaultModels[CapabilityEmbedding])
	multimodalDeployment := firstNonEmpty(cfg.MultimodalModel, chatDeployment)
	return &azureOpenAIAdapter{
		apiKey:               apiKey,
		baseURL:              strings.TrimRight(baseURL, "/"),
		chatDeployment:       chatDeployment,
		embeddingDeployment:  embeddingDeployment,
		multimodalDeployment: multimodalDeployment,
		httpClient:           httpClient,
	}
}

func (a *azureOpenAIAdapter) Chat(ctx context.Context, messages []ark.ChatMessage) (string, error) {
	reqBody := map[string]any{
		"messages": messages,
		"stream":   false,
	}
	var resp struct {
		Choices []struct {
			Message ark.ChatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := a.postJSON(ctx, "/openai/deployments/"+a.chatDeployment+"/chat/completions", reqBody, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("azure openai chat response did not contain content")
	}
	return resp.Choices[0].Message.Content, nil
}

func (a *azureOpenAIAdapter) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := map[string]any{"input": texts}
	var resp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := a.postJSON(ctx, "/openai/deployments/"+a.embeddingDeployment+"/embeddings", reqBody, &resp); err != nil {
		return nil, err
	}
	out := make([][]float64, len(resp.Data))
	for i := range resp.Data {
		out[i] = resp.Data[i].Embedding
	}
	return out, nil
}

func (a *azureOpenAIAdapter) MultimodalParse(ctx context.Context, name string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", fmt.Errorf("empty document %s", name)
	}
	reqBody := map[string]any{
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "请将输入文件解析为结构化 Markdown，只输出 Markdown。"},
				openAIContentPart(name, content),
			},
		}},
		"stream": false,
	}
	var resp struct {
		Choices []struct {
			Message ark.ChatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := a.postJSON(ctx, "/openai/deployments/"+a.multimodalDeployment+"/chat/completions", reqBody, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 || strings.TrimSpace(resp.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("azure openai multimodal response did not contain content")
	}
	return resp.Choices[0].Message.Content, nil
}

func (a *azureOpenAIAdapter) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path+"?api-version="+defaultAzureAPIVersion, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("api-key", a.apiKey)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("azure openai status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	if len(bodyBytes) == 0 {
		return nil
	}
	return json.Unmarshal(bodyBytes, out)
}

type voyageAdapter struct {
	apiKey         string
	baseURL        string
	embeddingModel string
	rerankModel    string
	httpClient     *http.Client
}

func newVoyageAdapter(apiKey string, baseURL string, cfg Config, httpClient *http.Client) *voyageAdapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	info := BuiltinRegistry().MustGet(VoyageAI)
	return &voyageAdapter{
		apiKey:         apiKey,
		baseURL:        strings.TrimRight(baseURL, "/"),
		embeddingModel: firstNonEmpty(cfg.EmbeddingModel, info.DefaultModels[CapabilityEmbedding]),
		rerankModel:    firstNonEmpty(cfg.RerankModel, info.DefaultModels[CapabilityRerank]),
		httpClient:     httpClient,
	}
}

func (a *voyageAdapter) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := map[string]any{"model": a.embeddingModel, "input": texts}
	var resp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := a.postJSON(ctx, "/embeddings", reqBody, &resp); err != nil {
		return nil, err
	}
	out := make([][]float64, len(resp.Data))
	for i := range resp.Data {
		out[i] = resp.Data[i].Embedding
	}
	return out, nil
}

func (a *voyageAdapter) Rerank(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
	documents := make([]string, len(docs))
	for i := range docs {
		documents[i] = docs[i].Content
	}
	if topN <= 0 || topN > len(documents) {
		topN = len(documents)
	}
	reqBody := map[string]any{
		"model":     a.rerankModel,
		"query":     query,
		"documents": documents,
		"top_k":     topN,
	}
	var resp struct {
		Data []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"data"`
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := a.postJSON(ctx, "/rerank", reqBody, &resp); err != nil {
		return nil, err
	}
	results := resp.Data
	if len(results) == 0 {
		results = resp.Results
	}
	out := make([]ark.RerankResult, len(results))
	for i := range results {
		out[i] = ark.RerankResult{Index: results[i].Index, Score: results[i].RelevanceScore}
	}
	return out, nil
}

func (a *voyageAdapter) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("voyage status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	if len(bodyBytes) == 0 {
		return nil
	}
	return json.Unmarshal(bodyBytes, out)
}

type cohereAdapter struct {
	apiKey         string
	baseURL        string
	chatModel      string
	embeddingModel string
	rerankModel    string
	httpClient     *http.Client
}

func newCohereAdapter(apiKey string, baseURL string, cfg Config, httpClient *http.Client) *cohereAdapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	info := BuiltinRegistry().MustGet(Cohere)
	chatModel := strings.TrimSpace(cfg.ChatModel)
	if chatModel == "" {
		chatModel = info.DefaultModels[CapabilityChat]
	}
	embeddingModel := strings.TrimSpace(cfg.EmbeddingModel)
	if embeddingModel == "" {
		embeddingModel = info.DefaultModels[CapabilityEmbedding]
	}
	rerankModel := strings.TrimSpace(cfg.RerankModel)
	if rerankModel == "" {
		rerankModel = info.DefaultModels[CapabilityRerank]
	}
	return &cohereAdapter{
		apiKey:         apiKey,
		baseURL:        strings.TrimRight(baseURL, "/"),
		chatModel:      chatModel,
		embeddingModel: embeddingModel,
		rerankModel:    rerankModel,
		httpClient:     httpClient,
	}
}

func (a *cohereAdapter) Chat(ctx context.Context, messages []ark.ChatMessage) (string, error) {
	reqBody := map[string]any{
		"model":    a.chatModel,
		"messages": cohereMessages(messages),
	}
	var resp struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
		Text string `json:"text"`
	}
	if err := a.postJSON(ctx, "/v2/chat", reqBody, &resp); err != nil {
		return "", err
	}
	for _, item := range resp.Message.Content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text, nil
		}
	}
	if strings.TrimSpace(resp.Text) != "" {
		return resp.Text, nil
	}
	return "", fmt.Errorf("cohere chat response did not contain content")
}

func (a *cohereAdapter) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := map[string]any{
		"model":           a.embeddingModel,
		"texts":           texts,
		"input_type":      "search_document",
		"embedding_types": []string{"float"},
	}
	var resp struct {
		Embeddings struct {
			Float [][]float64 `json:"float"`
		} `json:"embeddings"`
		LegacyEmbeddings [][]float64 `json:"-"`
	}
	raw := map[string]json.RawMessage{}
	if err := a.postJSON(ctx, "/v2/embed", reqBody, &raw); err != nil {
		return nil, err
	}
	if payload, ok := raw["embeddings"]; ok {
		if err := json.Unmarshal(payload, &resp.Embeddings); err == nil && len(resp.Embeddings.Float) > 0 {
			return resp.Embeddings.Float, nil
		}
		var legacy [][]float64
		if err := json.Unmarshal(payload, &legacy); err == nil && len(legacy) > 0 {
			return legacy, nil
		}
	}
	return nil, fmt.Errorf("cohere embed response did not contain vectors")
}

func (a *cohereAdapter) Rerank(ctx context.Context, query string, docs []ark.RerankDocument, topN int) ([]ark.RerankResult, error) {
	documents := make([]string, len(docs))
	for i := range docs {
		documents[i] = docs[i].Content
	}
	if topN <= 0 || topN > len(documents) {
		topN = len(documents)
	}
	reqBody := map[string]any{
		"model":     a.rerankModel,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	}
	var resp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
	}
	if err := a.postJSON(ctx, "/v2/rerank", reqBody, &resp); err != nil {
		return nil, err
	}
	out := make([]ark.RerankResult, len(resp.Results))
	for i := range resp.Results {
		out[i] = ark.RerankResult{Index: resp.Results[i].Index, Score: resp.Results[i].RelevanceScore}
	}
	return out, nil
}

func (a *cohereAdapter) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("cohere status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	if len(bodyBytes) == 0 {
		return nil
	}
	return json.Unmarshal(bodyBytes, out)
}

func cohereMessages(messages []ark.ChatMessage) []map[string]string {
	out := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		switch role {
		case "assistant", "system", "user":
		default:
			role = "user"
		}
		out = append(out, map[string]string{"role": role, "content": message.Content})
	}
	return out
}

type anthropicAdapter struct {
	apiKey          string
	baseURL         string
	model           string
	multimodalModel string
	httpClient      *http.Client
}

func newAnthropicAdapter(apiKey string, baseURL string, cfg Config, httpClient *http.Client) *anthropicAdapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	info := BuiltinRegistry().MustGet(Anthropic)
	chatModel := strings.TrimSpace(cfg.ChatModel)
	if chatModel == "" {
		chatModel = info.DefaultModels[CapabilityChat]
	}
	multimodalModel := strings.TrimSpace(cfg.MultimodalModel)
	if multimodalModel == "" {
		multimodalModel = chatModel
	}
	return &anthropicAdapter{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), model: chatModel, multimodalModel: multimodalModel, httpClient: httpClient}
}

func (a *anthropicAdapter) Chat(ctx context.Context, messages []ark.ChatMessage) (string, error) {
	reqBody := map[string]any{
		"model":      a.model,
		"max_tokens": 4096,
		"messages":   anthropicMessages(messages),
	}
	if system := systemPrompt(messages); system != "" {
		reqBody["system"] = system
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := a.postJSON(ctx, "/v1/messages", reqBody, &resp); err != nil {
		return "", err
	}
	for _, item := range resp.Content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic response did not contain content")
}

func (a *anthropicAdapter) MultimodalParse(ctx context.Context, name string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", fmt.Errorf("empty document %s", name)
	}
	reqBody := map[string]any{
		"model":      a.multimodalModel,
		"max_tokens": 4096,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "请将输入文件解析为结构化 Markdown，只输出 Markdown。"},
				anthropicMultimodalContentPart(name, content),
			},
		}},
	}
	var resp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := a.postJSON(ctx, "/v1/messages", reqBody, &resp); err != nil {
		return "", err
	}
	for _, item := range resp.Content {
		if strings.TrimSpace(item.Text) != "" {
			return item.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic multimodal response did not contain content")
}

func (a *anthropicAdapter) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", a.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return json.Unmarshal(bodyBytes, out)
}

func anthropicMessages(messages []ark.ChatMessage) []map[string]string {
	out := make([]map[string]string, 0, len(messages))
	for _, message := range messages {
		role := strings.ToLower(strings.TrimSpace(message.Role))
		if role == "system" {
			continue
		}
		if role != "assistant" {
			role = "user"
		}
		out = append(out, map[string]string{"role": role, "content": message.Content})
	}
	if len(out) == 0 {
		out = append(out, map[string]string{"role": "user", "content": ""})
	}
	return out
}

func anthropicMultimodalContentPart(name string, content []byte) map[string]any {
	source := map[string]string{
		"type":       "base64",
		"media_type": mediaType(name),
		"data":       base64.StdEncoding.EncodeToString(content),
	}
	if strings.HasPrefix(source["media_type"], "image/") {
		return map[string]any{"type": "image", "source": source}
	}
	return map[string]any{"type": "document", "source": source}
}

type geminiAdapter struct {
	apiKey          string
	baseURL         string
	model           string
	embeddingModel  string
	multimodalModel string
	httpClient      *http.Client
}

func newGeminiAdapter(apiKey string, baseURL string, cfg Config, httpClient *http.Client) *geminiAdapter {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	model := strings.TrimSpace(cfg.ChatModel)
	if model == "" {
		model = BuiltinRegistry().MustGet(Gemini).DefaultModels[CapabilityChat]
	}
	embeddingModel := strings.TrimSpace(cfg.EmbeddingModel)
	if embeddingModel == "" {
		embeddingModel = BuiltinRegistry().MustGet(Gemini).DefaultModels[CapabilityEmbedding]
	}
	multimodalModel := strings.TrimSpace(cfg.MultimodalModel)
	if multimodalModel == "" {
		multimodalModel = model
	}
	return &geminiAdapter{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), model: model, embeddingModel: embeddingModel, multimodalModel: multimodalModel, httpClient: httpClient}
}

func (a *geminiAdapter) Chat(ctx context.Context, messages []ark.ChatMessage) (string, error) {
	reqBody := map[string]any{
		"contents": []map[string]any{{
			"role": "user",
			"parts": []map[string]string{{
				"text": joinedPrompt(messages),
			}},
		}},
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	path := "/models/" + a.model + ":generateContent?key=" + a.apiKey
	if err := a.postJSON(ctx, path, reqBody, &resp); err != nil {
		return "", err
	}
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text, nil
			}
		}
	}
	return "", fmt.Errorf("gemini response did not contain content")
}

func (a *geminiAdapter) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i, text := range texts {
		reqBody := map[string]any{
			"model": "models/" + a.embeddingModel,
			"content": map[string]any{
				"parts": []map[string]string{{"text": text}},
			},
		}
		var resp struct {
			Embedding struct {
				Values []float64 `json:"values"`
			} `json:"embedding"`
		}
		path := "/models/" + a.embeddingModel + ":embedContent?key=" + a.apiKey
		if err := a.postJSON(ctx, path, reqBody, &resp); err != nil {
			return nil, err
		}
		if len(resp.Embedding.Values) == 0 {
			return nil, fmt.Errorf("gemini embedding response %d did not contain vector", i)
		}
		out[i] = resp.Embedding.Values
	}
	return out, nil
}

func (a *geminiAdapter) MultimodalParse(ctx context.Context, name string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", fmt.Errorf("empty document %s", name)
	}
	reqBody := map[string]any{
		"contents": []map[string]any{{
			"role": "user",
			"parts": []map[string]any{
				{"text": "请将输入文件解析为结构化 Markdown，只输出 Markdown。"},
				{
					"inline_data": map[string]string{
						"mime_type": mediaType(name),
						"data":      base64.StdEncoding.EncodeToString(content),
					},
				},
			},
		}},
	}
	var resp struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	path := "/models/" + a.multimodalModel + ":generateContent?key=" + a.apiKey
	if err := a.postJSON(ctx, path, reqBody, &resp); err != nil {
		return "", err
	}
	for _, candidate := range resp.Candidates {
		for _, part := range candidate.Content.Parts {
			if strings.TrimSpace(part.Text) != "" {
				return part.Text, nil
			}
		}
	}
	return "", fmt.Errorf("gemini multimodal response did not contain content")
}

func (a *geminiAdapter) postJSON(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gemini status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return json.Unmarshal(bodyBytes, out)
}

func systemPrompt(messages []ark.ChatMessage) string {
	var parts []string
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "system") && strings.TrimSpace(message.Content) != "" {
			parts = append(parts, message.Content)
		}
	}
	return strings.Join(parts, "\n")
}

func joinedPrompt(messages []ark.ChatMessage) string {
	var parts []string
	for _, message := range messages {
		if strings.TrimSpace(message.Content) == "" {
			continue
		}
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "user"
		}
		parts = append(parts, role+": "+message.Content)
	}
	return strings.Join(parts, "\n")
}

func apiKeyFor(keys map[Name]string, name Name) string {
	if keys == nil {
		return ""
	}
	if value := strings.TrimSpace(keys[name]); value != "" {
		return value
	}
	return strings.TrimSpace(keys[NormalizeName(string(name))])
}

func baseURLFor(urls map[Name]string, name Name) string {
	if urls == nil {
		return ""
	}
	if value := strings.TrimSpace(urls[name]); value != "" {
		return value
	}
	return strings.TrimSpace(urls[NormalizeName(string(name))])
}

func DefaultBaseURL(name Name) string {
	switch name {
	case VolcEngine:
		return "https://ark.cn-beijing.volces.com/api/v3"
	case OpenAI:
		return "https://api.openai.com/v1"
	case AzureOpenAI:
		return ""
	case Anthropic:
		return "https://api.anthropic.com"
	case Gemini:
		return "https://generativelanguage.googleapis.com/v1beta"
	case GoogleCloud:
		return ""
	case XAI:
		return "https://api.x.ai/v1"
	case Mistral:
		return "https://api.mistral.ai/v1"
	case Cohere:
		return "https://api.cohere.com"
	case DeepSeek:
		return "https://api.deepseek.com/v1"
	case Moonshot:
		return "https://api.moonshot.cn/v1"
	case MiniMax:
		return "https://api.minimax.chat/v1"
	case BaiChuan:
		return "https://api.baichuan-ai.com/v1"
	case ZhipuAI:
		return "https://open.bigmodel.cn/api/paas/v4"
	case TongyiQianwen:
		return "https://dashscope.aliyuncs.com/compatible-api/v1"
	case TencentHunyuan:
		return "https://api.hunyuan.cloud.tencent.com/v1"
	case XunFeiSpark:
		return "https://spark-api-open.xf-yun.com/v1"
	case BaiduYiyan:
		return "https://qianfan.baidubce.com/v2"
	case Xiaomi:
		return "https://api.xiaomi.com/v1"
	case Perplexity:
		return "https://api.perplexity.ai"
	case VoyageAI:
		return "https://api.voyageai.com/v1"
	case Jina:
		return "https://api.jina.ai/v1"
	default:
		return ""
	}
}
