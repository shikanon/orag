package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	core "github.com/shikanon/orag/internal/app"
	"github.com/shikanon/orag/internal/ingest"
	"github.com/shikanon/orag/internal/taskqueue"
)

// commandEnvelope is the stable machine-readable contract for governance
// commands. Success and error output always preserve the same top-level keys.
type commandEnvelope struct {
	OK      bool   `json:"ok"`
	Command string `json:"command"`
	Data    any    `json:"data,omitempty"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func writeCommandEnvelope(out io.Writer, command string, data any) error {
	return json.NewEncoder(out).Encode(commandEnvelope{OK: true, Command: command, Data: data})
}

func governanceSchemaCmd(out io.Writer) error {
	return writeCommandEnvelope(out, "schema", map[string]any{
		"schema_version": "oragctl.governance.v1",
		"commands": []map[string]any{
			{"name": "doctor", "format": "json-envelope", "side_effects": false},
			{"name": "schema", "format": "json-envelope", "side_effects": false},
			{"name": "task wait", "format": "json-envelope", "side_effects": false, "required_flags": []string{"--id"}},
			{"name": "doc wait", "format": "json-envelope", "side_effects": false, "required_flags": []string{"--id"}},
		},
		"exit_codes": map[string]int{"ok": 0, "invalid_request": 2, "not_found": 3, "operation_failed": 4},
	})
}

func doctorCmd(ctx context.Context, app *core.App, out io.Writer) error {
	checks, ready := app.Readiness(ctx)
	governance := map[string]string{
		"task_queue":      availability(app.TaskQueue != nil),
		"task_worker":     availability(app.TaskWorker != nil),
		"audit":           availability(app.Audit != nil),
		"model_readiness": availability(app.ModelReadiness != nil),
	}
	return writeCommandEnvelope(out, "doctor", map[string]any{"ready": ready, "checks": checks, "governance": governance})
}

func availability(ok bool) string {
	if ok {
		return "configured"
	}
	return "unavailable"
}

func taskWaitCmd(ctx context.Context, app *core.App, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("task wait", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("id", "", "task id")
	tenantID := fs.String("tenant-id", "tenant_default", "tenant id")
	timeout := fs.Duration("timeout", 10*time.Minute, "maximum wait duration")
	poll := fs.Duration("poll-interval", 500*time.Millisecond, "task polling interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return fmt.Errorf("--id is required")
	}
	if app.TaskQueue == nil {
		return fmt.Errorf("task queue is unavailable")
	}
	if *timeout <= 0 || *poll <= 0 {
		return fmt.Errorf("timeout and poll-interval must be positive")
	}

	waitCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	ticker := time.NewTicker(*poll)
	defer ticker.Stop()
	for {
		task, found, err := app.TaskQueue.Get(waitCtx, *tenantID, *id)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("task %q not found", *id)
		}
		if isTerminalTaskStatus(task.Status) {
			return writeCommandEnvelope(out, "task wait", map[string]any{"task": task, "terminal": true})
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wait for task %q: %w", *id, waitCtx.Err())
		case <-ticker.C:
		}
	}
}

func isTerminalTaskStatus(status taskqueue.TaskStatus) bool {
	switch status {
	case taskqueue.TaskStatusSucceeded, taskqueue.TaskStatusCancelled, taskqueue.TaskStatusFailedTerminal, taskqueue.TaskStatusDeadLetter:
		return true
	default:
		return false
	}
}

func docWaitCmd(ctx context.Context, app *core.App, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("doc wait", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	id := fs.String("id", "", "ingestion job id")
	tenantID := fs.String("tenant-id", "tenant_default", "tenant id")
	timeout := fs.Duration("timeout", 10*time.Minute, "maximum wait duration")
	poll := fs.Duration("poll-interval", 500*time.Millisecond, "job polling interval")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*id) == "" {
		return fmt.Errorf("--id is required")
	}
	if app.Ingest == nil || app.Ingest.Jobs == nil {
		return fmt.Errorf("ingestion job store is unavailable")
	}
	if *timeout <= 0 || *poll <= 0 {
		return fmt.Errorf("timeout and poll-interval must be positive")
	}

	waitCtx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	ticker := time.NewTicker(*poll)
	defer ticker.Stop()
	for {
		job, found, err := app.Ingest.Jobs.GetJob(waitCtx, *tenantID, *id)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("ingestion job %q not found", *id)
		}
		if job.Status == ingest.JobStatusSucceeded || job.Status == ingest.JobStatusFailed {
			return writeCommandEnvelope(out, "doc wait", map[string]any{"job": job, "terminal": true})
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("wait for ingestion job %q: %w", *id, waitCtx.Err())
		case <-ticker.C:
		}
	}
}
