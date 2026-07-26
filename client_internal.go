package orag

import "strings"

func (c *Client) tenant(value string) string {
	if strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return c.tenantID
}

func (c *Client) requireOpen(operation string) error {
	if c == nil || c.app == nil {
		return newError(CodeUnavailable, operation, "", "", false, errClientClosed)
	}
	if c.closed.Load() {
		return newError(CodeUnavailable, operation, "", "", false, errClientClosed)
	}
	return nil
}
