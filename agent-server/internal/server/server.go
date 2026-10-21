package server

import (
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/aruhiremath92/agent-server/agent"
	"github.com/aruhiremath92/agent-server/internal/auth"
	"github.com/aruhiremath92/agent-server/internal/httpapi"
	"github.com/aruhiremath92/agent-server/internal/location"
	"github.com/aruhiremath92/agent-server/internal/notes"
	"github.com/aruhiremath92/agent-server/internal/tasks"
	"github.com/aruhiremath92/agent-server/internal/webui"
)

type Store interface {
	notes.Repository
	tasks.Repository
	location.Repository
	location.TaskRepository
}

type WorkflowStore interface {
	Store
	location.WorkflowRepository
}

type RouteCalculator interface {
	location.RouteCalculator
}

type ChatAgent interface {
	location.ChatAgent
}

type Server struct {
	handler http.Handler
}

func NewServer(repository Store, routeCalculator RouteCalculator) (*Server, error) {
	return NewServerWithAgent(repository, routeCalculator, agent.NewAgent())
}

func NewServerWithAgent(repository Store, routeCalculator RouteCalculator, chatAgent ChatAgent) (*Server, error) {
	return newServer(repository, routeCalculator, chatAgent, nil)
}

func NewWorkflowServer(repository WorkflowStore, routeCalculator RouteCalculator, chatAgent ChatAgent, places location.PlaceFinder) (*Server, error) {
	if places == nil {
		return nil, fmt.Errorf("Google Places client is required")
	}
	return newServer(repository, routeCalculator, chatAgent, places)
}

func NewWorkflowServerWithAuth(repository WorkflowStore, routeCalculator RouteCalculator, chatAgent ChatAgent, places location.PlaceFinder, authConfig auth.Config) (*Server, error) {
	server, err := NewWorkflowServer(repository, routeCalculator, chatAgent, places)
	if err != nil {
		return nil, err
	}
	middleware, err := auth.Middleware(authConfig)
	if err != nil {
		return nil, fmt.Errorf("configure JWT authentication: %w", err)
	}
	server.handler = middleware(server.handler)
	return server, nil
}

func newServer(repository Store, routeCalculator RouteCalculator, chatAgent ChatAgent, places location.PlaceFinder) (*Server, error) {
	if repository == nil {
		return nil, fmt.Errorf("store is required")
	}
	if routeCalculator == nil {
		return nil, fmt.Errorf("route calculator is required")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"status": "ok"}})
	})
	mux.Handle("GET /", webui.Handler())
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			httpapi.WriteError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
			return
		}
		http.NotFound(w, r)
	})
	notes.NewHandler(repository).RegisterRoutes(mux)
	tasks.NewHandler(repository).RegisterRoutes(mux)
	var locationHandler *location.Handler
	if places == nil {
		locationHandler = location.NewHandler(repository, repository, routeCalculator, chatAgent)
	} else {
		workflowRepository, ok := repository.(WorkflowStore)
		if !ok {
			return nil, fmt.Errorf("store does not support location workflow persistence")
		}
		locationHandler = location.NewWorkflowHandler(repository, repository, routeCalculator, chatAgent, workflowRepository, places)
	}
	locationHandler.RegisterRoutes(mux)
	return &Server{handler: requestLogger(normalizeHTTPErrors(mux))}, nil
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) Start(address string) error {
	server := &http.Server{
		Addr:              address,
		Handler:           s.handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	slog.Info("starting API server", "address", address)
	return server.ListenAndServe()
}

func normalizeHTTPErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		next.ServeHTTP(response, r)
		if response.Code == http.StatusNotFound || response.Code == http.StatusMethodNotAllowed {
			for key, values := range response.Header() {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			code, message := "not_found", "endpoint not found"
			if response.Code == http.StatusMethodNotAllowed {
				code, message = "method_not_allowed", "method not allowed"
			}
			httpapi.WriteError(w, response.Code, code, message)
			return
		}
		for key, values := range response.Header() {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.Code)
		_, _ = w.Write(response.Body.Bytes())
	})
}

func requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, r)
		attributes := []any{
			"method", r.Method,
			"path", r.URL.Path,
			"status", response.status,
			"remote", r.RemoteAddr,
			"duration", time.Since(started),
		}
		if identity, ok := auth.IdentityFromContext(r.Context()); ok {
			attributes = append(attributes, "client_id", identity.ClientID, "household_id", identity.HouseholdID)
		}
		slog.Info("HTTP request", attributes...)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(body []byte) (int, error) {
	return w.ResponseWriter.Write(body)
}
