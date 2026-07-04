package ingest

import (
	"context"
	"fmt"
	"strings"

	"github.com/shikanon/orag/internal/llm/ark"
)

type ContextualFailureMode string

const (
	ContextualFailureFallback ContextualFailureMode = "fallback"
	ContextualFailureFail     ContextualFailureMode = "fail"
)

type ChatModel interface {
	Chat(ctx context.Context, messages []ark.ChatMessage) (string, error)
}

type LLMContextualizer struct {
	Model            ChatModel
	MaxDocumentChars int
	MaxChunkChars    int
	MaxContextChars  int
	FailureMode      ContextualFailureMode
}

func (c LLMContextualizer) Contextualize(ctx context.Context, req ContextualizationRequest) ([]string, []string, error) {
	contexts := make([]string, len(req.Chunks))
	if len(req.Chunks) == 0 {
		return contexts, nil, nil
	}
	if c.Model == nil {
		return c.handleFailure(contexts, fmt.Errorf("contextualization model is nil"))
	}
	document := trimRunes(req.DocumentText, defaultInt(c.MaxDocumentChars, 12000))
	for i, chunk := range req.Chunks {
		answer, err := c.Model.Chat(ctx, []ark.ChatMessage{
			{Role: "system", Content: "你是 RAG 文档分块定位器。请基于全文和当前 chunk，输出一句简短上下文，说明该 chunk 在全文中的位置和主题。只输出上下文本身。"},
			{Role: "user", Content: fmt.Sprintf("document_name: %s\n\ndocument:\n%s\n\nchunk:\n%s", req.DocumentName, document, trimRunes(chunk.Content, defaultInt(c.MaxChunkChars, 2000)))},
		})
		if err != nil {
			return c.handleFailure(contexts, err)
		}
		contexts[i] = cleanContextualText(answer, defaultInt(c.MaxContextChars, 500))
	}
	return contexts, nil, nil
}

func (c LLMContextualizer) handleFailure(contexts []string, err error) ([]string, []string, error) {
	if c.FailureMode == ContextualFailureFail {
		return nil, nil, err
	}
	return contexts, []string{"contextualization failed: " + err.Error()}, nil
}

func cleanContextualText(text string, maxRunes int) string {
	text = strings.TrimSpace(stripSimpleCodeFence(text))
	return trimRunes(text, maxRunes)
}

func stripSimpleCodeFence(text string) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return text
	}
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimPrefix(text, "text")
	text = strings.TrimPrefix(text, "markdown")
	text = strings.TrimSpace(text)
	text = strings.TrimSuffix(text, "```")
	return strings.TrimSpace(text)
}

func trimRunes(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes])
}

func defaultInt(value int, fallback int) int {
	if value > 0 {
		return value
	}
	return fallback
}
