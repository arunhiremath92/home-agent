package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMiddlewareAuthenticatesClientAndHousehold(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	config := Config{
		Secret: []byte("0123456789abcdef0123456789abcdef"), Audience: "agent-api",
		HouseholdID: "home-1", Now: func() time.Time { return now },
	}
	middleware, err := Middleware(config)
	if err != nil {
		t.Fatalf("Middleware() error = %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Error("authenticated request has no identity")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if identity.ClientID != "client-1" || identity.HouseholdID != "home-1" {
			t.Errorf("identity = %#v, want client-1/home-1", identity)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
	request.Header.Set("Authorization", "Bearer "+signedToken(t, config.Secret, map[string]any{
		"sub": "client-1", "aud": []string{"other", "agent-api"}, "household_id": "home-1",
		"exp": now.Add(time.Minute).Unix(),
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNoContent)
	}
}

func TestMiddlewareRejectsInvalidTokensAndAllowsHealth(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	config := Config{
		Secret: []byte("0123456789abcdef0123456789abcdef"), Audience: "agent-api",
		HouseholdID: "home-1", Now: func() time.Time { return now },
	}
	middleware, err := Middleware(config)
	if err != nil {
		t.Fatalf("Middleware() error = %v", err)
	}
	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name  string
		token string
	}{
		{name: "missing token"},
		{name: "expired token", token: signedToken(t, config.Secret, map[string]any{
			"sub": "client-1", "aud": "agent-api", "household_id": "home-1",
			"exp": now.Add(-time.Second).Unix(),
		})},
		{name: "wrong audience", token: signedToken(t, config.Secret, map[string]any{
			"sub": "client-1", "aud": "other", "household_id": "home-1",
			"exp": now.Add(time.Minute).Unix(),
		})},
		{name: "different household", token: signedToken(t, config.Secret, map[string]any{
			"sub": "client-1", "aud": "agent-api", "household_id": "home-2",
			"exp": now.Add(time.Minute).Unix(),
		})},
		{name: "missing household", token: signedToken(t, config.Secret, map[string]any{
			"sub": "client-1", "aud": "agent-api", "exp": now.Add(time.Minute).Unix(),
		})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/notes", nil)
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
		})
	}

	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusNoContent {
		t.Fatalf("health status = %d, want %d", health.Code, http.StatusNoContent)
	}
	dashboard := httptest.NewRecorder()
	handler.ServeHTTP(dashboard, httptest.NewRequest(http.MethodGet, "/", nil))
	if dashboard.Code != http.StatusNoContent {
		t.Fatalf("dashboard status = %d, want %d", dashboard.Code, http.StatusNoContent)
	}
}

func signedToken(t *testing.T, secret []byte, claims map[string]any) string {
	t.Helper()
	header, err := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(unsigned))
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
