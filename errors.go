package orag

import (
	"context"
	"errors"
	"fmt"

	"github.com/shikanon/orag/internal/platform/apperrors"
)

// Code is a stable SDK error category.
type Code string

const (
	CodeInvalidArgument Code = "invalid_argument"
	CodeUnauthorized    Code = "unauthorized"
	CodeForbidden       Code = "forbidden"
	CodeNotFound        Code = "not_found"
	CodeConflict        Code = "conflict"
	CodeUnavailable     Code = "unavailable"
	CodeDeadline        Code = "deadline"
	CodeCanceled        Code = "canceled"
	CodeInternal        Code = "internal"
)

var (
	ErrInvalidArgument = errors.New("orag: invalid argument")
	ErrUnauthorized    = errors.New("orag: unauthorized")
	ErrForbidden       = errors.New("orag: forbidden")
	ErrNotFound        = errors.New("orag: not found")
	ErrConflict        = errors.New("orag: conflict")
	ErrUnavailable     = errors.New("orag: unavailable")
	ErrDeadline        = errors.New("orag: deadline exceeded")
	ErrCanceled        = errors.New("orag: canceled")
	ErrInternal        = errors.New("orag: internal error")
	errClientClosed    = errors.New("client is closed")
)

// Error contains stable SDK error metadata and the original cause.
type Error struct {
	Code      Code
	Operation string
	Resource  string
	TraceID   string
	Retryable bool
	Err       error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Err == nil {
		return fmt.Sprintf("orag %s: %s", e.Operation, e.Code)
	}
	return fmt.Sprintf("orag %s: %s: %v", e.Operation, e.Code, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func (e *Error) Is(target error) bool { return target == sentinelForCode(e.Code) }

func wrapError(operation, resource, traceID string, err error) error {
	if err == nil {
		return nil
	}
	code := CodeInternal
	retryable := false
	switch {
	case errors.Is(err, context.Canceled):
		code = CodeCanceled
	case errors.Is(err, context.DeadlineExceeded):
		code = CodeDeadline
	case apperrors.IsCode(err, apperrors.CodeValidation):
		code = CodeInvalidArgument
	case apperrors.IsCode(err, apperrors.CodeUnauthorized):
		code = CodeUnauthorized
	case apperrors.IsCode(err, apperrors.CodeForbidden):
		code = CodeForbidden
	case apperrors.IsCode(err, apperrors.CodeNotFound):
		code = CodeNotFound
	case apperrors.IsCode(err, apperrors.CodeConflict):
		code = CodeConflict
	case apperrors.IsCode(err, apperrors.CodeUpstreamUnavailable):
		code = CodeUnavailable
		retryable = true
	}
	return &Error{Code: code, Operation: operation, Resource: resource, TraceID: traceID, Retryable: retryable, Err: err}
}

func sentinelForCode(code Code) error {
	switch code {
	case CodeInvalidArgument:
		return ErrInvalidArgument
	case CodeUnauthorized:
		return ErrUnauthorized
	case CodeForbidden:
		return ErrForbidden
	case CodeNotFound:
		return ErrNotFound
	case CodeConflict:
		return ErrConflict
	case CodeUnavailable:
		return ErrUnavailable
	case CodeDeadline:
		return ErrDeadline
	case CodeCanceled:
		return ErrCanceled
	default:
		return ErrInternal
	}
}
