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
	server, err := mcp.NewServer(tools, mcp.RuntimeConfigFromEnv(), nil)
	if err != nil {
		log.Fatalf("init MCP server: %v", err)
	}
	if err := server.Serve(ctx, os.Stdin, os.Stdout); err != nil {
		log.Fatalf("serve MCP stdio: %v", err)
	}
}
