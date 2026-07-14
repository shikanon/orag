package orag_test

import (
	"context"
	"errors"
	"testing"

	orag "github.com/shikanon/orag"
)

func TestSDKStreamQueryEmitsResponseAndDone(t *testing.T) {
	ctx := context.Background()
	client, err := orag.New(ctx, orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if _, err := client.IngestText(ctx, orag.IngestTextRequest{KnowledgeBaseID: "kb_default", Name: "stream.txt", SourceURI: "memory://stream", Text: "ORAG supports typed SDK query events."}); err != nil {
		t.Fatal(err)
	}

	var eventTypes []orag.QueryEventType
	for event := range client.StreamQuery(ctx, orag.QueryRequest{KnowledgeBaseID: "kb_default", Query: "What does ORAG support?"}) {
		if event.Err != nil {
			t.Fatalf("stream error = %v", event.Err)
		}
		eventTypes = append(eventTypes, event.Type)
	}
	if len(eventTypes) != 2 || eventTypes[0] != orag.QueryEventResponse || eventTypes[1] != orag.QueryEventDone {
		t.Fatalf("event types = %#v", eventTypes)
	}
}

func TestSDKStreamQueryCancellationIsTerminal(t *testing.T) {
	client, err := orag.New(context.Background(), orag.MockConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var events []orag.QueryEvent
	for event := range client.StreamQuery(ctx, orag.QueryRequest{KnowledgeBaseID: "kb_default", Query: "cancel"}) {
		events = append(events, event)
	}
	if len(events) != 1 || events[0].Type != orag.QueryEventError || !errors.Is(events[0].Err, orag.ErrCanceled) {
		t.Fatalf("events = %#v", events)
	}
}
