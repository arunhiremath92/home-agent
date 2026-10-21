package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func IssueToken(config Config, clientID, householdID string, now time.Time, lifetime time.Duration) (string, error) {
	if len(config.Secret) < 32 || strings.TrimSpace(config.Audience) == "" ||
		strings.TrimSpace(config.HouseholdID) == "" {
		return "", fmt.Errorf("JWT secret, audience, and configured household are required")
	}
	if strings.TrimSpace(clientID) == "" {
		return "", fmt.Errorf("client ID is required")
	}
	if householdID != config.HouseholdID {
		return "", fmt.Errorf("requested household does not match configured household")
	}
	if lifetime <= 0 || lifetime > 30*24*time.Hour {
		return "", fmt.Errorf("token lifetime must be greater than zero and at most 30 days")
	}
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		return "", fmt.Errorf("encode JWT header: %w", err)
	}
	claims, err := json.Marshal(map[string]any{
		"sub":          clientID,
		"aud":          config.Audience,
		"household_id": householdID,
		"iat":          now.Unix(),
		"nbf":          now.Unix(),
		"exp":          now.Add(lifetime).Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("encode JWT claims: %w", err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims)
	mac := hmac.New(sha256.New, config.Secret)
	if _, err := mac.Write([]byte(unsigned)); err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
