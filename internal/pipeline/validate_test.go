package pipeline

import "testing"

func TestValidateReportsUnknownNodeType(t *testing.T) {
	errors := Validate(Definition{Nodes: []Node{{ID: "mystery", Type: "unknown"}}}, BuiltinRegistry())
	assertValidationError(t, errors, ValidationError{Code: "unknown_node_type", NodeID: "mystery"})
}

func TestValidateReportsIncompatiblePorts(t *testing.T) {
	registry := NewRegistry(
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
	registry := NewRegistry(NodeDefinition{Type: "entry", Singleton: true})
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

func TestValidateReportsMissingAnswerTerminal(t *testing.T) {
	registry := validationRegistry()
	errors := Validate(Definition{Nodes: []Node{{ID: "entry", Type: "entry"}}}, registry)
	assertValidationError(t, errors, ValidationError{Code: "missing_answer_terminal"})
}

func TestValidateReportsNonExhaustiveBranches(t *testing.T) {
	registry := NewRegistry(
		NodeDefinition{Type: "entry", Entry: true, Singleton: true, Outputs: []PortDefinition{{Name: "out", Type: "flow", MaxConnections: 1}}},
		NodeDefinition{Type: "branch", Inputs: []PortDefinition{{Name: "in", Type: "flow", MaxConnections: 1}}, Outputs: []PortDefinition{
			{Name: "yes", Type: "flow", Required: true, MaxConnections: 1},
			{Name: "no", Type: "flow", Required: true, MaxConnections: 1},
		}},
		NodeDefinition{Type: "answer", Terminal: true, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}},
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
	registry := NewRegistry(
		NodeDefinition{Type: "entry", Entry: true, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}},
		NodeDefinition{Type: "answer", Terminal: true, Inputs: []PortDefinition{{Name: "in", Type: "flow", Required: true}}},
	)
	errors := Validate(Definition{Nodes: []Node{{ID: "entry", Type: "entry"}, {ID: "answer", Type: "answer"}}}, registry)
	assertValidationError(t, errors, ValidationError{Code: "missing_required_input", NodeID: "answer", Port: "in"})
}

func TestValidateReportsCycleWhenNodeTypesDoNotAllowIt(t *testing.T) {
	registry := NewRegistry(
		NodeDefinition{Type: "entry", Entry: true, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}},
		NodeDefinition{Type: "answer", Terminal: true, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}},
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

func validationRegistry() Registry {
	return NewRegistry(
		NodeDefinition{Type: "entry", Entry: true, Singleton: true, Outputs: []PortDefinition{{Name: "out", Type: "flow", MaxConnections: 1}}},
		NodeDefinition{Type: "step", Inputs: []PortDefinition{{Name: "in", Type: "flow"}}, Outputs: []PortDefinition{{Name: "out", Type: "flow"}}},
		NodeDefinition{Type: "answer", Terminal: true, Inputs: []PortDefinition{{Name: "in", Type: "flow"}}},
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
