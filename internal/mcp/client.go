package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/shikanon/orag/internal/observability"
	"github.com/shikanon/orag/internal/storage/postgres"
)

const (
	defaultBaseURL = "http://localhost:8080"
	defaultTimeout = 30 * time.Second
)

type RuntimeConfig struct {
	BaseURL  string
	Token    string
	TenantID string
	Timeout  time.Duration
}

func RuntimeConfigFromEnv() RuntimeConfig {
	timeout := defaultTimeout
	if raw := strings.TrimSpace(os.Getenv("ORAG_MCP_TIMEOUT")); raw != "" {
		if parsed, err := time.ParseDuration(raw); err == nil {
			timeout = parsed
		}
	}
	return RuntimeConfig{
		BaseURL:  firstNonEmpty(os.Getenv("ORAG_API_BASE_URL"), defaultBaseURL),
		Token:    strings.TrimSpace(os.Getenv("ORAG_API_TOKEN")),
		TenantID: strings.TrimSpace(os.Getenv("ORAG_TENANT_ID")),
		Timeout:  timeout,
	}
}

type HTTPToolClient struct {
	client *http.Client
}

func NewHTTPToolClient(client *http.Client) *HTTPToolClient {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPToolClient{client: client}
}

type HTTPTraceGetter struct {
	client *http.Client
	cfg    RuntimeConfig
}

func NewHTTPTraceGetter(client *http.Client, cfg RuntimeConfig) *HTTPTraceGetter {
	if client == nil {
		client = http.DefaultClient
	}
	return &HTTPTraceGetter{client: client, cfg: cfg}
}

func (g *HTTPTraceGetter) GetTrace(ctx context.Context, traceID string) (postgres.TraceRecord, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return postgres.TraceRecord{}, false, nil
	}
	if !runtimeConfigAvailable(g.cfg) {
		return postgres.TraceRecord{}, false, errors.New("trace runtime configuration is unavailable")
	}
	endpoint, err := joinURL(g.cfg.BaseURL, "/v1/traces/"+url.PathEscape(traceID))
	if err != nil {
		return postgres.TraceRecord{}, false, err
	}

	timeout := g.cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return postgres.TraceRecord{}, false, err
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(g.cfg.Token))
	req.Header.Set("X-ORAG-Tenant-ID", strings.TrimSpace(g.cfg.TenantID))

	resp, err := g.client.Do(req)
	if err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return postgres.TraceRecord{}, false, context.DeadlineExceeded
		}
		return postgres.TraceRecord{}, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return postgres.TraceRecord{}, false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return postgres.TraceRecord{}, false, fmt.Errorf("trace API returned HTTP %d", resp.StatusCode)
	}

	var record postgres.TraceRecord
	if err := json.NewDecoder(resp.Body).Decode(&record); err != nil {
		return postgres.TraceRecord{}, false, err
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = traceID
	}
	if record.NodeSpans == nil {
		record.NodeSpans = []postgres.TraceNodeSpan{}
	}
	return record, true, nil
}

func (c *HTTPToolClient) CallTool(ctx context.Context, cfg RuntimeConfig, tool ToolDefinition, args map[string]any, meta map[string]any) (ToolResult, error) {
	if err := validateRuntimeConfig(cfg, tool.Capability); err != nil {
		return ToolResult{}, err
	}

	body, err := json.Marshal(args)
	if err != nil {
		return ToolResult{}, newRPCError(codeInvalidParams, "invalid_tool_arguments", "tool arguments are not JSON encodable", map[string]any{"tool": tool.Name})
	}
	endpoint, err := joinURL(cfg.BaseURL, tool.Capability.Path)
	if err != nil {
		return ToolResult{}, newRPCError(codeConfigError, "invalid_base_url", "ORAG_API_BASE_URL is invalid", nil)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(callCtx, tool.Capability.Method, endpoint, bytes.NewReader(body))
	if err != nil {
		return ToolResult{}, newRPCError(codeInternalError, "request_create_failed", "failed to create downstream request", nil)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("X-ORAG-Tenant-ID", cfg.TenantID)
	if traceID := traceIDFromMeta(meta); traceID != "" {
		req.Header.Set(firstNonEmpty(tool.Capability.TraceRequestHeader, observability.TraceIDHeader), traceID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
			return ToolResult{}, newRPCError(codeTimeoutError, "downstream_timeout", "ORAG API request timed out", map[string]any{"tool": tool.Name})
		}
		return ToolResult{}, newRPCError(codeHTTPError, "downstream_unavailable", "ORAG API request failed", map[string]any{"tool": tool.Name})
	}
	defer resp.Body.Close()

	payload, readErr := decodePayload(resp.Body)
	if readErr != nil {
		return ToolResult{}, newRPCError(codeHTTPError, "invalid_downstream_response", "ORAG API returned an invalid JSON response", map[string]any{"status": resp.StatusCode})
	}
	traceID := strings.TrimSpace(resp.Header.Get(firstNonEmpty(tool.Capability.TraceResponseHeader, observability.TraceIDHeader)))
	if traceID == "" {
		traceID = traceIDFromPayload(payload, tool.Capability.TraceResponseField)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ToolResult{}, downstreamError(resp.StatusCode, payload, traceID)
	}
	return ToolResult{Payload: payload, TraceID: traceID, Status: resp.StatusCode}, nil
}

func validateRuntimeConfig(cfg RuntimeConfig, cap Capability) error {
	if !cap.AuthRequired {
		return nil
	}
	missing := make([]string, 0, 3)
	if strings.TrimSpace(cfg.BaseURL) == "" {
		missing = append(missing, "ORAG_API_BASE_URL")
	}
	if strings.TrimSpace(cfg.Token) == "" {
		missing = append(missing, "ORAG_API_TOKEN")
	}
	if strings.TrimSpace(cfg.TenantID) == "" {
		missing = append(missing, "ORAG_TENANT_ID")
	}
	if len(missing) > 0 {
		return newRPCError(codeConfigError, "missing_config", "missing required ORAG MCP configuration", map[string]any{"missing_env": missing})
	}
	return nil
}

func runtimeConfigAvailable(cfg RuntimeConfig) bool {
	return strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.Token) != "" && strings.TrimSpace(cfg.TenantID) != ""
}

func downstreamError(status int, payload any, traceID string) *RPCError {
	code := "downstream_error"
	message := http.StatusText(status)
	if body, ok := payload.(map[string]any); ok {
		if errBody, ok := body["error"].(map[string]any); ok {
			if rawCode, ok := errBody["code"].(string); ok && strings.TrimSpace(rawCode) != "" {
				code = strings.TrimSpace(rawCode)
			}
			if rawMessage, ok := errBody["message"].(string); ok && strings.TrimSpace(rawMessage) != "" {
				message = strings.TrimSpace(rawMessage)
			}
			if traceID == "" {
				traceID, _ = errBody["trace_id"].(string)
			}
		}
	}
	if code == "downstream_error" {
		switch status {
		case http.StatusUnauthorized, http.StatusForbidden:
			code = "downstream_auth_error"
		case http.StatusTooManyRequests:
			code = "downstream_rate_limited"
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			code = "downstream_validation_error"
		case http.StatusGatewayTimeout, http.StatusRequestTimeout:
			code = "downstream_timeout"
		}
	}
	data := map[string]any{"status": status}
	if traceID != "" {
		data["trace_id"] = traceID
	}
	return newRPCError(codeHTTPError, code, message, data)
}

func decodePayload(r io.Reader) (any, error) {
	var payload any
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	return normalizeJSONNumbers(payload), nil
}

func normalizeJSONNumbers(value any) any {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			v[key] = normalizeJSONNumbers(item)
		}
		return v
	case []any:
		for i, item := range v {
			v[i] = normalizeJSONNumbers(item)
		}
		return v
	case json.Number:
		if i, err := v.Int64(); err == nil {
			return i
		}
		if f, err := v.Float64(); err == nil {
			return f
		}
		return v.String()
	default:
		return value
	}
}

func joinURL(base, path string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/" + strings.TrimLeft(path, "/")
	return parsed.String(), nil
}

func traceIDFromMeta(meta map[string]any) string {
	for _, key := range []string{"trace_id", "traceId"} {
		if value, ok := meta[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func traceIDFromPayload(payload any, field string) string {
	field = strings.TrimSpace(field)
	if field == "" {
		return ""
	}
	body, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	value, _ := body[field].(string)
	return strings.TrimSpace(value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
