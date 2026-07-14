// Package orag provides the public embeddable Go SDK for ORAG.
//
// The SDK and the ORAG HTTP service share the same application services. Use
// MockConfig for deterministic, dependency-free examples and tests, or provide
// explicit PostgreSQL, Qdrant, and model-provider configuration for a real
// deployment. Public signatures never expose packages below internal/.
package orag

import (
	"context"
	"sync"
	"sync/atomic"

	core "github.com/shikanon/orag/internal/app"
	internalconfig "github.com/shikanon/orag/internal/config"
	"github.com/shikanon/orag/internal/platform/logger"
)

const defaultTenantID = "tenant_default"

// Client owns an embedded ORAG application and its external resources.
// Client methods are safe for concurrent use unless a method documents
// otherwise.
type Client struct {
	app      *core.App
	tenantID string

	closeOnce sync.Once
	closeErr  error
	closed    atomic.Bool
}

// New creates an embedded ORAG client from explicit configuration. It does not
// read ambient environment variables.
func New(ctx context.Context, cfg Config) (*Client, error) {
	internalCfg, err := cfg.internal()
	if err != nil {
		return nil, wrapError("new", "", "", err)
	}
	logg := cfg.Logger
	if logg == nil {
		logg = logger.New(internalCfg.Server.Debug)
	}
	application, err := core.New(ctx, internalCfg, logg)
	if err != nil {
		return nil, wrapError("new", "", "", err)
	}
	tenantID := cfg.TenantID
	if tenantID == "" {
		tenantID = defaultTenantID
	}
	return &Client{app: application, tenantID: tenantID}, nil
}

// NewFromEnv creates an embedded client using the same environment-based
// configuration loader as cmd/orag-api.
func NewFromEnv(ctx context.Context) (*Client, error) {
	cfg, err := internalconfig.Load()
	if err != nil {
		return nil, wrapError("new_from_env", "", "", err)
	}
	application, err := core.New(ctx, cfg, logger.New(cfg.Server.Debug))
	if err != nil {
		return nil, wrapError("new_from_env", "", "", err)
	}
	return &Client{app: application, tenantID: defaultTenantID}, nil
}

// Close releases resources owned by the client. It is nil-safe and
// idempotent.
func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		if c.app != nil {
			c.closeErr = c.app.Close()
		}
	})
	return c.closeErr
}

// ReadinessCheck describes one embedded dependency check.
type ReadinessCheck struct {
	Status string
	Error  string
}

// Readiness is the aggregate embedded application readiness state.
type Readiness struct {
	Ready  bool
	Checks map[string]ReadinessCheck
}

// Readiness checks configured storage and model dependencies.
func (c *Client) Readiness(ctx context.Context) (Readiness, error) {
	if err := c.requireOpen("readiness"); err != nil {
		return Readiness{}, err
	}
	checks, ready := c.app.Readiness(ctx)
	result := Readiness{Ready: ready, Checks: make(map[string]ReadinessCheck, len(checks))}
	for name, check := range checks {
		result.Checks[name] = ReadinessCheck{Status: check.Status, Error: check.Error}
	}
	return result, nil
}
