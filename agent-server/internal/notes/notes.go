package notes

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aruhiremath92/agent-server/internal/httpapi"
)

var ErrNotFound = errors.New("note not found")

type Note struct {
	ID        int       `json:"id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Connector interface {
	CreateNote(content string) (Note, error)
	ListNotes() ([]Note, error)
	DeleteNote(id int) error
}

type Repository = Connector

type Handler struct {
	repository Connector
}

func NewHandler(repository Connector) *Handler {
	return &Handler{repository: repository}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/notes", h.create)
	mux.HandleFunc("GET /api/v1/notes", h.list)
	mux.HandleFunc("DELETE /api/v1/notes/{id}", h.delete)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Content string `json:"content"`
	}
	if err := httpapi.DecodeJSON(w, r, &request); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	content := strings.TrimSpace(request.Content)
	if content == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "content is required")
		return
	}
	note, err := h.repository.CreateNote(content)
	if err != nil {
		slog.Error("failed to create note", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create note")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"data": note})
}

func (h *Handler) list(w http.ResponseWriter, _ *http.Request) {
	notes, err := h.repository.ListNotes()
	if err != nil {
		slog.Error("failed to list notes", "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list notes")
		return
	}
	if notes == nil {
		notes = []Note{}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"data": notes})
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "note id must be a positive integer")
		return
	}
	if err := h.repository.DeleteNote(id); err != nil {
		if errors.Is(err, ErrNotFound) {
			httpapi.WriteError(w, http.StatusNotFound, "not_found", "note not found")
			return
		}
		slog.Error("failed to delete note", "note_id", id, "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to delete note")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
