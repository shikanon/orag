package agentsync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shikanon/orag/internal/agentskills"
	"github.com/shikanon/orag/internal/mcp"
)

const mcpToolsPath = ".mcp/tools/ralph-loop.json"

// GeneratedFile is a deterministic artifact rendered from the OpenAPI capability manifest.
type GeneratedFile struct {
	Target  string
	Path    string
	Content string
}

// GenerateFromOpenAPI renders MCP tool schema and all agent Skill artifacts from OpenAPI.
func GenerateFromOpenAPI(ctx context.Context, openAPIPath string) ([]GeneratedFile, error) {
	mcpSchema, err := renderMCPTools(ctx, openAPIPath)
	if err != nil {
		return nil, err
	}
	files := []GeneratedFile{{
		Target:  "mcp",
		Path:    mcpToolsPath,
		Content: mcpSchema,
	}}

	skillFiles, err := agentskills.GenerateFromOpenAPI(openAPIPath)
	if err != nil {
		return nil, err
	}
	for _, file := range skillFiles {
		files = append(files, GeneratedFile{
			Target:  file.Target,
			Path:    file.Path,
			Content: file.Content,
		})
	}
	return files, nil
}

// WriteFiles writes generated artifacts below outputDir.
func WriteFiles(outputDir string, files []GeneratedFile) error {
	for _, file := range files {
		targetPath := filepath.Join(outputDir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(targetPath, []byte(file.Content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// CheckFiles verifies generated artifacts match files on disk without modifying them.
func CheckFiles(outputDir string, files []GeneratedFile) error {
	var drifted []string
	for _, file := range files {
		targetPath := filepath.Join(outputDir, filepath.FromSlash(file.Path))
		body, err := os.ReadFile(targetPath)
		if err != nil {
			drifted = append(drifted, fmt.Sprintf("%s: %v", file.Path, err))
			continue
		}
		if string(body) != file.Content {
			drifted = append(drifted, fmt.Sprintf("%s: generated content differs", file.Path))
		}
	}
	if len(drifted) > 0 {
		return DriftError{Files: drifted}
	}
	return nil
}

// DriftError reports files that are missing or no longer match generated output.
type DriftError struct {
	Files []string
}

func (e DriftError) Error() string {
	return fmt.Sprintf("agent artifacts are out of sync: %v", e.Files)
}

type mcpToolsDocument struct {
	GeneratedFrom     string               `json:"generated_from"`
	ManifestExtension string               `json:"manifest_extension"`
	ProtocolVersion   string               `json:"protocol_version"`
	Tools             []mcp.ToolDefinition `json:"tools"`
}

func renderMCPTools(ctx context.Context, openAPIPath string) (string, error) {
	tools, err := mcp.LoadToolsFromOpenAPI(ctx, openAPIPath)
	if err != nil {
		return "", err
	}
	publicTools := make([]mcp.ToolDefinition, 0, len(tools))
	for _, tool := range tools {
		publicTools = append(publicTools, mcp.ToolDefinition{
			Name:         tool.Name,
			Description:  tool.Description,
			InputSchema:  tool.InputSchema,
			OutputSchema: tool.OutputSchema,
			Annotations:  tool.Annotations,
		})
	}
	doc := mcpToolsDocument{
		GeneratedFrom:     filepath.ToSlash(openAPIPath),
		ManifestExtension: "x-orag-agent-capabilities",
		ProtocolVersion:   mcp.ProtocolVersion,
		Tools:             publicTools,
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return "", err
	}
	return buf.String(), nil
}
