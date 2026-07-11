package pipeline

import (
	"encoding/json"
	"fmt"
	"sort"
)

type Registry struct {
	definitions map[string]NodeDefinition
}

func NewRegistry(definitions ...NodeDefinition) (Registry, error) {
	registry := Registry{definitions: make(map[string]NodeDefinition, len(definitions))}
	for _, definition := range definitions {
		if _, exists := registry.definitions[definition.Type]; exists {
			return Registry{}, fmt.Errorf("duplicate pipeline node type %q", definition.Type)
		}
		registry.definitions[definition.Type] = definition
	}
	return registry, nil
}

// MustRegistry constructs a registry or panics. It is intended for static definitions.
func MustRegistry(definitions ...NodeDefinition) Registry {
	registry, err := NewRegistry(definitions...)
	if err != nil {
		panic(err)
	}
	return registry
}

func (r Registry) Lookup(typeID string) (NodeDefinition, bool) {
	definition, ok := r.definitions[typeID]
	return definition, ok
}

func (r Registry) Definitions() []NodeDefinition {
	result := make([]NodeDefinition, 0, len(r.definitions))
	for _, definition := range r.definitions {
		result = append(result, definition)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Type < result[j].Type })
	return result
}

var emptyObjectSchema = json.RawMessage(`{"type":"object","additionalProperties":false}`)
var emptyObject = json.RawMessage(`{}`)

// BuiltinRegistry returns the stable public node type IDs backed by internal/graph.
func BuiltinRegistry() Registry {
	type stage struct {
		id, name, category    string
		entry, producesAnswer bool
		singleton             bool
	}
	stages := []stage{
		{id: "init", name: "Query Input", category: "input", entry: true, singleton: true},
		{id: "query_route", name: "Query Route", category: "routing"},
		{id: "semantic_cache_lookup", name: "Semantic Cache Lookup", category: "cache"},
		{id: "query_rewrite", name: "Query Rewrite", category: "query"},
		{id: "multi_query", name: "Multi Query", category: "query"},
		{id: "hybrid_retrieve", name: "Hybrid Retrieve", category: "retrieval"},
		{id: "ark_rerank", name: "Ark Rerank", category: "retrieval"},
		{id: "context_pack", name: "Context Pack", category: "context"},
		{id: "prompt_prefix_cache", name: "Prompt Prefix Cache", category: "prompt"},
		{id: "ark_generate", name: "Ark Generate", category: "generation", producesAnswer: true},
		{id: "semantic_cache_write", name: "Semantic Cache Write", category: "cache"},
	}
	definitions := make([]NodeDefinition, 0, len(stages))
	for i, item := range stages {
		definition := NodeDefinition{
			Type: item.id, DisplayName: item.name, Category: item.category,
			SchemaVersion: 1, ConfigSchema: append(json.RawMessage(nil), emptyObjectSchema...),
			DefaultConfig: append(json.RawMessage(nil), emptyObject...), Singleton: item.singleton,
			Entry: item.entry, ProducesAnswer: item.producesAnswer,
		}
		if i > 0 {
			definition.Inputs = []PortDefinition{{Name: "in", Type: "rag_state", MaxConnections: 1}}
		}
		if i < len(stages)-1 {
			definition.Outputs = []PortDefinition{{Name: "out", Type: "rag_state", MaxConnections: 1}}
			definition.AllowedTargets = []string{stages[i+1].id}
		}
		definitions = append(definitions, definition)
	}
	return MustRegistry(definitions...)
}
