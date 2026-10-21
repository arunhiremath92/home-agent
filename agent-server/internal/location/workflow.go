package location

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aruhiremath92/agent-server/internal/tasks"
)

var ErrShortcutNotFound = errors.New("shortcut not found")
var ErrLocationHistoryNotFound = errors.New("location history entry not found")

type Shortcut struct {
	Name        string    `json:"name"`
	Description string    `json:"description"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type HomeLocation struct {
	Latitude     float64   `json:"latitude"`
	Longitude    float64   `json:"longitude"`
	RadiusMeters int       `json:"radius_meters"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type LocationHistory struct {
	ID        int             `json:"id"`
	CreatedAt time.Time       `json:"created_at"`
	Latitude  float64         `json:"latitude"`
	Longitude float64         `json:"longitude"`
	Response  json.RawMessage `json:"response"`
}

type Place struct {
	Name          string   `json:"name"`
	Type          string   `json:"type,omitempty"`
	Address       string   `json:"address,omitempty"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	GoogleMapsURL string   `json:"google_maps_url,omitempty"`
	AddressTypes  []string `json:"address_types,omitempty"`
}

type TaskAction struct {
	TaskID          int    `json:"task_id"`
	TaskTitle       string `json:"task_title"`
	ActionType      string `json:"action_type"`
	EstimatedMinute int    `json:"estimated_minutes"`
	Source          string `json:"source"`
}

type ShortcutSuggestion struct {
	ShortcutName string         `json:"shortcutName"`
	Reason       string         `json:"reason"`
	Input        map[string]any `json:"input,omitempty"`
}

type LocationUpdateResponse struct {
	Location           Place                `json:"location"`
	AtHome             bool                 `json:"at_home"`
	NearbyPlaces       []Place              `json:"nearby_places"`
	PendingTaskActions []TaskAction         `json:"pending_task_actions"`
	SuggestedShortcuts []ShortcutSuggestion `json:"suggested_shortcuts"`
	Message            string               `json:"message"`
	DecisionSource     string               `json:"decision_source"`
	Routes             []Route              `json:"routes"`
	TaskRecommendation string               `json:"task_recommendation"`
	Warnings           []string             `json:"warnings,omitempty"`
}

type WorkflowRepository interface {
	ListShortcuts() ([]Shortcut, error)
	SaveShortcut(Shortcut) (Shortcut, error)
	DeleteShortcut(name string) error
	GetHomeLocation() (HomeLocation, bool, error)
	SaveHomeLocation(HomeLocation) (HomeLocation, error)
	SaveLocationHistory(LocationHistory) (LocationHistory, error)
	ListLocationHistory(limit int) ([]LocationHistory, error)
	DeleteLocationHistory(id int) error
}

type PlaceFinder interface {
	ResolveLocation(context.Context, float64, float64) (Place, error)
	SearchNearby(context.Context, float64, float64, []string) ([]Place, error)
}

type ActionEstimator interface {
	Estimate([]tasks.Task, bool) []TaskAction
}

type workflowPlan struct {
	Message            string               `json:"message"`
	SuggestedShortcuts []ShortcutSuggestion `json:"suggested_shortcuts"`
}
