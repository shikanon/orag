package auth

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAPIKeyCreateAuthenticateListAndRevoke(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryAPIKeyRepository()
	service := NewAPIKeyService(repo, "test-pepper")
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	service.rand = func(dst []byte) (int, error) {
		for i := range dst {
			dst[i] = byte(i + 1)
		}
		return len(dst), nil
	}
	expires := now.Add(24 * time.Hour)
	created, err := service.Create(ctx, APIKeyCreateInput{
		TenantID: "tenant_1", ProjectID: "prj_1", Name: "CI deploy",
		Role: RoleProjectEditor, CreatedBy: "user_admin", ExpiresAt: &expires,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(created.Secret, apiKeyPrefix+created.APIKey.ID+"_") {
		t.Fatalf("secret has unexpected format")
	}
	if created.APIKey.KeyHash == "" || strings.Contains(created.APIKey.KeyHash, created.Secret) {
		t.Fatal("key hash is missing or contains the secret")
	}

	principal, err := service.Authenticate(ctx, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if principal.Kind != PrincipalAPIKey || principal.SubjectID != created.APIKey.ID || principal.ProjectID != "prj_1" || principal.Role != RoleProjectEditor {
		t.Fatalf("principal = %#v", principal)
	}

	items, err := service.List(ctx, "tenant_1")
	if err != nil || len(items) != 1 {
		t.Fatalf("List() items=%d err=%v", len(items), err)
	}
	if items[0].LastUsedAt == nil || !items[0].LastUsedAt.Equal(now) {
		t.Fatalf("last_used_at = %v, want %v", items[0].LastUsedAt, now)
	}
	encoded, err := json.Marshal(items[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "key_hash") || strings.Contains(string(encoded), created.Secret) {
		t.Fatalf("metadata response leaked credential material: %s", encoded)
	}
	otherItems, err := service.List(ctx, "tenant_2")
	if err != nil || len(otherItems) != 0 {
		t.Fatalf("cross-tenant list items=%d err=%v", len(otherItems), err)
	}

	if err := service.Revoke(ctx, "tenant_2", created.APIKey.ID); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("cross-tenant revoke error = %v", err)
	}
	if err := service.Revoke(ctx, "tenant_1", created.APIKey.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Revoke(ctx, "tenant_1", created.APIKey.ID); err != nil {
		t.Fatalf("idempotent revoke error = %v", err)
	}
	if _, err := service.Authenticate(ctx, created.Secret); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("Authenticate() error = %v, want revoked", err)
	}
}

func TestAPIKeyCreateValidation(t *testing.T) {
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	service := NewAPIKeyService(NewMemoryAPIKeyRepository(), "test-pepper")
	service.now = func() time.Time { return now }
	tests := []struct {
		name  string
		input APIKeyCreateInput
	}{
		{"missing fields", APIKeyCreateInput{}},
		{"role required", APIKeyCreateInput{TenantID: "tenant_1", Name: "key", CreatedBy: "user_1"}},
		{"unknown role", APIKeyCreateInput{TenantID: "tenant_1", Name: "key", CreatedBy: "user_1", Role: "owner"}},
		{"project role requires project", APIKeyCreateInput{TenantID: "tenant_1", Name: "key", CreatedBy: "user_1", Role: RoleProjectViewer}},
		{"past expiry", APIKeyCreateInput{TenantID: "tenant_1", Name: "key", CreatedBy: "user_1", Role: RoleTenantAdmin, ExpiresAt: timePointer(now)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := service.Create(context.Background(), tt.input); !errors.Is(err, ErrAPIKeyInvalid) {
				t.Fatalf("Create() error = %v, want invalid", err)
			}
		})
	}
}

func TestAPIKeyRotateAtomicallyReplacesActiveSource(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryAPIKeyRepository()
	service := NewAPIKeyService(repo, "test-pepper")
	now := time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	expires := now.Add(24 * time.Hour)
	source, err := service.Create(ctx, APIKeyCreateInput{TenantID: "tenant_1", ProjectID: "prj_1", Name: "CI deploy", Role: RoleProjectEditor, CreatedBy: "user_1", ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := service.Rotate(ctx, APIKeyRotateInput{TenantID: "tenant_1", KeyID: source.APIKey.ID, RotatedBy: "user_2"})
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Secret == "" || replacement.APIKey.ID == source.APIKey.ID || replacement.APIKey.RotatedFromKeyID != source.APIKey.ID {
		t.Fatalf("replacement = %#v", replacement)
	}
	if replacement.APIKey.TenantID != source.APIKey.TenantID || replacement.APIKey.ProjectID != source.APIKey.ProjectID || replacement.APIKey.Role != source.APIKey.Role || replacement.APIKey.Name != source.APIKey.Name || replacement.APIKey.ExpiresAt == nil || !replacement.APIKey.ExpiresAt.Equal(expires) {
		t.Fatalf("rotation did not preserve source scope: source=%#v replacement=%#v", source.APIKey, replacement.APIKey)
	}
	if _, err := service.Authenticate(ctx, source.Secret); !errors.Is(err, ErrAPIKeyRevoked) {
		t.Fatalf("source authentication error = %v, want revoked", err)
	}
	if principal, err := service.Authenticate(ctx, replacement.Secret); err != nil || principal.SubjectID != replacement.APIKey.ID || principal.ProjectID != "prj_1" {
		t.Fatalf("replacement authentication principal=%#v err=%v", principal, err)
	}
	items, err := service.List(ctx, "tenant_1")
	if err != nil || len(items) != 2 || !hasRotatedAPIKey(items, replacement.APIKey.ID, source.APIKey.ID) || strings.Contains(items[0].Prefix, replacement.Secret) {
		t.Fatalf("listed rotation metadata=%#v err=%v", items, err)
	}
	if _, err := service.Rotate(ctx, APIKeyRotateInput{TenantID: "tenant_1", KeyID: source.APIKey.ID, RotatedBy: "user_2"}); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("second rotation error = %v, want not found", err)
	}
	if _, err := service.Rotate(ctx, APIKeyRotateInput{TenantID: "tenant_2", KeyID: replacement.APIKey.ID, RotatedBy: "user_2"}); !errors.Is(err, ErrAPIKeyNotFound) {
		t.Fatalf("cross-tenant rotation error = %v, want not found", err)
	}
}

func hasRotatedAPIKey(items []APIKey, id, sourceID string) bool {
	for _, item := range items {
		if item.ID == id && item.RotatedFromKeyID == sourceID {
			return true
		}
	}
	return false
}

func TestAPIKeyAuthenticationRejectsMalformedWrongAndExpiredSecrets(t *testing.T) {
	ctx := context.Background()
	repo := NewMemoryAPIKeyRepository()
	service := NewAPIKeyService(repo, "test-pepper")
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	created, err := service.Create(ctx, APIKeyCreateInput{TenantID: "tenant_1", Name: "robot", Role: RoleTenantAdmin, CreatedBy: "user_1"})
	if err != nil {
		t.Fatal(err)
	}

	for _, malformed := range []string{"", "bearer", "orag_sk_bad", created.APIKey.Prefix + "_short"} {
		if _, err := service.Authenticate(ctx, malformed); !errors.Is(err, ErrAPIKeyInvalid) {
			t.Fatalf("Authenticate(%q) error = %v", malformed, err)
		}
	}
	replacement := "A"
	if strings.HasSuffix(created.Secret, replacement) {
		replacement = "B"
	}
	wrong := created.Secret[:len(created.Secret)-1] + replacement
	if _, err := service.Authenticate(ctx, wrong); !errors.Is(err, ErrAPIKeyInvalid) {
		t.Fatalf("wrong secret error = %v", err)
	}

	expires := now.Add(time.Hour)
	expiring, err := service.Create(ctx, APIKeyCreateInput{TenantID: "tenant_1", Name: "short", Role: RoleTenantAdmin, CreatedBy: "user_1", ExpiresAt: &expires})
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return expires }
	if _, err := service.Authenticate(ctx, expiring.Secret); !errors.Is(err, ErrAPIKeyExpired) {
		t.Fatalf("expired secret error = %v", err)
	}
}

func TestAPIKeyCreateFailsClosedOnRandomSourceFailure(t *testing.T) {
	service := NewAPIKeyService(NewMemoryAPIKeyRepository(), "test-pepper")
	service.rand = func([]byte) (int, error) { return 0, errors.New("entropy unavailable") }
	_, err := service.Create(context.Background(), APIKeyCreateInput{TenantID: "tenant_1", Name: "key", Role: RoleTenantAdmin, CreatedBy: "user_1"})
	if err == nil || errors.Is(err, ErrAPIKeyInvalid) {
		t.Fatalf("Create() error = %v, want entropy error", err)
	}
}

func TestAPIKeyHashIsBoundToServerPepper(t *testing.T) {
	left := NewAPIKeyService(NewMemoryAPIKeyRepository(), "pepper-a")
	right := NewAPIKeyService(NewMemoryAPIKeyRepository(), "pepper-b")
	secret := "synthetic-non-credential-test-input"
	if left.hashAPIKey(secret) == right.hashAPIKey(secret) {
		t.Fatal("different server peppers produced the same API key hash")
	}
}

func TestAPIKeyLastUsedWritesAreThrottledAndBestEffort(t *testing.T) {
	ctx := context.Background()
	base := NewMemoryAPIKeyRepository()
	repo := &countingAPIKeyRepository{MemoryAPIKeyRepository: base}
	service := NewAPIKeyService(repo, "test-pepper")
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	created, err := service.Create(ctx, APIKeyCreateInput{TenantID: "tenant_1", Name: "robot", Role: RoleTenantAdmin, CreatedBy: "user_1"})
	if err != nil {
		t.Fatal(err)
	}
	wrong := created.Secret[:len(created.Secret)-1] + "A"
	if strings.HasSuffix(created.Secret, "A") {
		wrong = created.Secret[:len(created.Secret)-1] + "B"
	}
	if _, err := service.Authenticate(ctx, wrong); !errors.Is(err, ErrAPIKeyInvalid) {
		t.Fatalf("wrong secret error = %v", err)
	}
	if got := repo.touches.Load(); got != 0 {
		t.Fatalf("invalid authentication touches = %d, want 0", got)
	}

	if _, err := service.Authenticate(ctx, created.Secret); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Authenticate(ctx, created.Secret); err != nil {
		t.Fatal(err)
	}
	now = now.Add(defaultLastUsedWriteInterval - time.Second)
	if _, err := service.Authenticate(ctx, created.Secret); err != nil {
		t.Fatal(err)
	}
	if got := repo.touches.Load(); got != 1 {
		t.Fatalf("touches before interval = %d, want 1", got)
	}

	now = now.Add(time.Second)
	if _, err := service.Authenticate(ctx, created.Secret); err != nil {
		t.Fatal(err)
	}
	if got := repo.touches.Load(); got != 2 {
		t.Fatalf("touches after interval = %d, want 2", got)
	}
	items, err := service.List(ctx, "tenant_1")
	if err != nil || len(items) != 1 || items[0].LastUsedAt == nil || !items[0].LastUsedAt.Equal(now) {
		t.Fatalf("last-used metadata = %#v, err=%v", items, err)
	}

	repo.touchErr = errors.New("audit store unavailable")
	now = now.Add(defaultLastUsedWriteInterval)
	if _, err := service.Authenticate(ctx, created.Secret); err != nil {
		t.Fatalf("best-effort touch changed authentication result: %v", err)
	}
	if _, err := service.Authenticate(ctx, created.Secret); err != nil {
		t.Fatal(err)
	}
	if got := repo.touches.Load(); got != 3 {
		t.Fatalf("failed touch attempts = %d, want one throttled attempt", got)
	}
}

type countingAPIKeyRepository struct {
	*MemoryAPIKeyRepository
	touches  atomic.Int64
	touchErr error
}

func (r *countingAPIKeyRepository) TouchAPIKeyLastUsed(ctx context.Context, id string, usedAt, notAfter time.Time) error {
	r.touches.Add(1)
	if r.touchErr != nil {
		return r.touchErr
	}
	return r.MemoryAPIKeyRepository.TouchAPIKeyLastUsed(ctx, id, usedAt, notAfter)
}

func timePointer(value time.Time) *time.Time { return &value }
