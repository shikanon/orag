package http

import (
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/shikanon/orag/internal/observability"
)

type ErrorResponse struct {
	Error ErrorBody `json:"error"`
}

type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TraceID string `json:"trace_id"`
}

func writeError(c *app.RequestContext, status int, code, message string) {
	traceID := requestTraceID(c)
	c.Set("error_code", code)
	c.JSON(status, ErrorResponse{Error: ErrorBody{
		Code:    code,
		Message: message,
		TraceID: traceID,
	}})
}

func requestTraceID(c *app.RequestContext) string {
	if v, ok := c.Get("trace_id"); ok {
		if traceID, ok := v.(string); ok && traceID != "" {
			return traceID
		}
	}
	traceID := strings.TrimSpace(string(c.GetHeader(observability.TraceIDHeader)))
	if traceID == "" {
		traceID = observability.NewTraceID()
	}
	c.Set("trace_id", traceID)
	c.Header(observability.TraceIDHeader, traceID)
	return traceID
}

func statusCodeName(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	default:
		return "internal_error"
	}
}
