package pipeline

import (
	"fmt"
	"sort"
)

type graphEdge struct {
	target string
	id     string
}

func Validate(pipeline Definition, registry Registry) []ValidationError {
	var errors []ValidationError
	nodes := make(map[string]Node, len(pipeline.Nodes))
	definitions := make(map[string]NodeDefinition, len(pipeline.Nodes))
	singletons := make(map[string]string)
	entries := make([]string, 0, 1)
	answerTerminals := 0

	for _, node := range pipeline.Nodes {
		nodes[node.ID] = node
		definition, ok := registry.Lookup(node.Type)
		if !ok {
			errors = append(errors, validationError("unknown_node_type", node.ID, "", "", "unknown node type %q", node.Type))
			continue
		}
		definitions[node.ID] = definition
		if node.SchemaVersion != 0 && node.SchemaVersion != definition.SchemaVersion {
			errors = append(errors, validationError("unsupported_schema_version", node.ID, "", "", "node schema version %d is not supported", node.SchemaVersion))
		}
		if definition.Singleton {
			if _, exists := singletons[node.Type]; exists {
				errors = append(errors, validationError("duplicate_singleton", node.ID, "", "", "node type %q may only appear once", node.Type))
			} else {
				singletons[node.Type] = node.ID
			}
		}
		if definition.Entry {
			entries = append(entries, node.ID)
		}
		if definition.Terminal {
			answerTerminals++
		}
	}
	if len(entries) != 1 {
		errors = append(errors, validationError("invalid_query_entry_count", "", "", "", "pipeline must contain exactly one query entry; found %d", len(entries)))
	}
	if answerTerminals == 0 {
		errors = append(errors, validationError("missing_answer_terminal", "", "", "", "pipeline must contain at least one answer terminal"))
	}

	outgoing := make(map[string][]string)
	graphEdges := make(map[string][]graphEdge)
	portCounts := make(map[string]int)
	for _, edge := range pipeline.Edges {
		source, sourceOK := nodes[edge.SourceNodeID]
		target, targetOK := nodes[edge.TargetNodeID]
		if !sourceOK || !targetOK {
			errors = append(errors, validationError("unknown_edge_node", "", edge.ID, "", "edge references an unknown node"))
			continue
		}
		sourceDefinition, sourceKnown := definitions[source.ID]
		targetDefinition, targetKnown := definitions[target.ID]
		if !sourceKnown || !targetKnown {
			continue
		}
		sourcePort, sourcePortOK := findPort(sourceDefinition.Outputs, edge.SourcePort)
		targetPort, targetPortOK := findPort(targetDefinition.Inputs, edge.TargetPort)
		if !sourcePortOK || !targetPortOK {
			errors = append(errors, validationError("unknown_port", "", edge.ID, missingPort(edge, sourcePortOK), "edge references an unknown port"))
			continue
		}
		if sourcePort.Type != targetPort.Type {
			errors = append(errors, validationError("incompatible_port_types", "", edge.ID, "", "port type %q cannot connect to %q", sourcePort.Type, targetPort.Type))
			continue
		}
		if len(sourceDefinition.AllowedTargets) > 0 && !contains(sourceDefinition.AllowedTargets, target.Type) {
			errors = append(errors, validationError("edge_not_allowed", "", edge.ID, "", "node type %q cannot connect to %q", source.Type, target.Type))
			continue
		}
		outgoing[source.ID] = append(outgoing[source.ID], target.ID)
		graphEdges[source.ID] = append(graphEdges[source.ID], graphEdge{target: target.ID, id: edge.ID})
		portCounts[portKey(source.ID, "out", sourcePort.Name)]++
		portCounts[portKey(target.ID, "in", targetPort.Name)]++
	}

	for nodeID, definition := range definitions {
		for _, port := range definition.Inputs {
			count := portCounts[portKey(nodeID, "in", port.Name)]
			if port.Required && count == 0 {
				errors = append(errors, validationError("missing_required_input", nodeID, "", port.Name, "required input %q is not connected", port.Name))
			}
			if port.MaxConnections > 0 && count > port.MaxConnections {
				errors = append(errors, validationError("port_cardinality_exceeded", nodeID, "", port.Name, "input port has %d connections; maximum is %d", count, port.MaxConnections))
			}
		}
		for _, port := range definition.Outputs {
			count := portCounts[portKey(nodeID, "out", port.Name)]
			if port.MaxConnections > 0 && count > port.MaxConnections {
				errors = append(errors, validationError("port_cardinality_exceeded", nodeID, "", port.Name, "output port has %d connections; maximum is %d", count, port.MaxConnections))
			}
			if port.Required && count == 0 {
				errors = append(errors, validationError("non_exhaustive_branch", nodeID, "", port.Name, "required branch %q is not connected", port.Name))
			}
		}
	}

	if len(entries) == 1 {
		reachable := reachableFrom(entries[0], outgoing)
		for _, node := range pipeline.Nodes {
			if _, known := definitions[node.ID]; known && !reachable[node.ID] {
				errors = append(errors, validationError("unreachable_node", node.ID, "", "", "node is not reachable from the query entry"))
			}
		}
	}
	errors = append(errors, validateCycles(graphEdges, definitions)...)
	return errors
}

func findPort(ports []PortDefinition, name string) (PortDefinition, bool) {
	for _, port := range ports {
		if port.Name == name {
			return port, true
		}
	}
	return PortDefinition{}, false
}

func reachableFrom(entry string, outgoing map[string][]string) map[string]bool {
	reachable := map[string]bool{entry: true}
	queue := []string{entry}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, next := range outgoing[current] {
			if !reachable[next] {
				reachable[next] = true
				queue = append(queue, next)
			}
		}
	}
	return reachable
}

func validationError(code, nodeID, edgeID, port, format string, args ...any) ValidationError {
	return ValidationError{Code: code, Message: fmt.Sprintf(format, args...), NodeID: nodeID, EdgeID: edgeID, Port: port}
}

func portKey(nodeID, direction, port string) string {
	return nodeID + "\x00" + direction + "\x00" + port
}

func missingPort(edge Edge, sourceFound bool) string {
	if !sourceFound {
		return edge.SourcePort
	}
	return edge.TargetPort
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func validateCycles(edges map[string][]graphEdge, definitions map[string]NodeDefinition) []ValidationError {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := make(map[string]int, len(definitions))
	stack := make([]string, 0, len(definitions))
	stackIndex := make(map[string]int, len(definitions))
	var errors []ValidationError
	var visit func(string)
	visit = func(nodeID string) {
		state[nodeID] = visiting
		stackIndex[nodeID] = len(stack)
		stack = append(stack, nodeID)
		for _, edge := range edges[nodeID] {
			switch state[edge.target] {
			case unvisited:
				visit(edge.target)
			case visiting:
				allowed := true
				for _, cycleNodeID := range stack[stackIndex[edge.target]:] {
					if !definitions[cycleNodeID].AllowsCycles {
						allowed = false
						break
					}
				}
				if !allowed {
					errors = append(errors, validationError("cycle_not_allowed", "", edge.id, "", "cycle includes a node type that does not allow cycles"))
				}
			}
		}
		stack = stack[:len(stack)-1]
		delete(stackIndex, nodeID)
		state[nodeID] = visited
	}
	nodeIDs := make([]string, 0, len(definitions))
	for nodeID := range definitions {
		nodeIDs = append(nodeIDs, nodeID)
	}
	sort.Strings(nodeIDs)
	for _, nodeID := range nodeIDs {
		if state[nodeID] == unvisited {
			visit(nodeID)
		}
	}
	return errors
}
