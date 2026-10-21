package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIssueTokenProducesUsableToken(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	config := Config{
		Secret:      []byte("0123456789abcdef0123456789abcdef"),
		Audience:    "home-agent-api",
		HouseholdID: "home",
	}
	token, err := IssueToken(config, "phone-app", "home", now, time.Hour)
	if err != nil {
		t.Fatalf("IssueToken() error = %v", err)
	}
	identity, err := verify(token, config, now)
	if err != nil {
		t.Fatalf("verify(issued token) error = %v", err)
	}
	if identity.ClientID != "phone-app" || identity.HouseholdID != "home" {
		t.Fatalf("identity = %#v, want phone-app/home", identity)
	}
}

func TestIssueTokenRejectsWrongHouseholdAndExcessiveLifetime(t *testing.T) {
	config := Config{
		Secret:      []byte("0123456789abcdef0123456789abcdef"),
		Audience:    "home-agent-api",
		HouseholdID: "home",
	}
	for _, test := range []struct {
		name      string
		household string
		lifetime  time.Duration
	}{
		{name: "wrong household", household: "other", lifetime: time.Hour},
		{name: "long lifetime", household: "home", lifetime: 31 * 24 * time.Hour},
		{name: "zero lifetime", household: "home"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := IssueToken(config, "client", test.household, time.Now(), test.lifetime); err == nil {
				t.Fatal("IssueToken() error = nil, want validation error")
			}
		})
	}
}

func TestConfigFromFileRequiresPrivatePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent.env")
	testSecret := strings.Repeat("x", 32)
	data := []byte("JWT_SECRET=" + testSecret + "\nAGENT_API_AUDIENCE=api\nAGENT_HOUSEHOLD_ID=home\nOPENAI_API_KEY=not-loaded\n")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	config, err := ConfigFromFile(path)
	if err != nil {
		t.Fatalf("ConfigFromFile() error = %v", err)
	}
	if config.Audience != "api" || config.HouseholdID != "home" || string(config.Secret) != testSecret {
		t.Fatalf("ConfigFromFile() = %#v, want configured auth values", config)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfigFromFile(path); err == nil {
		t.Fatal("ConfigFromFile() error = nil, want insecure permissions error")
	}
}
