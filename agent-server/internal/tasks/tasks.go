package tasks

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/aruhiremath92/agent-server/internal/httpapi"
)

type Task struct {
	ID          int        `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	DueAt       *time.Time `json:"due_at,omitempty"`
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
}

type Repository interface {
	AddTask(Task) (Task, error)
	ListTasks(status string) ([]Task, error)
	GetPendingTasks() ([]Task, error)
}

type Handler struct {
	repository Repository
}

func NewHandler(repository Repository) *Handler {
	return &Handler{repository: repository}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/tasks", h.create)
	mux.HandleFunc("GET /api/v1/tasks", h.list)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Title       string     `json:"title"`
		Description string     `json:"description"`
		DueAt       *time.Time `json:"due_at,omitempty"`
	}
	if err := httpapi.DecodeJSON(w, r, &request); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	request.Title = strings.TrimSpace(request.Title)
	if request.Title == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "title is required")
		return
	}
	task := Task{Title: request.Title, Description: request.Description, DueAt: request.DueAt}
	created, err := h.repository.AddTask(task)
	if err != nil {
		slog.Error("failed to create task", "title", task.Title, "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create task")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	tasks, err := h.repository.ListTasks(status)
	if err != nil {
		slog.Error("failed to list tasks", "status", status, "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list tasks")
		return
	}
	if tasks == nil {
		tasks = []Task{}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"data": tasks})
}
