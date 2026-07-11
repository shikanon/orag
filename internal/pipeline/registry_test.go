package pipeline

import "testing"

func TestBuiltinRegistryDescribesCurrentGraphStages(t *testing.T) {
	registry := BuiltinRegistry()
	want := []string{
		"init", "query_route", "semantic_cache_lookup", "query_rewrite", "multi_query",
		"hybrid_retrieve", "ark_rerank", "context_pack", "prompt_prefix_cache",
		"ark_generate", "semantic_cache_write",
	}
	for _, typeID := range want {
		definition, ok := registry.Lookup(typeID)
		if !ok {
			t.Errorf("BuiltinRegistry().Lookup(%q) = false", typeID)
			continue
		}
		if definition.Type != typeID || definition.SchemaVersion == 0 {
			t.Errorf("definition %q = %#v", typeID, definition)
		}
		if len(definition.ConfigSchema) == 0 {
			t.Errorf("definition %q has no configuration schema", typeID)
		}
	}
	if definition, _ := registry.Lookup("init"); !definition.Singleton || !definition.Entry {
		t.Fatalf("init constraints = %#v, want singleton entry", definition)
	}
	if definition, _ := registry.Lookup("ark_generate"); !definition.ProducesAnswer {
		t.Fatalf("ark_generate constraints = %#v, want answer producer", definition)
	}
}

func TestRegistryDefinitionsAreSortedByStableTypeID(t *testing.T) {
	registry := MustRegistry(NodeDefinition{Type: "zeta"}, NodeDefinition{Type: "alpha"})
	definitions := registry.Definitions()
	if len(definitions) != 2 || definitions[0].Type != "alpha" || definitions[1].Type != "zeta" {
		t.Fatalf("Definitions() = %#v, want alpha then zeta", definitions)
	}
}

func TestNewRegistryRejectsDuplicateTypeIDs(t *testing.T) {
	_, err := NewRegistry(NodeDefinition{Type: "duplicate"}, NodeDefinition{Type: "duplicate"})
	if err == nil {
		t.Fatal("NewRegistry() error = nil, want duplicate type error")
	}
}
