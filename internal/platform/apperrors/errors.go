package apperrors

import (
	"errors"
	"fmt"
)

type Code string

const (
	CodeValidation          Code = "validation"
	CodeUnauthorized        Code = "unauthorized"
	CodeForbidden           Code = "forbidden"
	CodeNotFound            Code = "not_found"
	CodeConflict            Code = "conflict"
	CodeRateLimited         Code = "rate_limited"
	CodeUpstreamUnavailable Code = "upstream_unavailable"
	CodeInternal            Code = "internal"
)

type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *Error) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("%s: %s", e.Code, e.Message)
	}
	return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
}

func (e *Error) Unwrap() error {
	return e.Err
}

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(code Code, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

func IsCode(err error, code Code) bool {
	var appErr *Error
	if errors.As(err, &appErr) {
		return appErr.Code == code
	}
	return false
}
