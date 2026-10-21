package location

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	routespb "cloud.google.com/go/maps/routing/apiv2/routingpb"
	"github.com/aruhiremath92/agent-server/agent"
	"github.com/aruhiremath92/agent-server/internal/tasks"
	"google.golang.org/protobuf/types/known/durationpb"
)

type fakeRepository struct {
	memories []Memory
	pending  []tasks.Task
	err      error
}

func (r *fakeRepository) AddLocationMemory(memory Memory) (Memory, error) {
	memory.ID = len(r.memories) + 1
	r.memories = append(r.memories, memory)
	return memory, r.err
}
func (r *fakeRepository) GetLocationMemories(string, string) ([]Memory, error) {
	return r.memories, r.err
}
func (r *fakeRepository) GetPendingTasks() ([]tasks.Task, error) { return r.pending, r.err }

type fakeRouteCalculator struct {
	routes []*routespb.Route
	err    error
}

func (r *fakeRouteCalculator) CalculateRoutes(float64, float64) ([]*routespb.Route, error) {
	return r.routes, r.err
}

type fakeChatAgent struct {
	response agent.ModelResponse
	err      error
}

func (a *fakeChatAgent) SendRequest(context.Context, []agent.ModelMessage) (agent.ModelResponse, error) {
	return a.response, a.err
}

func TestCreateAndListLocationMemories(t *testing.T) {
	repository := &fakeRepository{}
	mux := http.NewServeMux()
	NewHandler(repository, repository, &fakeRouteCalculator{}, &fakeChatAgent{}).RegisterRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/location-memories", strings.NewReader(
		`{"store_type":"restaurant","store_name":"Cafe","latitude":40.7,"longitude":-74.0,"message":"Great croissants"}`)))
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), `"message":"Great croissants"`) {
		t.Fatalf("create response = %d %s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/location-memories?store_name=Cafe", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), "Great croissants") {
		t.Fatalf("list response = %d %s", list.Code, list.Body.String())
	}
}

func TestLocationSummaryGroceryFallback(t *testing.T) {
	repository := &fakeRepository{pending: []tasks.Task{
		{Title: "Milk", Description: "2%"},
	}}
	mux := http.NewServeMux()
	NewHandler(repository, repository, &fakeRouteCalculator{}, &fakeChatAgent{err: errors.New("offline")}).RegisterRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		"/api/v1/location-memories/summary?store_type=grocery&store_name=Market", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Milk: 2%") {
		t.Fatalf("summary response = %d %s", response.Code, response.Body.String())
	}
}

func TestLocationUpdateTypedResponse(t *testing.T) {
	routes := &fakeRouteCalculator{routes: []*routespb.Route{{
		DistanceMeters: 2700,
		Duration:       durationpb.New(2 * time.Minute),
	}}}
	chat := &fakeChatAgent{response: agent.ModelResponse{Choices: []agent.ModelChoice{{
		Message: agent.ModelMessage{Content: "No"},
	}}}}
	mux := http.NewServeMux()
	NewHandler(&fakeRepository{}, &fakeRepository{}, routes, chat).RegisterRoutes(mux)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/location-updates",
		strings.NewReader(`{"latitude":40.7,"longitude":-74.0}`)))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"distance_meters":2700`) ||
		!strings.Contains(response.Body.String(), `"task_recommendation":"do_not_run"`) {
		t.Fatalf("location update response = %d %s", response.Code, response.Body.String())
	}
}

func TestTaskRecommendationUsesFixedDurationThreshold(t *testing.T) {
	for _, test := range []struct {
		name      string
		duration  time.Duration
		recommend string
	}{
		{name: "below task duration", duration: 199 * time.Second, recommend: "do_not_run"},
		{name: "enough time", duration: 200 * time.Second, recommend: "run"},
		{name: "outside recommendation window", duration: 31 * time.Minute, recommend: "not_evaluated"},
	} {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			NewHandler(&fakeRepository{}, &fakeRepository{},
				&fakeRouteCalculator{routes: []*routespb.Route{{
					Duration: durationpb.New(test.duration),
				}}},
				&fakeChatAgent{err: errors.New("should not be called")},
			).RegisterRoutes(mux)
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/location-updates",
				strings.NewReader(`{"latitude":40.7,"longitude":-74.0}`)))
			if response.Code != http.StatusOK || !strings.Contains(response.Body.String(),
				`"task_recommendation":"`+test.recommend+`"`) {
				t.Fatalf("response = %d %s, want recommendation %q", response.Code, response.Body.String(), test.recommend)
			}
		})
	}
}
