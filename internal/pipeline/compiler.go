package pipeline

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/compose"
	"github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/rag"
)

// Compiler turns a validated pipeline definition into a runnable graph. Node
// implementations are always selected from this server-owned allow-list.
type Compiler struct {
	Registry Registry
	Service  *rag.Service
}

func NewCompiler(service *rag.Service, registry Registry) Compiler {
	return Compiler{Service: service, Registry: registry}
}

func (c Compiler) Compile(ctx context.Context, definition Definition) (compose.Runnable[graph.State, graph.State], error) {
	if errors := Validate(definition, c.Registry); len(errors) > 0 {
		return nil, ValidationErrors(errors)
	}
	if c.Service == nil {
		return nil, fmt.Errorf("pipeline compiler requires a RAG service")
	}
	nodes := graph.NodeSet{Service: c.Service}
	functions := map[string]func(context.Context, graph.State) (graph.State, error){
		"init": nodes.Init, "query_route": nodes.QueryRoute,
		"semantic_cache_lookup": nodes.SemanticCacheLookup, "query_rewrite": nodes.QueryRewrite,
		"multi_query": nodes.MultiQuery, "hybrid_retrieve": nodes.HybridRetrieve,
		"ark_rerank": nodes.Rerank, "context_pack": nodes.ContextPack,
		"prompt_prefix_cache": nodes.PromptPrefixCache, "ark_generate": nodes.Generate,
		"semantic_cache_write": nodes.SemanticCacheWrite,
	}
	builder := compose.NewGraph[graph.State, graph.State]()
	incoming := make(map[string]bool, len(definition.Nodes))
	outgoing := make(map[string]bool, len(definition.Nodes))
	for _, node := range definition.Nodes {
		fn, ok := functions[node.Type]
		if !ok {
			return nil, fmt.Errorf("pipeline node type %q has no runtime implementation", node.Type)
		}
		if err := builder.AddLambdaNode(node.ID, compose.InvokableLambda(fn)); err != nil {
			return nil, err
		}
	}
	for _, edge := range definition.Edges {
		if err := builder.AddEdge(edge.SourceNodeID, edge.TargetNodeID); err != nil {
			return nil, err
		}
		incoming[edge.TargetNodeID] = true
		outgoing[edge.SourceNodeID] = true
	}
	for _, node := range definition.Nodes {
		if !incoming[node.ID] {
			if err := builder.AddEdge(compose.START, node.ID); err != nil {
				return nil, err
			}
		}
		if !outgoing[node.ID] {
			if err := builder.AddEdge(node.ID, compose.END); err != nil {
				return nil, err
			}
		}
	}
	return builder.Compile(ctx, compose.WithGraphName("orag_pipeline_definition"))
}
