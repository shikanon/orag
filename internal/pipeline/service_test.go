package pipeline

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func validDefinition() Definition {
	types := []string{"init", "query_route", "semantic_cache_lookup", "query_rewrite", "multi_query", "hybrid_retrieve", "ark_rerank", "context_pack", "prompt_prefix_cache", "ark_generate", "semantic_cache_write"}
	nodes := make([]Node, len(types))
	edges := make([]Edge, len(types)-1)
	for i, typeID := range types {
		nodes[i] = Node{ID: typeID, Type: typeID}
		if i > 0 {
			edges[i-1] = Edge{ID: fmt.Sprintf("e%d", i), SourceNodeID: types[i-1], SourcePort: "out", TargetNodeID: typeID, TargetPort: "in"}
		}
	}
	return Definition{Nodes: nodes, Edges: edges}
}

func TestSaveDraftValidatesAndUsesRevisionCAS(t *testing.T) {
	repo := NewMemoryRepository()
	service := NewService(repo, BuiltinRegistry())
	now := time.Now().UTC()
	if err := service.CreatePipeline(context.Background(), Pipeline{ID: "pipe_1", ProjectID: "prj_1", Name: "Support", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	first, err := service.SaveDraft(context.Background(), "prj_1", "pipe_1", 0, validDefinition())
	if err != nil || first.Revision != 1 {
		t.Fatalf("first save = %#v, %v", first, err)
	}
	second, err := service.SaveDraft(context.Background(), "prj_1", "pipe_1", 0, validDefinition())
	if !errors.Is(err, ErrRevisionConflict) || second.Revision != 1 {
		t.Fatalf("stale save = %#v, %v", second, err)
	}
}

func TestSaveDraftRejectsInvalidGraph(t *testing.T) {
	service := NewService(NewMemoryRepository(), BuiltinRegistry())
	_, err := service.SaveDraft(context.Background(), "prj_1", "pipe_1", 0, Definition{Nodes: []Node{{ID: "unknown", Type: "nope"}}})
	var validation ValidationErrors
	if !errors.As(err, &validation) || !errors.Is(err, ErrInvalidDefinition) {
		t.Fatalf("error = %v, want validation errors", err)
	}
}
