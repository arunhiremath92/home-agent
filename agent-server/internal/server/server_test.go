package server

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
	"github.com/aruhiremath92/agent-server/internal/location"
	"github.com/aruhiremath92/agent-server/internal/notes"
	"github.com/aruhiremath92/agent-server/internal/tasks"
	"google.golang.org/protobuf/types/known/durationpb"
)

type fakeStore struct {
	notes        []notes.Note
	tasks        []tasks.Task
	memories     []location.Memory
	pending      []tasks.Task
	shortcuts    []location.Shortcut
	home         *location.HomeLocation
	history      []location.LocationHistory
	storeErr     error
	nextNoteID   int
	nextTaskID   int
	nextMemoryID int
}

func (s *fakeStore) CreateNote(content string) (notes.Note, error) {
	s.nextNoteID++
	note := notes.Note{ID: s.nextNoteID, Content: content, CreatedAt: time.Now()}
	s.notes = append(s.notes, note)
	return note, s.storeErr
}
func (s *fakeStore) ListNotes() ([]notes.Note, error) { return s.notes, s.storeErr }
func (s *fakeStore) DeleteNote(id int) error {
	for i, note := range s.notes {
		if note.ID == id {
			s.notes = append(s.notes[:i], s.notes[i+1:]...)
			return s.storeErr
		}
	}
	return notes.ErrNotFound
}
func (s *fakeStore) AddTask(task tasks.Task) (tasks.Task, error) {
	s.nextTaskID++
	task.ID, task.Status, task.CreatedAt = s.nextTaskID, "pending", time.Now()
	s.tasks = append(s.tasks, task)
	return task, s.storeErr
}
func (s *fakeStore) ListTasks(status string) ([]tasks.Task, error) {
	if status == "" {
		return s.tasks, s.storeErr
	}
	var result []tasks.Task
	for _, task := range s.tasks {
		if task.Status == status {
			result = append(result, task)
		}
	}
	return result, s.storeErr
}
func (s *fakeStore) GetPendingTasks() ([]tasks.Task, error) { return s.pending, s.storeErr }
func (s *fakeStore) AddLocationMemory(memory location.Memory) (location.Memory, error) {
	s.nextMemoryID++
	memory.ID = s.nextMemoryID
	s.memories = append(s.memories, memory)
	return memory, s.storeErr
}
func (s *fakeStore) GetLocationMemories(storeType, storeName string) ([]location.Memory, error) {
	return s.memories, s.storeErr
}
func (s *fakeStore) ListShortcuts() ([]location.Shortcut, error) {
	return s.shortcuts, s.storeErr
}
func (s *fakeStore) SaveShortcut(shortcut location.Shortcut) (location.Shortcut, error) {
	for i := range s.shortcuts {
		if s.shortcuts[i].Name == shortcut.Name {
			s.shortcuts[i] = shortcut
			return shortcut, s.storeErr
		}
	}
	s.shortcuts = append(s.shortcuts, shortcut)
	return shortcut, s.storeErr
}
func (s *fakeStore) DeleteShortcut(name string) error {
	for i, shortcut := range s.shortcuts {
		if shortcut.Name == name {
			s.shortcuts = append(s.shortcuts[:i], s.shortcuts[i+1:]...)
			return s.storeErr
		}
	}
	return location.ErrShortcutNotFound
}
func (s *fakeStore) GetHomeLocation() (location.HomeLocation, bool, error) {
	if s.home == nil {
		return location.HomeLocation{}, false, s.storeErr
	}
	return *s.home, true, s.storeErr
}
func (s *fakeStore) SaveHomeLocation(home location.HomeLocation) (location.HomeLocation, error) {
	s.home = &home
	return home, s.storeErr
}
func (s *fakeStore) SaveLocationHistory(event location.LocationHistory) (location.LocationHistory, error) {
	event.ID = len(s.history) + 1
	event.CreatedAt = time.Now().UTC()
	s.history = append(s.history, event)
	return event, s.storeErr
}
func (s *fakeStore) ListLocationHistory(limit int) ([]location.LocationHistory, error) {
	if limit < len(s.history) {
		return s.history[len(s.history)-limit:], s.storeErr
	}
	return s.history, s.storeErr
}
func (s *fakeStore) DeleteLocationHistory(id int) error {
	for i, event := range s.history {
		if event.ID == id {
			s.history = append(s.history[:i], s.history[i+1:]...)
			return s.storeErr
		}
	}
	return location.ErrLocationHistoryNotFound
}

type fakeRoutes struct {
	routes    []*routespb.Route
	err       error
	lat       float64
	lng       float64
	originLat float64
	originLng float64
}

func (r *fakeRoutes) CalculateRoutes(lat, lng float64) ([]*routespb.Route, error) {
	r.lat, r.lng = lat, lng
	return r.routes, r.err
}
func (r *fakeRoutes) CalculateRoutesFrom(originLat, originLng, lat, lng float64) ([]*routespb.Route, error) {
	r.originLat, r.originLng = originLat, originLng
	r.lat, r.lng = lat, lng
	return r.routes, r.err
}

type fakeAgent struct {
	response  agent.ModelResponse
	err       error
	responses []string
	prompts   []string
}

func (a *fakeAgent) SendRequest(_ context.Context, messages []agent.ModelMessage) (agent.ModelResponse, error) {
	var prompt strings.Builder
	for _, message := range messages {
		prompt.WriteString(message.Content)
		prompt.WriteByte('\n')
	}
	a.prompts = append(a.prompts, prompt.String())
	if len(a.responses) > 0 {
		content := a.responses[0]
		a.responses = a.responses[1:]
		return agent.ModelResponse{Choices: []agent.ModelChoice{{
			Message: agent.ModelMessage{Content: content},
		}}}, a.err
	}
	return a.response, a.err
}

type fakePlaces struct {
	resolved     location.Place
	nearby       []location.Place
	resolveCalls int
	searchCalls  int
	searchTypes  []string
	err          error
}

func (p *fakePlaces) ResolveLocation(_ context.Context, lat, lng float64) (location.Place, error) {
	p.resolveCalls++
	p.resolved.Latitude, p.resolved.Longitude = lat, lng
	return p.resolved, p.err
}
func (p *fakePlaces) SearchNearby(_ context.Context, _, _ float64, types []string) ([]location.Place, error) {
	p.searchCalls++
	p.searchTypes = types
	return p.nearby, p.err
}

func TestNewServerRequiresDependencies(t *testing.T) {
	routeCalculator := &fakeRoutes{}
	if _, err := NewServer(nil, routeCalculator); err == nil {
		t.Fatal("NewServer(nil, routeCalculator) expected an error")
	}
	if _, err := NewServer(&fakeStore{}, nil); err == nil {
		t.Fatal("NewServer(store, nil) expected an error")
	}
}

func TestDashboardServesEmbeddedAssets(t *testing.T) {
	server, err := NewServerWithAgent(&fakeStore{}, &fakeRoutes{}, &fakeAgent{})
	if err != nil {
		t.Fatalf("NewServerWithAgent() error = %v", err)
	}
	for _, path := range []string{"/", "/app.css", "/app.js"} {
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Errorf("GET %s status = %d, want %d", path, response.Code, http.StatusOK)
		}
		if got := response.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s Cache-Control = %q, want no-store", path, got)
		}
	}
}

func TestRESTRoutesAndAgentCRUDRemoved(t *testing.T) {
	repository := &fakeStore{}
	routes := &fakeRoutes{}
	server, err := NewServerWithAgent(repository, routes, &fakeAgent{})
	if err != nil {
		t.Fatalf("NewServerWithAgent() error = %v", err)
	}

	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodGet, "/healthz", "", http.StatusOK},
		{http.MethodPost, "/api/v1/notes", `{"content":"call Sam"}`, http.StatusCreated},
		{http.MethodGet, "/api/v1/notes", "", http.StatusOK},
		{http.MethodPost, "/api/v1/tasks", `{"title":"buy milk"}`, http.StatusCreated},
		{http.MethodGet, "/api/v1/tasks?status=pending", "", http.StatusOK},
		{http.MethodPost, "/api/v1/location-memories", `{"store_type":"restaurant","store_name":"Cafe","latitude":40.7,"longitude":-74.0,"message":"great latte"}`, http.StatusCreated},
		{http.MethodGet, "/api/v1/location-memories?store_name=Cafe", "", http.StatusOK},
		{http.MethodPost, "/add-agent", `{}`, http.StatusNotFound},
		{http.MethodPost, "/event", `{}`, http.StatusNotFound},
		{http.MethodPost, "/healthz", "", http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			response := httptest.NewRecorder()
			server.Handler().ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.status, response.Body.String())
			}
			if test.status == http.StatusNotFound || test.status == http.StatusMethodNotAllowed {
				if response.Header().Get("Content-Type") != "application/json" ||
					!strings.Contains(response.Body.String(), `"error"`) {
					t.Errorf("error response should use the standard JSON envelope; headers=%v body=%s",
						response.Header(), response.Body.String())
				}
			}
		})
	}
}

func TestLocationUpdateReturnsTypedRoutes(t *testing.T) {
	routes := &fakeRoutes{routes: []*routespb.Route{{
		DistanceMeters: 3210,
		Duration:       durationpb.New(10 * time.Minute),
	}}}
	mockAgent := &fakeAgent{response: agent.ModelResponse{Choices: []agent.ModelChoice{{
		Message: agent.ModelMessage{Content: "yes"},
	}}}}
	server, err := NewServerWithAgent(&fakeStore{}, routes, mockAgent)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/location-updates",
		strings.NewReader(`{"latitude":40.7,"longitude":-74.0}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusOK, response.Body.String())
	}
	if routes.lat != 40.7 || routes.lng != -74.0 {
		t.Errorf("route request coordinates = %f,%f", routes.lat, routes.lng)
	}
	if !strings.Contains(response.Body.String(), `"task_recommendation":"run"`) ||
		!strings.Contains(response.Body.String(), `"distance_meters":3210`) {
		t.Errorf("response = %s, want typed route data and recommendation", response.Body.String())
	}
}

func TestLocationUpdateRejectsMissingCoordinates(t *testing.T) {
	server, err := NewServerWithAgent(&fakeStore{}, &fakeRoutes{}, &fakeAgent{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/location-updates", strings.NewReader(`{}`))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
}

func TestLocationSummaryFallsBackOnModelFailure(t *testing.T) {
	repository := &fakeStore{
		memories: []location.Memory{{Message: "Great croissants"}},
	}
	server, err := NewServerWithAgent(repository, &fakeRoutes{}, &fakeAgent{err: errors.New("offline")})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet,
		"/api/v1/location-memories/summary?store_type=restaurant&store_name=Cafe", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Great croissants") {
		t.Fatalf("status/body = %d/%s, want successful fallback summary", response.Code, response.Body.String())
	}
}

func TestLocationWorkflowRegistersShortcutsConfiguresHomeAndMaintainsHistory(t *testing.T) {
	repository := &fakeStore{
		pending: []tasks.Task{{ID: 4, Title: "Run Roomba"}},
		shortcuts: []location.Shortcut{{
			Name: "Start Roomba", Description: "Start the robot vacuum when home",
		}},
		home: &location.HomeLocation{Latitude: 40.7, Longitude: -74.0, RadiusMeters: 150},
	}
	places := &fakePlaces{
		resolved: location.Place{Name: "Neighborhood", Type: "locality"},
		nearby:   []location.Place{{Name: "Best Buy", Type: "electronics_store"}},
	}
	model := &fakeAgent{responses: []string{
		`{"place_types":["electronics_store","bogus_type"]}`,
		`{"message":"You are home. You can start the Roomba.","suggested_shortcuts":[{"shortcutName":"Start Roomba","reason":"You are home."},{"shortcutName":"Invented shortcut","reason":"not registered"}]}`,
	}}
	routes := &fakeRoutes{}
	s, err := NewWorkflowServer(repository, routes, model, places)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/location-updates",
		strings.NewReader(`{"latitude":40.7,"longitude":-74.0}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("location update status/body = %d/%s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{
		`"at_home":true`,
		`"type":"residential"`,
		`"action_type":"start_robot_vacuum"`,
		`"estimated_minutes":45`,
		`"source":"mock"`,
		`"shortcutName":"Start Roomba"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("location response does not contain %s: %s", want, body)
		}
	}
	if strings.Contains(body, "Invented shortcut") {
		t.Errorf("response included an unregistered shortcut: %s", body)
	}
	if places.resolveCalls != 0 || places.searchCalls != 1 ||
		len(places.searchTypes) != 1 || places.searchTypes[0] != "electronics_store" {
		t.Errorf("Google place calls resolve/search/types = %d/%d/%v", places.resolveCalls, places.searchCalls, places.searchTypes)
	}
	if routes.originLat != 40.7 || routes.originLng != -74.0 {
		t.Errorf("route origin = %f,%f; want configured home coordinates", routes.originLat, routes.originLng)
	}
	if len(repository.history) != 1 {
		t.Fatalf("history count = %d, want 1", len(repository.history))
	}
	historyRequest := httptest.NewRequest(http.MethodGet, "/api/v1/location-updates/history", nil)
	historyResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(historyResponse, historyRequest)
	if historyResponse.Code != http.StatusOK || !strings.Contains(historyResponse.Body.String(), `"id":1`) {
		t.Fatalf("history response = %d %s", historyResponse.Code, historyResponse.Body.String())
	}
	deleteRequest := httptest.NewRequest(http.MethodDelete, "/api/v1/location-updates/history/1", nil)
	deleteResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(deleteResponse, deleteRequest)
	if deleteResponse.Code != http.StatusNoContent || len(repository.history) != 0 {
		t.Fatalf("delete history response = %d %s, remaining=%d", deleteResponse.Code, deleteResponse.Body.String(), len(repository.history))
	}
}

func TestLocationWorkflowConfiguresHomeAndShortcut(t *testing.T) {
	repository := &fakeStore{}
	s, err := NewWorkflowServer(repository, &fakeRoutes{}, &fakeAgent{}, &fakePlaces{})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		method string
		path   string
		body   string
		want   int
	}{
		{http.MethodPut, "/api/v1/settings/home-location", `{"latitude":40.7,"longitude":-74.0}`, http.StatusOK},
		{http.MethodPost, "/api/v1/shortcuts", `{"name":"Start Roomba","description":"Run the robot vacuum"}`, http.StatusOK},
		{http.MethodGet, "/api/v1/settings/home-location", "", http.StatusOK},
		{http.MethodGet, "/api/v1/shortcuts", "", http.StatusOK},
		{http.MethodDelete, "/api/v1/shortcuts/Start%20Roomba", "", http.StatusNoContent},
	} {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, request)
		if response.Code != test.want {
			t.Errorf("%s %s = %d %s, want %d", test.method, test.path, response.Code, response.Body.String(), test.want)
		}
	}

	if repository.home == nil || repository.home.RadiusMeters != 150 {
		t.Errorf("saved home location = %+v, want default 150m radius", repository.home)
	}
}

func TestLocationWorkflowSearchesForTaskCategoriesEvenWhenModelOmitsThem(t *testing.T) {
	repository := &fakeStore{pending: []tasks.Task{{ID: 1, Title: "Buy milk"}}}
	places := &fakePlaces{resolved: location.Place{Name: "Market Road", Type: "route"}}
	model := &fakeAgent{responses: []string{
		`{"place_types":[]}`,
		`{"message":"There are nearby grocery stores for your milk task.","suggested_shortcuts":[]}`,
	}}
	s, err := NewWorkflowServer(repository, &fakeRoutes{}, model, places)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/location-updates",
		strings.NewReader(`{"latitude":40.7,"longitude":-74.0}`))
	response := httptest.NewRecorder()
	s.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("location update = %d %s", response.Code, response.Body.String())
	}
	if places.searchCalls != 1 ||
		!strings.Contains(strings.Join(places.searchTypes, ","), "grocery_store") ||
		!strings.Contains(strings.Join(places.searchTypes, ","), "supermarket") {
		t.Errorf("search calls/types = %d/%v", places.searchCalls, places.searchTypes)
	}
}
