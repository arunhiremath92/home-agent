package location

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	routespb "cloud.google.com/go/maps/routing/apiv2/routingpb"
	"github.com/aruhiremath92/agent-server/agent"
	"github.com/aruhiremath92/agent-server/internal/httpapi"
	"github.com/aruhiremath92/agent-server/internal/tasks"
)

type Memory struct {
	ID        int       `json:"id,omitempty"`
	StoreType string    `json:"store_type"`
	StoreName string    `json:"store_name"`
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

type Repository interface {
	AddLocationMemory(Memory) (Memory, error)
	GetLocationMemories(storeType, storeName string) ([]Memory, error)
}

type TaskRepository interface {
	GetPendingTasks() ([]tasks.Task, error)
}

type RouteCalculator interface {
	CalculateRoutes(destinationLat, destinationLng float64) ([]*routespb.Route, error)
}

type HomeRouteCalculator interface {
	CalculateRoutesFrom(originLat, originLng, destinationLat, destinationLng float64) ([]*routespb.Route, error)
}

type ChatAgent interface {
	SendRequest(context.Context, []agent.ModelMessage) (agent.ModelResponse, error)
}

type Handler struct {
	memories Repository
	tasks    TaskRepository
	routes   RouteCalculator
	agent    ChatAgent
	workflow WorkflowRepository
	places   PlaceFinder
	actions  ActionEstimator
}

func NewHandler(memories Repository, taskRepository TaskRepository, routes RouteCalculator, chatAgent ChatAgent) *Handler {
	return &Handler{memories: memories, tasks: taskRepository, routes: routes, agent: chatAgent}
}

func NewWorkflowHandler(memories Repository, taskRepository TaskRepository, routes RouteCalculator, chatAgent ChatAgent, workflow WorkflowRepository, places PlaceFinder) *Handler {
	return &Handler{
		memories: memories,
		tasks:    taskRepository,
		routes:   routes,
		agent:    chatAgent,
		workflow: workflow,
		places:   places,
		actions:  MockActionEstimator{},
	}
}

func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/location-memories", h.createMemory)
	mux.HandleFunc("GET /api/v1/location-memories", h.listMemories)
	mux.HandleFunc("GET /api/v1/location-memories/summary", h.summarizeMemories)
	mux.HandleFunc("POST /api/v1/location-updates", h.locationUpdate)
	if h.workflow != nil {
		mux.HandleFunc("POST /api/v1/shortcuts", h.saveShortcut)
		mux.HandleFunc("GET /api/v1/shortcuts", h.listShortcuts)
		mux.HandleFunc("DELETE /api/v1/shortcuts/{name}", h.deleteShortcut)
		mux.HandleFunc("GET /api/v1/settings/home-location", h.getHomeLocation)
		mux.HandleFunc("PUT /api/v1/settings/home-location", h.saveHomeLocation)
		mux.HandleFunc("GET /api/v1/location-updates/history", h.listLocationHistory)
		mux.HandleFunc("DELETE /api/v1/location-updates/history/{id}", h.deleteLocationHistory)
	}
}

func (h *Handler) createMemory(w http.ResponseWriter, r *http.Request) {
	var request struct {
		StoreType string  `json:"store_type"`
		StoreName string  `json:"store_name"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Message   string  `json:"message"`
	}
	if err := httpapi.DecodeJSON(w, r, &request); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	memory := Memory{
		StoreType: strings.TrimSpace(request.StoreType),
		StoreName: strings.TrimSpace(request.StoreName),
		Latitude:  request.Latitude,
		Longitude: request.Longitude,
		Message:   strings.TrimSpace(request.Message),
	}
	if memory.StoreType == "" || memory.StoreName == "" || memory.Message == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "store_type, store_name, and message are required")
		return
	}
	if memory.Latitude < -90 || memory.Latitude > 90 || memory.Longitude < -180 || memory.Longitude > 180 {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "latitude must be between -90 and 90 and longitude between -180 and 180")
		return
	}
	if memory.Timestamp.IsZero() {
		memory.Timestamp = time.Now().UTC()
	}
	created, err := h.memories.AddLocationMemory(memory)
	if err != nil {
		slog.Error("failed to add location memory", "store_type", memory.StoreType, "store_name", memory.StoreName, "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to create location memory")
		return
	}
	httpapi.WriteJSON(w, http.StatusCreated, map[string]any{"data": created})
}

func (h *Handler) listMemories(w http.ResponseWriter, r *http.Request) {
	storeName := strings.TrimSpace(r.URL.Query().Get("store_name"))
	storeType := strings.TrimSpace(r.URL.Query().Get("store_type"))
	if storeName == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "store_name is required")
		return
	}
	memories, err := h.memories.GetLocationMemories(storeType, storeName)
	if err != nil {
		slog.Error("failed to list location memories", "store_type", storeType, "store_name", storeName, "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to list location memories")
		return
	}
	if memories == nil {
		memories = []Memory{}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"data": memories})
}

func (h *Handler) summarizeMemories(w http.ResponseWriter, r *http.Request) {
	storeName := strings.TrimSpace(r.URL.Query().Get("store_name"))
	storeType := strings.TrimSpace(r.URL.Query().Get("store_type"))
	if storeName == "" {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "store_name is required")
		return
	}
	memories, err := h.memories.GetLocationMemories(storeType, storeName)
	if err != nil {
		slog.Error("failed to get memories for summary", "store_type", storeType, "store_name", storeName, "error", err)
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get location memories")
		return
	}
	sort.SliceStable(memories, func(i, j int) bool {
		return memories[i].Timestamp.After(memories[j].Timestamp)
	})

	grocery := isGroceryStore(storeType)
	var pending []tasks.Task
	if grocery {
		pending, err = h.tasks.GetPendingTasks()
		if err != nil {
			slog.Error("failed to load pending tasks for grocery summary", "store_name", storeName, "error", err)
			httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to get pending tasks")
			return
		}
	}
	promptInput := any(memories)
	instruction := "Summarize these location memories concisely. For restaurants, highlight what the user likes or dislikes and what to order. Do not invent details."
	if grocery {
		promptInput = struct {
			StoreName string       `json:"store_name"`
			Tasks     []tasks.Task `json:"pending_tasks"`
			Memories  []Memory     `json:"store_memories,omitempty"`
		}{storeName, pending, memories}
		instruction = "Summarize the pending tasks as a concise shopping list for this grocery store. Do not invent items. Mention store memories if relevant. If there are no tasks, say there is nothing on the grocery list."
	}
	encoded, err := json.Marshal(promptInput)
	if err != nil {
		httpapi.WriteError(w, http.StatusInternalServerError, "internal_error", "failed to encode summary input")
		return
	}
	var summary string
	if h.agent != nil {
		response, chatErr := h.agent.SendRequest(r.Context(), []agent.ModelMessage{
			{Role: "system", Content: instruction},
			{Role: "user", Content: string(encoded)},
		})
		if chatErr == nil && len(response.Choices) > 0 {
			summary = strings.TrimSpace(response.Choices[0].Message.Content)
		} else if chatErr != nil {
			slog.Warn("location summary model request failed", "store_name", storeName, "error", chatErr)
		}
	}
	if summary == "" {
		if grocery {
			summary = groceryFallback(pending)
		} else {
			summary = memoryFallback(memories)
		}
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"data": map[string]string{"summary": summary}})
}

type Route struct {
	DistanceMeters int32  `json:"distance_meters"`
	Duration       string `json:"duration"`
}

type LocationUpdateResult struct {
	Routes             []Route `json:"routes"`
	TaskRecommendation string  `json:"task_recommendation"`
}

func (h *Handler) locationUpdate(w http.ResponseWriter, r *http.Request) {
	if h.workflow != nil {
		h.processLocationUpdate(w, r)
		return
	}
	var request struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
	}
	if err := httpapi.DecodeJSON(w, r, &request); err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if request.Latitude == nil || request.Longitude == nil {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "latitude and longitude are required")
		return
	}
	latitude, longitude := *request.Latitude, *request.Longitude
	if latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 || latitude == 0 && longitude == 0 {
		httpapi.WriteError(w, http.StatusBadRequest, "validation_error", "coordinates must be valid and cannot both be zero")
		return
	}
	routes, err := h.routes.CalculateRoutes(latitude, longitude)
	if err != nil {
		slog.Error("route calculation failed", "latitude", latitude, "longitude", longitude, "error", err)
		httpapi.WriteError(w, http.StatusBadGateway, "route_calculation_failed", "failed to calculate routes")
		return
	}
	result := LocationUpdateResult{
		Routes:             make([]Route, 0, len(routes)),
		TaskRecommendation: "not_evaluated",
	}
	shortest := 24 * time.Hour
	for _, route := range routes {
		if route == nil || route.Duration == nil {
			continue
		}
		duration := route.Duration.AsDuration()
		result.Routes = append(result.Routes, Route{DistanceMeters: route.DistanceMeters, Duration: duration.String()})
		if duration < shortest {
			shortest = duration
		}
	}
	if shortest <= 30*time.Minute {
		if shortest >= 200*time.Second {
			result.TaskRecommendation = "run"
		} else {
			result.TaskRecommendation = "do_not_run"
		}
		slog.Debug("calculated task recommendation from route duration",
			"shortest_route", shortest,
			"recommendation", result.TaskRecommendation,
		)
	}
	httpapi.WriteJSON(w, http.StatusOK, map[string]any{"data": result})
}

func isGroceryStore(storeType string) bool {
	normalized := strings.ToLower(strings.TrimSpace(storeType))
	normalized = strings.NewReplacer("_", " ", "-", " ").Replace(normalized)
	normalized = strings.Join(strings.Fields(normalized), " ")
	switch normalized {
	case "grocery", "groceries", "grocery list", "groceries list", "grocery store", "supermarket":
		return true
	default:
		return false
	}
}

func groceryFallback(pending []tasks.Task) string {
	if len(pending) == 0 {
		return "There is nothing on the grocery list."
	}
	items := make([]string, 0, len(pending))
	for _, task := range pending {
		item := task.Title
		if task.Description != "" {
			item += ": " + task.Description
		}
		items = append(items, item)
	}
	return strings.Join(items, "\n")
}

func memoryFallback(memories []Memory) string {
	if len(memories) == 0 {
		return "No memories are recorded for this location."
	}
	const limit = 5
	if len(memories) > limit {
		memories = memories[:limit]
	}
	items := make([]string, 0, len(memories))
	for _, memory := range memories {
		items = append(items, memory.Message)
	}
	return strings.Join(items, "\n")
}
