package ark

import (
	"context"

	"github.com/cloudwego/eino/components/embedding"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

var _ model.BaseChatModel = (*EinoChatModel)(nil)
var _ embedding.Embedder = (*EinoEmbedder)(nil)

type EinoChatModel struct {
	Client *Client
}

func NewEinoChatModel(client *Client) *EinoChatModel {
	return &EinoChatModel{Client: client}
}

func (m *EinoChatModel) Generate(ctx context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	messages := make([]ChatMessage, 0, len(input))
	for _, item := range input {
		if item == nil {
			continue
		}
		messages = append(messages, ChatMessage{Role: string(item.Role), Content: item.Content})
	}
	answer, err := m.Client.Chat(ctx, messages)
	if err != nil {
		return nil, err
	}
	return schema.AssistantMessage(answer, nil), nil
}

func (m *EinoChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](1)
	go func() {
		defer writer.Close()
		msg, err := m.Generate(ctx, input, opts...)
		writer.Send(msg, err)
	}()
	return reader, nil
}

type EinoEmbedder struct {
	Client *Client
}

func NewEinoEmbedder(client *Client) *EinoEmbedder {
	return &EinoEmbedder{Client: client}
}

func (e *EinoEmbedder) EmbedStrings(ctx context.Context, texts []string, _ ...embedding.Option) ([][]float64, error) {
	return e.Client.Embed(ctx, texts)
}
