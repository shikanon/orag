package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
)

const (
	apiKeyPrefix                 = "orag_sk_"
	defaultLastUsedWriteInterval = 5 * time.Minute
	maxLastUsedThrottleEntries   = 10_000
)

var (
	ErrAPIKeyNotFound = errors.New("api key not found")
	ErrAPIKeyInvalid  = errors.New("invalid api key")
	ErrAPIKeyExpired  = errors.New("api key expired")
	ErrAPIKeyRevoked  = errors.New("api key revoked")
)

type APIKey struct {
	ID         string     `json:"id"`
	TenantID   string     `json:"tenant_id"`
	ProjectID  string     `json:"project_id,omitempty"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	KeyHash    string     `json:"-"`
	Role       Role       `json:"role"`
	CreatedBy  string     `json:"created_by"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
}

type APIKeyCreateInput struct {
	TenantID  string
	ProjectID string
	Name      string
	Role      Role
	CreatedBy string
	ExpiresAt *time.Time
}

type APIKeyCreateResult struct {
	APIKey APIKey `json:"api_key"`
	Secret string `json:"secret"`
}

type APIKeyRepository interface {
	CreateAPIKey(context.Context, APIKey) error
	ListAPIKeys(context.Context, string) ([]APIKey, error)
	GetAPIKeyByID(context.Context, string) (APIKey, bool, error)
	RevokeAPIKey(context.Context, string, string, time.Time) (bool, error)
	TouchAPIKeyLastUsed(context.Context, string, time.Time, time.Time) error
}

type APIKeyService struct {
	repo   APIKeyRepository
	pepper []byte
	now    func() time.Time
	rand   func([]byte) (int, error)

	lastUsedMu            sync.Mutex
	lastUsedWriteInterval time.Duration
	lastUsedAttempts      map[string]time.Time
}

func NewAPIKeyService(repo APIKeyRepository, pepper string) *APIKeyService {
	return &APIKeyService{
		repo: repo, pepper: []byte(pepper), now: func() time.Time { return time.Now().UTC() }, rand: rand.Read,
		lastUsedWriteInterval: defaultLastUsedWriteInterval, lastUsedAttempts: make(map[string]time.Time),
	}
}

func (s *APIKeyService) Create(ctx context.Context, input APIKeyCreateInput) (APIKeyCreateResult, error) {
	input.TenantID = strings.TrimSpace(input.TenantID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.Name = strings.TrimSpace(input.Name)
	input.CreatedBy = strings.TrimSpace(input.CreatedBy)
	if input.TenantID == "" || input.Name == "" || input.CreatedBy == "" {
		return APIKeyCreateResult{}, fmt.Errorf("%w: tenant, name, and creator are required", ErrAPIKeyInvalid)
	}
	if input.Role != RoleTenantAdmin && input.Role != RoleProjectEditor && input.Role != RoleProjectViewer {
		return APIKeyCreateResult{}, fmt.Errorf("%w: unsupported role", ErrAPIKeyInvalid)
	}
	if input.Role != RoleTenantAdmin && input.ProjectID == "" {
		return APIKeyCreateResult{}, fmt.Errorf("%w: project role requires project", ErrAPIKeyInvalid)
	}
	now := s.now()
	if input.ExpiresAt != nil && !input.ExpiresAt.After(now) {
		return APIKeyCreateResult{}, fmt.Errorf("%w: expiry must be in the future", ErrAPIKeyInvalid)
	}

	keyID := id.New("key")
	secretBytes := make([]byte, 32)
	if n, err := s.rand(secretBytes); err != nil || n != len(secretBytes) {
		if err == nil {
			err = errors.New("short random read")
		}
		return APIKeyCreateResult{}, fmt.Errorf("generate api key: %w", err)
	}
	secret := apiKeyPrefix + keyID + "_" + base64.RawURLEncoding.EncodeToString(secretBytes)
	item := APIKey{
		ID:        keyID,
		TenantID:  input.TenantID,
		ProjectID: input.ProjectID,
		Name:      input.Name,
		Prefix:    apiKeyPrefix + keyID,
		KeyHash:   s.hashAPIKey(secret),
		Role:      input.Role,
		CreatedBy: input.CreatedBy,
		CreatedAt: now,
		ExpiresAt: input.ExpiresAt,
	}
	if err := s.repo.CreateAPIKey(ctx, item); err != nil {
		return APIKeyCreateResult{}, err
	}
	return APIKeyCreateResult{APIKey: item, Secret: secret}, nil
}

func (s *APIKeyService) List(ctx context.Context, tenantID string) ([]APIKey, error) {
	return s.repo.ListAPIKeys(ctx, strings.TrimSpace(tenantID))
}

func (s *APIKeyService) Revoke(ctx context.Context, tenantID, keyID string) error {
	revoked, err := s.repo.RevokeAPIKey(ctx, strings.TrimSpace(tenantID), strings.TrimSpace(keyID), s.now())
	if err != nil {
		return err
	}
	if !revoked {
		return ErrAPIKeyNotFound
	}
	return nil
}

func (s *APIKeyService) Authenticate(ctx context.Context, secret string) (Principal, error) {
	keyID, ok := parseAPIKeyID(secret)
	if !ok {
		return Principal{}, ErrAPIKeyInvalid
	}
	item, found, err := s.repo.GetAPIKeyByID(ctx, keyID)
	if err != nil {
		return Principal{}, err
	}
	if !found || subtle.ConstantTimeCompare([]byte(item.KeyHash), []byte(s.hashAPIKey(secret))) != 1 {
		return Principal{}, ErrAPIKeyInvalid
	}
	if item.RevokedAt != nil {
		return Principal{}, ErrAPIKeyRevoked
	}
	now := s.now()
	if item.ExpiresAt != nil && !item.ExpiresAt.After(now) {
		return Principal{}, ErrAPIKeyExpired
	}
	principal := Principal{Kind: PrincipalAPIKey, SubjectID: item.ID, TenantID: item.TenantID, Role: item.Role, ProjectID: item.ProjectID}
	if !principal.Valid() {
		return Principal{}, ErrAPIKeyInvalid
	}
	s.touchLastUsed(ctx, item.ID, now)
	return principal, nil
}

func (s *APIKeyService) touchLastUsed(ctx context.Context, keyID string, usedAt time.Time) {
	interval := s.lastUsedWriteInterval
	s.lastUsedMu.Lock()
	lastAttempt, attempted := s.lastUsedAttempts[keyID]
	if attempted && interval > 0 && usedAt.Before(lastAttempt.Add(interval)) {
		s.lastUsedMu.Unlock()
		return
	}
	if !attempted && len(s.lastUsedAttempts) >= maxLastUsedThrottleEntries {
		clear(s.lastUsedAttempts)
	}
	s.lastUsedAttempts[keyID] = usedAt
	s.lastUsedMu.Unlock()

	// Authentication has already succeeded. Usage accounting is deliberately
	// best-effort so an audit-write failure cannot turn a valid credential into
	// an availability failure. The repository cutoff also protects multi-instance
	// deployments from writing the same key on every request.
	_ = s.repo.TouchAPIKeyLastUsed(ctx, keyID, usedAt, usedAt.Add(-interval))
}

func (s *APIKeyService) hashAPIKey(secret string) string {
	mac := hmac.New(sha256.New, s.pepper)
	_, _ = mac.Write([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func parseAPIKeyID(secret string) (string, bool) {
	if !strings.HasPrefix(secret, apiKeyPrefix) {
		return "", false
	}
	raw := strings.TrimPrefix(secret, apiKeyPrefix)
	if !strings.HasPrefix(raw, "key_") {
		return "", false
	}
	remainderSeparator := strings.IndexByte(raw[len("key_"):], '_')
	if remainderSeparator < 0 {
		return "", false
	}
	separator := len("key_") + remainderSeparator
	if separator <= 0 || separator == len(raw)-1 {
		return "", false
	}
	keyID := raw[:separator]
	material, err := base64.RawURLEncoding.DecodeString(raw[separator+1:])
	if err != nil || len(material) != 32 {
		return "", false
	}
	return keyID, true
}
