package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/aruhiremath92/agent-server/internal/httpapi"
)

type Identity struct {
	ClientID    string
	HouseholdID string
}

type identityKey struct{}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey{}).(Identity)
	return identity, ok
}

type Config struct {
	Secret      []byte
	Audience    string
	HouseholdID string
	Now         func() time.Time
}

func ConfigFromEnvironment() (Config, error) {
	return NewConfig(
		os.Getenv("JWT_SECRET"),
		os.Getenv("AGENT_API_AUDIENCE"),
		os.Getenv("AGENT_HOUSEHOLD_ID"),
	)
}

func ConfigFromFile(path string) (Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Config{}, fmt.Errorf("stat auth config file: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Config{}, fmt.Errorf("auth config file permissions must not allow group or other access")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read auth config file: %w", err)
	}
	values := make(map[string]string, 3)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "JWT_SECRET", "AGENT_API_AUDIENCE", "AGENT_HOUSEHOLD_ID":
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return NewConfig(values["JWT_SECRET"], values["AGENT_API_AUDIENCE"], values["AGENT_HOUSEHOLD_ID"])
}

func NewConfig(secretValue, audienceValue, householdValue string) (Config, error) {
	secret := strings.TrimSpace(secretValue)
	audience := strings.TrimSpace(audienceValue)
	householdID := strings.TrimSpace(householdValue)
	if len(secret) < 32 {
		return Config{}, fmt.Errorf("JWT_SECRET must contain at least 32 characters")
	}
	if audience == "" {
		return Config{}, fmt.Errorf("AGENT_API_AUDIENCE must be set")
	}
	if householdID == "" {
		return Config{}, fmt.Errorf("AGENT_HOUSEHOLD_ID must be set")
	}
	return Config{Secret: []byte(secret), Audience: audience, HouseholdID: householdID}, nil
}

func Middleware(config Config) (func(http.Handler) http.Handler, error) {
	if len(config.Secret) < 32 {
		return nil, errors.New("JWT secret must contain at least 32 bytes")
	}
	if strings.TrimSpace(config.Audience) == "" {
		return nil, errors.New("JWT audience is required")
	}
	if strings.TrimSpace(config.HouseholdID) == "" {
		return nil, errors.New("configured household ID is required")
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && (r.URL.Path == "/healthz" ||
				r.URL.Path == "/" || r.URL.Path == "/app.css" || r.URL.Path == "/app.js") {
				next.ServeHTTP(w, r)
				return
			}

			header := strings.TrimSpace(r.Header.Get("Authorization"))
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				unauthorized(w)
				return
			}
			identity, err := verify(parts[1], config, now())
			if err != nil || identity.HouseholdID != config.HouseholdID {
				unauthorized(w)
				return
			}
			ctx := r.Context()
			r = r.WithContext(withIdentity(ctx, identity))
			next.ServeHTTP(w, r)
		})
	}, nil
}

func withIdentity(ctx context.Context, identity Identity) context.Context {
	return context.WithValue(ctx, identityKey{}, identity)
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="agent-api"`)
	httpapi.WriteError(w, http.StatusUnauthorized, "unauthorized", "a valid bearer token is required")
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	Type      string `json:"typ"`
}

type jwtClaims struct {
	Subject     string          `json:"sub"`
	Audience    json.RawMessage `json:"aud"`
	HouseholdID string          `json:"household_id"`
	ExpiresAt   *int64          `json:"exp"`
	NotBefore   *int64          `json:"nbf"`
}

func verify(token string, config Config, now time.Time) (Identity, error) {
	if len(token) > 16*1024 {
		return Identity{}, errors.New("JWT exceeds maximum size")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Identity{}, errors.New("JWT must have three segments")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Identity{}, fmt.Errorf("decode JWT header: %w", err)
	}
	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Algorithm != "HS256" {
		return Identity{}, errors.New("JWT algorithm must be HS256")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Identity{}, fmt.Errorf("decode JWT signature: %w", err)
	}
	mac := hmac.New(sha256.New, config.Secret)
	_, _ = mac.Write([]byte(parts[0] + "." + parts[1]))
	expected := mac.Sum(nil)
	if len(signature) != len(expected) || subtle.ConstantTimeCompare(signature, expected) != 1 {
		return Identity{}, errors.New("JWT signature is invalid")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, fmt.Errorf("decode JWT claims: %w", err)
	}
	var claims jwtClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return Identity{}, fmt.Errorf("decode JWT claims: %w", err)
	}
	if claims.Subject == "" || claims.HouseholdID == "" || claims.ExpiresAt == nil {
		return Identity{}, errors.New("JWT requires sub, household_id, and exp claims")
	}
	nowUnix := now.Unix()
	if nowUnix >= *claims.ExpiresAt {
		return Identity{}, errors.New("JWT is expired")
	}
	if claims.NotBefore != nil && nowUnix < *claims.NotBefore {
		return Identity{}, errors.New("JWT is not active")
	}
	var audience string
	if err := json.Unmarshal(claims.Audience, &audience); err == nil {
		if audience != config.Audience {
			return Identity{}, errors.New("JWT audience does not match")
		}
	} else {
		var audiences []string
		if err := json.Unmarshal(claims.Audience, &audiences); err != nil {
			return Identity{}, errors.New("JWT audience claim is invalid")
		}
		matched := false
		for _, candidate := range audiences {
			if candidate == config.Audience {
				matched = true
				break
			}
		}
		if !matched {
			return Identity{}, errors.New("JWT audience does not match")
		}
	}
	return Identity{ClientID: claims.Subject, HouseholdID: claims.HouseholdID}, nil
}
