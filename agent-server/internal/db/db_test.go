package db

import (
	"net/url"
	"testing"
)

func TestConnectionURLFromEnvironmentEscapesCredentials(t *testing.T) {
	t.Setenv("POSTGRES_USER", "agent")
	t.Setenv("POSTGRES_PASSWORD", "pass@word:/?#")
	t.Setenv("POSTGRES_HOST", "database")
	t.Setenv("POSTGRES_PORT", "5432")
	t.Setenv("POSTGRES_DB", "agent")

	rawURL, err := connectionURLFromEnvironment()
	if err != nil {
		t.Fatalf("connectionURLFromEnvironment() error = %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse connection URL: %v", err)
	}
	password, ok := parsed.User.Password()
	if !ok || password != "pass@word:/?#" {
		t.Errorf("parsed password = %q (present=%t), want original password", password, ok)
	}
	if parsed.Host != "database:5432" || parsed.Path != "/agent" {
		t.Errorf("connection URL host/path = %q/%q", parsed.Host, parsed.Path)
	}
}

func TestConnectionURLRequiresCredentialsAndDatabase(t *testing.T) {
	t.Setenv("POSTGRES_USER", "")
	t.Setenv("POSTGRES_PASSWORD", "")
	t.Setenv("POSTGRES_DB", "")
	if _, err := connectionURLFromEnvironment(); err == nil {
		t.Fatal("connectionURLFromEnvironment() error = nil, want missing configuration error")
	}
}
