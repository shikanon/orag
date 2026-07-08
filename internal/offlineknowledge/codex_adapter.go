package offlineknowledge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"time"
)

var (
	ErrCodexUnavailable = errors.New("offline knowledge codex analyzer is unavailable")
	ErrCodexDisabled    = errors.New("offline knowledge codex analyzer is disabled")
	ErrCodexInvalidJSON = errors.New("codex response invalid json")
)

type CodexRunnerConfig struct {
	Enabled    bool
	Command    []string
	Endpoint   string
	Headers    map[string]string
	Timeout    time.Duration
	HTTPClient *http.Client
}

type CodexRunnerAdapter struct {
	config CodexRunnerConfig
}

func NewCodexRunnerAdapter(config CodexRunnerConfig) CodexRunnerAdapter {
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	return CodexRunnerAdapter{config: config}
}

func (a CodexRunnerAdapter) AnalyzeCodex(ctx context.Context, request CodexAnalyzeRequest) (CodexAnalyzeResponse, error) {
	if !a.config.Enabled {
		return CodexAnalyzeResponse{}, ErrCodexDisabled
	}
	if len(a.config.Command) == 0 && a.config.Endpoint == "" {
		return CodexAnalyzeResponse{}, ErrCodexUnavailable
	}
	if a.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.config.Timeout)
		defer cancel()
	}

	payload, err := json.Marshal(request)
	if err != nil {
		return CodexAnalyzeResponse{}, err
	}
	var raw []byte
	if len(a.config.Command) > 0 {
		raw, err = a.runCommand(ctx, payload)
	} else {
		raw, err = a.callEndpoint(ctx, payload)
	}
	if err != nil {
		return CodexAnalyzeResponse{}, err
	}
	response, err := decodeCodexAnalyzeResponse(raw)
	if err != nil {
		return CodexAnalyzeResponse{}, err
	}
	if err := ValidateCodexResponse(response, request.Constraints.Quota); err != nil {
		return CodexAnalyzeResponse{}, err
	}
	return response, nil
}

func (a CodexRunnerAdapter) runCommand(ctx context.Context, payload []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, a.config.Command[0], a.config.Command[1:]...)
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return nil, fmt.Errorf("%w: command failed: %s", ErrCodexUnavailable, stderr.String())
		}
		return nil, fmt.Errorf("%w: command failed: %v", ErrCodexUnavailable, err)
	}
	return out, nil
}

func (a CodexRunnerAdapter) callEndpoint(ctx context.Context, payload []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.config.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	for key, value := range a.config.Headers {
		req.Header.Set(key, value)
	}
	resp, err := a.config.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: endpoint request failed: %v", ErrCodexUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%w: endpoint status %d", ErrCodexUnavailable, resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeCodexAnalyzeResponse(raw []byte) (CodexAnalyzeResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response CodexAnalyzeResponse
	if err := decoder.Decode(&response); err != nil {
		return CodexAnalyzeResponse{}, fmt.Errorf("%w: %v", ErrCodexInvalidJSON, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return CodexAnalyzeResponse{}, ErrCodexInvalidJSON
	}
	return response, nil
}
