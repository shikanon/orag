package pipeline

import "encoding/json"

// PortDefinition describes one typed connection point exposed by a node type.
type PortDefinition struct {
	Name           string `json:"name"`
	Type           string `json:"type"`
	Required       bool   `json:"required,omitempty"`
	MaxConnections int    `json:"max_connections,omitempty"`
}

// NodeDefinition is server-owned metadata for a node type that may appear in a pipeline.
type NodeDefinition struct {
	Type           string           `json:"type"`
	DisplayName    string           `json:"display_name,omitempty"`
	Category       string           `json:"category,omitempty"`
	Description    string           `json:"description,omitempty"`
	SchemaVersion  int              `json:"schema_version"`
	ConfigSchema   json.RawMessage  `json:"config_schema"`
	DefaultConfig  json.RawMessage  `json:"default_config,omitempty"`
	Inputs         []PortDefinition `json:"inputs,omitempty"`
	Outputs        []PortDefinition `json:"outputs,omitempty"`
	Singleton      bool             `json:"singleton,omitempty"`
	Entry          bool             `json:"entry,omitempty"`
	ProducesAnswer bool             `json:"produces_answer,omitempty"`
	AllowsCycles   bool             `json:"allows_cycles,omitempty"`
	AllowedTargets []string         `json:"allowed_targets,omitempty"`
}

type Definition struct {
	Nodes []Node `json:"nodes"`
	Edges []Edge `json:"edges"`
}

type Node struct {
	ID            string          `json:"id"`
	Type          string          `json:"type"`
	SchemaVersion int             `json:"schema_version,omitempty"`
	Config        json.RawMessage `json:"config,omitempty"`
}

type Edge struct {
	ID           string `json:"id"`
	SourceNodeID string `json:"source_node_id"`
	SourcePort   string `json:"source_port"`
	TargetNodeID string `json:"target_node_id"`
	TargetPort   string `json:"target_port"`
}

// ValidationError identifies both the problem and its graph address.
type ValidationError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	NodeID  string `json:"node_id,omitempty"`
	EdgeID  string `json:"edge_id,omitempty"`
	Port    string `json:"port,omitempty"`
}
