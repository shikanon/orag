package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudwego/eino/compose"
	"github.com/shikanon/orag/internal/graph"
	"github.com/shikanon/orag/internal/rag"
	"github.com/shikanon/orag/internal/release"
)

var (
	ErrProjectRequired              = errors.New("project is required for versioned query execution")
	ErrProductionVersionUnavailable = errors.New("production has no active pipeline version")
	ErrFrozenVersionInvalid         = errors.New("active pipeline version has an invalid frozen definition")
)

// CompiledExecutor is implemented by graph.RAGGraph. Keeping it small makes
// the release resolver independently testable and prevents a pipeline package
// dependency on application wiring.
type CompiledExecutor interface {
	InvokeCompiled(context.Context, compose.Runnable[graph.State, graph.State], rag.QueryRequest) (rag.QueryResponse, error)
}

// ProductionRunner resolves the project's production environment on every
// query and runs only its immutable, evaluated pipeline version. Query callers
// never supply the project version, release, or executable definition.
type ProductionRunner struct {
	Release  *release.Service
	Compiler Compiler
	Executor CompiledExecutor
}

func (r ProductionRunner) Query(ctx context.Context, request rag.QueryRequest) (rag.QueryResponse, error) {
	projectID := strings.TrimSpace(request.ProjectID)
	if projectID == "" {
		return rag.QueryResponse{}, ErrProjectRequired
	}
	if r.Release == nil || r.Executor == nil {
		return rag.QueryResponse{}, fmt.Errorf("production runner is not configured")
	}
	environment, err := r.Release.Environment(ctx, projectID, release.Production)
	if err != nil {
		return rag.QueryResponse{}, err
	}
	if strings.TrimSpace(environment.ActiveVersionID) == "" {
		return rag.QueryResponse{}, ErrProductionVersionUnavailable
	}
	version, err := r.Release.Version(ctx, projectID, environment.ActiveVersionID)
	if err != nil {
		return rag.QueryResponse{}, err
	}
	if strings.TrimSpace(version.PipelineID) == "" || len(version.Definition) == 0 {
		return rag.QueryResponse{}, fmt.Errorf("%w: active version %q lacks a pipeline snapshot", ErrFrozenVersionInvalid, version.ID)
	}
	var definition Definition
	if err := json.Unmarshal(version.Definition, &definition); err != nil {
		return rag.QueryResponse{}, fmt.Errorf("%w: %v", ErrFrozenVersionInvalid, err)
	}
	runnable, err := r.Compiler.Compile(ctx, definition)
	if err != nil {
		return rag.QueryResponse{}, fmt.Errorf("%w: %v", ErrFrozenVersionInvalid, err)
	}
	request.ProjectID = projectID
	request.PipelineID = version.PipelineID
	request.PipelineVersionID = version.ID
	request.ReleaseID = environment.ActiveReleaseID
	request.Environment = string(release.Production)
	evidence, err := r.Release.Evidence(ctx, projectID, version.ID, release.Production)
	if err != nil {
		return rag.QueryResponse{}, err
	}
	request.DatasetID = evidence.DatasetID
	request.EvaluationRunID = evidence.EvaluationRunID
	return r.Executor.InvokeCompiled(ctx, runnable, request)
}
