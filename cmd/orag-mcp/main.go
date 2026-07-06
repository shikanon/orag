package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/shikanon/orag/internal/mcp"
)

func main() {
	fs := flag.NewFlagSet("orag-mcp", flag.ExitOnError)
	openAPIPath := fs.String("openapi", "api/openapi.yaml", "path to ORAG OpenAPI contract")
	_ = fs.Parse(os.Args[1:])

	ctx := context.Background()
	tools, err := mcp.LoadToolsFromOpenAPI(ctx, *openAPIPath)
	if err != nil {
		log.Fatalf("load MCP tools: %v", err)
	}
	for _, artifact := range []string{
		"agent/mcp/tools/orag-self-check.json",
		"agent/mcp/tools/orag-self-diagnose.json",
		"agent/mcp/tools/orag-self-ops.json",
	} {
		if _, err := os.Stat(artifact); err != nil {
			continue
		}
		artifactTools, err := mcp.LoadToolsFromArtifacts(artifact)
		if err != nil {
			log.Fatalf("load generated MCP tools %s: %v", artifact, err)
		}
		tools = append(tools, artifactTools...)
	}
	server, err := mcp.NewServer(tools, mcp.RuntimeConfigFromEnv(), nil)
	if err != nil {
		log.Fatalf("init MCP server: %v", err)
	}
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("serve MCP stdio: %v", err)
	}
}
