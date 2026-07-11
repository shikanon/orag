package pipeline

import (
	"reflect"
	"testing"
)

func TestValidateReportsUnknownNodeType(t *testing.T) {
	errors := Validate(Definition{Nodes: []Node{{ID: "mystery", Type: "unknown"}}}, BuiltinRegistry())
	assertValidationError(t, errors, ValidationError{Code: "unknown_node_type", NodeID: "mystery"})
}

func TestValidateReportsIncompatiblePorts(t *testing.T) {
	registry := MustRegistry(
		NodeDefinition{Type: "source", Outputs: []PortDefinition{{Name: "out", Type: "documents"}}},
		NodeDefinition{Type: "sink", Inputs: []PortDefinition{{Name: "in", Type: "query"}}},
	)
	errors := Validate(Definition{
		Nodes: []Node{{ID: "source", Type: "source"}, {ID: "sink", Type: "sink"}},
		Edges: []Edge{{ID: "edge-1", SourceNodeID: "source", SourcePort: "out", TargetNodeID: "sink", TargetPort: "in"}},
	}, registry)
	assertValidationError(t, errors, ValidationError{Code: "incompatible_port_types", EdgeID: "edge-1"})
}

func TestValidateReportsDuplicateSingleton(t *testing.T) {
	registry := MustRegistry(NodeDefinition{Type: "entry", Singleton: true})
	errors := Validate(Definition{Nodes: []Node{{ID: "a", Type: "entry"}, {ID: "b", Type: "entry"}}}, registry)
	assertValidationError(t, errors, ValidationError{Code: "duplicate_singleton", NodeID: "b"})
}

func TestValidateReportsUnreachableNode(t *testing.T) {
	registry := validationRegistry()
	errors := Validate(Definition{
		Nodes: []Node{{ID: "entry", Type: "entry"}, {ID: "answer", Type: "answer"}, {ID: "orphan", Type: "step"}},
		Edges: []Edge{{ID: "e1", SourceNodeID: "entry", SourcePort: "out", TargetNodeID: "answer", TargetPort: "in"}},
	}, registry)
	assertValidationError(t, errors, ValidationError{Code: "unreachable_node", NodeID: "orphan"})
}

func TestValidateReportsMissingAnswerProducer(t *testing.T) {
	registry := validationRegistry()
	errors := Validate(Definition{Nodes: []Node{{ID: "entry", Type: "entry"}}}, registry)
	assertValidationError(t, errors, ValidationError{Code: "missing_answer_producer"})
}

func TestValidateReportsNonExhaustiveBranches(t *testing.T) {
	registry := MustRegistry(
		NodeDefinition{Type: "entry", Entry: true, Singleton: true, Outputs: []PortDefinition{{Name: "out", Type: "flow", MaxConnections: 1}}},
		NodeDefinition{Type: "branch", Inputs: []PortDefinition{{Name: "in", Type: "flow", MaxConnections: 1}}, Outputs: []PortDefinition{
			{Name: "yes", Type: "flow", Required: true, MaxConnections: 1},
			{Name: "no", Type: "flow", Required: true, MaxConnections: 1},
		}},
		NodeDefinition{Type: "answer", ProducesAnswer: true, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}},
	)
	errors := Validate(Definition{
		Nodes: []Node{{ID: "entry", Type: "entry"}, {ID: "choice", Type: "branch"}, {ID: "answer", Type: "answer"}},
		Edges: []Edge{
			{ID: "e1", SourceNodeID: "entry", SourcePort: "out", TargetNodeID: "choice", TargetPort: "in"},
			{ID: "e2", SourceNodeID: "choice", SourcePort: "yes", TargetNodeID: "answer", TargetPort: "in"},
		},
	}, registry)
	assertValidationError(t, errors, ValidationError{Code: "non_exhaustive_branch", NodeID: "choice", Port: "no"})
}

func TestValidateReportsMissingRequiredInput(t *testing.T) {
	registry := MustRegistry(
		NodeDefinition{Type: "entry", Entry: true, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}},
		NodeDefinition{Type: "answer", ProducesAnswer: true, Inputs: []PortDefinition{{Name: "in", Type: "flow", Required: true}}},
	)
	errors := Validate(Definition{Nodes: []Node{{ID: "entry", Type: "entry"}, {ID: "answer", Type: "answer"}}}, registry)
	assertValidationError(t, errors, ValidationError{Code: "missing_required_input", NodeID: "answer", Port: "in"})
}

func TestValidateReportsCycleWhenNodeTypesDoNotAllowIt(t *testing.T) {
	registry := MustRegistry(
		NodeDefinition{Type: "entry", Entry: true, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}},
		NodeDefinition{Type: "answer", ProducesAnswer: true, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}},
	)
	errors := Validate(Definition{
		Nodes: []Node{{ID: "entry", Type: "entry"}, {ID: "answer", Type: "answer"}},
		Edges: []Edge{
			{ID: "forward", SourceNodeID: "entry", SourcePort: "out", TargetNodeID: "answer", TargetPort: "in"},
			{ID: "back", SourceNodeID: "answer", SourcePort: "out", TargetNodeID: "entry", TargetPort: "in"},
		},
	}, registry)
	assertValidationError(t, errors, ValidationError{Code: "cycle_not_allowed", EdgeID: "forward"})
}

func TestValidateRejectsNonEmptyNodeConfig(t *testing.T) {
	registry := MustRegistry(NodeDefinition{Type: "entry", Entry: true, ProducesAnswer: true})
	for _, config := range []string{`{"unexpected":true}`, `[]`, `{broken`} {
		errors := Validate(Definition{Nodes: []Node{{ID: "entry", Type: "entry", Config: []byte(config)}}}, registry)
		assertValidationError(t, errors, ValidationError{Code: "invalid_node_config", NodeID: "entry"})
	}
}

func TestValidateAcceptsNilAndEmptyObjectConfig(t *testing.T) {
	registry := MustRegistry(NodeDefinition{Type: "entry", Entry: true, ProducesAnswer: true})
	for _, config := range [][]byte{nil, {}, []byte(`{}`), []byte(`{ }`)} {
		if errors := Validate(Definition{Nodes: []Node{{ID: "entry", Type: "entry", Config: config}}}, registry); len(errors) != 0 {
			t.Fatalf("Validate(config=%q) = %#v, want no errors", config, errors)
		}
	}
}

func TestValidateErrorOrderIsStableAndExact(t *testing.T) {
	registry := MustRegistry(
		NodeDefinition{Type: "entry", Entry: true, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}},
		NodeDefinition{Type: "answer", ProducesAnswer: true, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}},
		NodeDefinition{Type: "step", Inputs: []PortDefinition{{Name: "in", Type: "flow", Required: true}}, Outputs: []PortDefinition{{Name: "out", Type: "flow", Required: true}}},
	)
	definition := Definition{
		Nodes: []Node{{ID: "entry", Type: "entry"}, {ID: "answer", Type: "answer"}, {ID: "zeta", Type: "step"}, {ID: "alpha", Type: "step"}},
		Edges: []Edge{{ID: "valid", SourceNodeID: "entry", SourcePort: "out", TargetNodeID: "answer", TargetPort: "in"}},
	}
	want := []ValidationError{
		{Code: "missing_required_input", Message: `required input "in" is not connected`, NodeID: "alpha", Port: "in"},
		{Code: "non_exhaustive_branch", Message: `required branch "out" is not connected`, NodeID: "alpha", Port: "out"},
		{Code: "missing_required_input", Message: `required input "in" is not connected`, NodeID: "zeta", Port: "in"},
		{Code: "non_exhaustive_branch", Message: `required branch "out" is not connected`, NodeID: "zeta", Port: "out"},
		{Code: "unreachable_node", Message: "node is not reachable from the query entry", NodeID: "zeta"},
		{Code: "unreachable_node", Message: "node is not reachable from the query entry", NodeID: "alpha"},
	}
	for i := 0; i < 50; i++ {
		got := Validate(definition, registry)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("Validate() iteration %d = %#v, want %#v", i, got, want)
		}
	}
}

func validationRegistry() Registry {
	return MustRegistry(
		NodeDefinition{Type: "entry", Entry: true, Singleton: true, Outputs: []PortDefinition{{Name: "out", Type: "flow", MaxConnections: 1}}},
		NodeDefinition{Type: "step", Inputs: []PortDefinition{{Name: "in", Type: "flow"}}, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}},
		NodeDefinition{Type: "answer", ProducesAnswer: true, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}},
	)
}

func assertValidationError(t *testing.T, errors []ValidationError, want ValidationError) {
	t.Helper()
	for _, got := range errors {
		if got.Code == want.Code && got.NodeID == want.NodeID && got.EdgeID == want.EdgeID && got.Port == want.Port {
			return
		}
	}
	t.Fatalf("validation errors = %#v, want %#v", errors, want)
}
