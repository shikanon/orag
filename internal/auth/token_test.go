package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"
)

func TestIssueTokenCarriesTenantAdminPrincipal(t *testing.T) {
	service := NewService("test-secret", time.Hour)
	token, err := service.IssueToken("tenant_1", "user_1")
	if err != nil {
		t.Fatal(err)
	}
	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Role != RoleTenantAdmin {
		t.Fatalf("role = %q, want %q", claims.Role, RoleTenantAdmin)
	}
	if principal := claims.Principal(); !principal.Valid() || principal.Kind != PrincipalUser {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestParseTokenAcceptsLegacyRolelessAdminToken(t *testing.T) {
	service := NewService("test-secret", time.Hour)
	token := signTestClaims(t, "test-secret", map[string]any{
		"tenant_id": "tenant_1",
		"user_id":   "user_1",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	claims, err := service.ParseToken(token)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Role != RoleTenantAdmin {
		t.Fatalf("role = %q, want tenant_admin", claims.Role)
	}
}

func TestParseTokenRejectsUnsupportedUserRole(t *testing.T) {
	service := NewService("test-secret", time.Hour)
	token := signTestClaims(t, "test-secret", map[string]any{
		"tenant_id": "tenant_1",
		"user_id":   "user_1",
		"role":      "project_viewer",
		"exp":       time.Now().Add(time.Hour).Unix(),
	})
	if _, err := service.ParseToken(token); err == nil {
		t.Fatal("ParseToken() error = nil, want unsupported user role rejected")
	}
}

func signTestClaims(t *testing.T, secret string, claims map[string]any) string {
	t.Helper()
	body, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
