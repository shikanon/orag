package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

type Claims struct {
	TenantID  string `json:"tenant_id"`
	UserID    string `json:"user_id"`
	Role      Role   `json:"role,omitempty"`
	ExpiresAt int64  `json:"exp"`
}

type Service struct {
	secret []byte
	ttl    time.Duration
}

func NewService(secret string, ttl time.Duration) *Service {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Service{secret: []byte(secret), ttl: ttl}
}

func (s *Service) IssueToken(tenantID, userID string) (string, error) {
	claims := Claims{
		TenantID:  tenantID,
		UserID:    userID,
		Role:      RoleTenantAdmin,
		ExpiresAt: time.Now().Add(s.ttl).Unix(),
	}
	body, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	signature := s.sign(payload)
	return payload + "." + signature, nil
}

func (s *Service) ParseToken(token string) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return Claims{}, errors.New("invalid token format")
	}
	if !hmac.Equal([]byte(s.sign(parts[0])), []byte(parts[1])) {
		return Claims{}, errors.New("invalid token signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Claims{}, err
	}
	var claims Claims
	if err := json.Unmarshal(body, &claims); err != nil {
		return Claims{}, err
	}
	if claims.ExpiresAt < time.Now().Unix() {
		return Claims{}, errors.New("token expired")
	}
	if claims.TenantID == "" || claims.UserID == "" {
		return Claims{}, fmt.Errorf("token missing tenant or user")
	}
	// Tokens issued before RBAC did not carry a role. They represented the
	// bootstrap administrator and remain tenant-admin compatible during beta.
	if claims.Role == "" {
		claims.Role = RoleTenantAdmin
	}
	principal := claims.Principal()
	if !principal.Valid() || principal.Role != RoleTenantAdmin {
		return Claims{}, errors.New("invalid token role")
	}
	return claims, nil
}

func (c Claims) Principal() Principal {
	return Principal{
		Kind:      PrincipalUser,
		SubjectID: c.UserID,
		TenantID:  c.TenantID,
		Role:      c.Role,
	}
}

func (s *Service) sign(payload string) string {
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
