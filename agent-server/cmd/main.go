package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/aruhiremath92/agent-server/agent"
	"github.com/aruhiremath92/agent-server/internal/auth"
	"github.com/aruhiremath92/agent-server/internal/location"
	"github.com/aruhiremath92/agent-server/internal/server"
	"github.com/aruhiremath92/agent-server/internal/store"
	"github.com/aruhiremath92/agent-server/route"
)

const (
	// ServerPort is the port on which the agent server will listen for incoming requests.
	ServerPort = ":8080"
)

func main() {
	configureLogger()
	if err := run(); err != nil {
		slog.Error("agent server stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	authConfig, err := auth.ConfigFromEnvironment()
	if err != nil {
		return fmt.Errorf("configure API authentication: %w", err)
	}

	database, err := store.NewStore(context.Background())
	if err != nil {
		return fmt.Errorf("initialize store: %w", err)
	}
	defer database.Close()

	routeCalculator, err := route.NewRouteCalculator()
	if err != nil {
		return fmt.Errorf("initialize route calculator: %w", err)
	}
	defer func() {
		if err := routeCalculator.Close(); err != nil {
			slog.Error("failed to close route calculator", "error", err)
		}
	}()

	placesClient, err := location.NewGooglePlacesClient()
	if err != nil {
		return fmt.Errorf("initialize Google Places client: %w", err)
	}

	chatAgent, err := agent.NewConfiguredAgent()
	if err != nil {
		return fmt.Errorf("initialize agent: %w", err)
	}

	agentServer, err := server.NewWorkflowServerWithAuth(database, routeCalculator, chatAgent, placesClient, authConfig)
	if err != nil {
		return fmt.Errorf("initialize agent server: %w", err)
	}
	address := strings.TrimSpace(os.Getenv("AGENT_SERVER_ADDRESS"))
	if address == "" {
		address = ServerPort
	}
	if err := agentServer.Start(address); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("HTTP server failed: %w", err)
	}
	return nil
}

func configureLogger() {
	level := slog.LevelInfo
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LOG_LEVEL")), "debug") {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
}
