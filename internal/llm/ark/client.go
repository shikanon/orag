package ark

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

type Config struct {
	APIKey              string
	BaseURL             string
	ChatModel           string
	EmbeddingModel      string
	EmbeddingDimensions int
	RerankProvider      string
	RerankBaseURL       string
	RerankModel         string
	RerankAPIKey        string
	RerankInstruct      string
	MultimodalModel     string
	Timeout             time.Duration
	RetryTimes          int
}

type Client struct {
	cfg        Config
	httpClient *http.Client
}

func NewClient(cfg Config, httpClient *http.Client) *Client {
	if cfg.EmbeddingDimensions <= 0 {
		cfg.EmbeddingDimensions = 1024
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.RetryTimes < 0 {
		cfg.RetryTimes = 0
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.Timeout}
	}
	return &Client{cfg: cfg, httpClient: httpClient}
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (c *Client) Chat(ctx context.Context, messages []ChatMessage) (string, error) {
	if c.cfg.APIKey == "" {
		return deterministicAnswer(messages), nil
	}
	reqBody := map[string]any{
		"model":    c.cfg.ChatModel,
		"messages": messages,
		"stream":   false,
	}
	var resp struct {
		Choices []struct {
			Message ChatMessage `json:"message"`
		} `json:"choices"`
		Output []struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := c.postJSON(ctx, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", reqBody, &resp); err != nil {
		return "", err
	}
	if len(resp.Choices) > 0 {
		return resp.Choices[0].Message.Content, nil
	}
	if len(resp.Output) > 0 && len(resp.Output[0].Content) > 0 {
		return resp.Output[0].Content[0].Text, nil
	}
	return "", fmt.Errorf("ark chat response did not contain content")
}

func (c *Client) ChatStream(ctx context.Context, messages []ChatMessage) (<-chan StreamChunk, <-chan error) {
	chunks := make(chan StreamChunk, 8)
	errs := make(chan error, 1)
	go func() {
		defer close(chunks)
		defer close(errs)
		if c.cfg.APIKey == "" {
			chunks <- StreamChunk{Content: deterministicAnswer(messages)}
			chunks <- StreamChunk{Done: true}
			return
		}
		reqBody := map[string]any{
			"model":    c.cfg.ChatModel,
			"messages": messages,
			"stream":   true,
		}
		if err := c.postStream(ctx, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", reqBody, chunks); err != nil {
			errs <- err
			return
		}
		chunks <- StreamChunk{Done: true}
	}()
	return chunks, errs
}

func (c *Client) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if c.cfg.APIKey == "" {
		out := make([][]float64, len(texts))
		for i, text := range texts {
			out[i] = deterministicEmbedding(text, c.cfg.EmbeddingDimensions)
		}
		return out, nil
	}
	if isMultimodalEmbeddingModel(c.cfg.EmbeddingModel) {
		return c.embedMultimodal(ctx, texts)
	}
	reqBody := map[string]any{
		"model": c.cfg.EmbeddingModel,
		"input": texts,
	}
	var resp struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := c.postJSON(ctx, strings.TrimRight(c.cfg.BaseURL, "/")+"/embeddings", reqBody, &resp); err != nil {
		return nil, err
	}
	if len(resp.Data) != len(texts) {
		return nil, fmt.Errorf("ark embedding response count %d does not match input count %d", len(resp.Data), len(texts))
	}
	out := make([][]float64, len(resp.Data))
	for i := range resp.Data {
		out[i] = resp.Data[i].Embedding
	}
	return out, nil
}

func (c *Client) embedMultimodal(ctx context.Context, texts []string) ([][]float64, error) {
	out := make([][]float64, len(texts))
	for i, text := range texts {
		reqBody := map[string]any{
			"model":           c.cfg.EmbeddingModel,
			"encoding_format": "float",
			"dimensions":      c.cfg.EmbeddingDimensions,
			"input": []map[string]string{{
				"type": "text",
				"text": text,
			}},
		}
		var resp struct {
			Data struct {
				Embedding []float64 `json:"embedding"`
			} `json:"data"`
		}
		if err := c.postJSON(ctx, strings.TrimRight(c.cfg.BaseURL, "/")+"/embeddings/multimodal", reqBody, &resp); err != nil {
			return nil, err
		}
		if len(resp.Data.Embedding) == 0 {
			return nil, fmt.Errorf("ark multimodal embedding response %d did not contain vector", i)
		}
		out[i] = resp.Data.Embedding
	}
	return out, nil
}

type RerankDocument struct {
	ID      string
	Content string
}

type RerankResult struct {
	Index int
	Score float64
}

func (c *Client) Rerank(ctx context.Context, query string, docs []RerankDocument, topN int) ([]RerankResult, error) {
	provider := strings.ToLower(strings.TrimSpace(c.cfg.RerankProvider))
	if provider == "" {
		provider = "volcengine"
	}
	if provider == "aliyun" {
		return c.rerankAliyun(ctx, query, docs, topN)
	}
	if c.cfg.APIKey == "" {
		results := make([]RerankResult, len(docs))
		for i, doc := range docs {
			results[i] = RerankResult{Index: i, Score: lexicalScore(query, doc.Content)}
		}
		sortRerank(results)
		return limit(results, topN), nil
	}
	documents := make([]string, len(docs))
	for i := range docs {
		documents[i] = docs[i].Content
	}
	reqBody := map[string]any{
		"model":     c.cfg.RerankModel,
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
	if err := c.postJSON(ctx, strings.TrimRight(c.cfg.RerankBaseURL, "/")+"/rerank", reqBody, &resp); err != nil {
		return nil, err
	}
	out := make([]RerankResult, len(resp.Results))
	for i := range resp.Results {
		out[i] = RerankResult{Index: resp.Results[i].Index, Score: resp.Results[i].RelevanceScore}
	}
	return out, nil
}

func (c *Client) rerankAliyun(ctx context.Context, query string, docs []RerankDocument, topN int) ([]RerankResult, error) {
	apiKey := strings.TrimSpace(c.cfg.RerankAPIKey)
	if apiKey == "" {
		return nil, fmt.Errorf("ALIYUN_RERANK_API_KEY is required when RERANK_PROVIDER=aliyun")
	}
	documents := make([]string, len(docs))
	for i := range docs {
		documents[i] = docs[i].Content
	}
	if topN <= 0 || topN > len(documents) {
		topN = len(documents)
	}
	reqBody := map[string]any{
		"model":     c.cfg.RerankModel,
		"query":     query,
		"documents": documents,
		"top_n":     topN,
	}
	if strings.TrimSpace(c.cfg.RerankInstruct) != "" {
		reqBody["instruct"] = c.cfg.RerankInstruct
	}
	var resp struct {
		Results []struct {
			Index          int     `json:"index"`
			RelevanceScore float64 `json:"relevance_score"`
		} `json:"results"`
		Output struct {
			Results []struct {
				Index          int     `json:"index"`
				RelevanceScore float64 `json:"relevance_score"`
			} `json:"results"`
		} `json:"output"`
	}
	url := strings.TrimRight(c.cfg.RerankBaseURL, "/") + "/reranks"
	if err := c.postJSONWithBearer(ctx, url, reqBody, &resp, apiKey); err != nil {
		return nil, err
	}
	results := resp.Results
	if len(results) == 0 {
		results = resp.Output.Results
	}
	out := make([]RerankResult, len(results))
	for i := range results {
		out[i] = RerankResult{Index: results[i].Index, Score: results[i].RelevanceScore}
	}
	return out, nil
}

func (c *Client) MultimodalParse(ctx context.Context, name string, content []byte) (string, error) {
	if len(content) == 0 {
		return "", fmt.Errorf("empty document %s", name)
	}
	if c.cfg.APIKey != "" {
		reqBody := map[string]any{
			"model": c.cfg.MultimodalModel,
			"messages": []map[string]any{{
				"role": "user",
				"content": []map[string]any{
					{"type": "text", "text": "请将输入文件解析为结构化 Markdown，只输出 Markdown。"},
					multimodalContentPart(name, content),
				},
			}},
			"stream": false,
		}
		var resp struct {
			Choices []struct {
				Message ChatMessage `json:"message"`
			} `json:"choices"`
			Output []struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"output"`
		}
		if err := c.postJSON(ctx, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", reqBody, &resp); err != nil {
			return "", err
		}
		if len(resp.Choices) > 0 && strings.TrimSpace(resp.Choices[0].Message.Content) != "" {
			return resp.Choices[0].Message.Content, nil
		}
		if len(resp.Output) > 0 && len(resp.Output[0].Content) > 0 {
			return resp.Output[0].Content[0].Text, nil
		}
		return "", fmt.Errorf("ark multimodal response did not contain content")
	}
	text := string(content)
	if len(text) > 64*1024 {
		text = text[:64*1024]
	}
	return fmt.Sprintf("# %s\n\n%s", name, text), nil
}

func multimodalContentPart(name string, content []byte) map[string]any {
	dataURL := "data:" + mediaType(name) + ";base64," + base64.StdEncoding.EncodeToString(content)
	if strings.HasSuffix(strings.ToLower(name), ".png") ||
		strings.HasSuffix(strings.ToLower(name), ".jpg") ||
		strings.HasSuffix(strings.ToLower(name), ".jpeg") ||
		strings.HasSuffix(strings.ToLower(name), ".webp") {
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
	case strings.HasSuffix(lower, ".mp3"):
		return "audio/mpeg"
	case strings.HasSuffix(lower, ".wav"):
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

func isMultimodalEmbeddingModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "embedding-vision")
}

func (c *Client) postJSON(ctx context.Context, url string, body any, out any) error {
	return c.postJSONWithBearer(ctx, url, body, out, c.cfg.APIKey)
}

func (c *Client) postJSONWithBearer(ctx context.Context, url string, body any, out any, apiKey string) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt <= c.cfg.RetryTimes; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+apiKey)
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			defer resp.Body.Close()
			bodyBytes, _ := io.ReadAll(resp.Body)
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if len(bodyBytes) == 0 {
					return nil
				}
				return json.Unmarshal(bodyBytes, out)
			}
			lastErr = fmt.Errorf("ark status %d: %s", resp.StatusCode, string(bodyBytes))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		}
	}
	return lastErr
}

func (c *Client) postStream(ctx context.Context, url string, body any, chunks chan<- StreamChunk) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ark stream status %d: %s", resp.StatusCode, string(bodyBytes))
	}
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}
		content := streamContent(data)
		if content != "" {
			chunks <- StreamChunk{Content: content}
		}
	}
	return scanner.Err()
}

func streamContent(data string) string {
	var resp struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			Message ChatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(data), &resp); err != nil {
		return ""
	}
	if len(resp.Choices) == 0 {
		return ""
	}
	if resp.Choices[0].Delta.Content != "" {
		return resp.Choices[0].Delta.Content
	}
	return resp.Choices[0].Message.Content
}

func deterministicAnswer(messages []ChatMessage) string {
	var user string
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			user = messages[i].Content
			break
		}
	}
	if user == "" {
		user = "当前问题"
	}
	return "基于已检索上下文，" + truncate(user, 120)
}

func deterministicEmbedding(text string, dims int) []float64 {
	vec := make([]float64, dims)
	seed := sha256.Sum256([]byte(text))
	for i := range vec {
		offset := (i * 4) % len(seed)
		n := binary.BigEndian.Uint32(seed[offset : offset+4])
		vec[i] = float64(n%2000)/1000.0 - 1.0
	}
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return vec
	}
	for i := range vec {
		vec[i] /= norm
	}
	return vec
}

func lexicalScore(query, content string) float64 {
	terms := strings.Fields(strings.ToLower(query))
	text := strings.ToLower(content)
	if len(terms) == 0 {
		return 0
	}
	hits := 0
	for _, term := range terms {
		if strings.Contains(text, term) {
			hits++
		}
	}
	return float64(hits) / float64(len(terms))
}

func sortRerank(results []RerankResult) {
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

func limit(results []RerankResult, topN int) []RerankResult {
	if topN > 0 && len(results) > topN {
		return results[:topN]
	}
	return results
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
