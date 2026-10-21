package tasks

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRepository struct {
	items []Task
}

func (r *fakeRepository) AddTask(task Task) (Task, error) {
	task.ID = len(r.items) + 1
	task.Status = "pending"
	r.items = append(r.items, task)
	return task, nil
}
func (r *fakeRepository) ListTasks(status string) ([]Task, error) {
	if status == "" {
		return r.items, nil
	}
	result := make([]Task, 0)
	for _, task := range r.items {
		if task.Status == status {
			result = append(result, task)
		}
	}
	return result, nil
}
func (r *fakeRepository) GetPendingTasks() ([]Task, error) { return r.ListTasks("pending") }

func TestTaskRESTEndpoints(t *testing.T) {
	repository := &fakeRepository{}
	mux := http.NewServeMux()
	NewHandler(repository).RegisterRoutes(mux)
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, httptest.NewRequest(http.MethodPost, "/api/v1/tasks",
		strings.NewReader(`{"title":" Buy milk ","description":"2%"}`)))
	if create.Code != http.StatusCreated || !strings.Contains(create.Body.String(), `"title":"Buy milk"`) {
		t.Fatalf("create response status/body = %d/%s", create.Code, create.Body.String())
	}
	list := httptest.NewRecorder()
	mux.ServeHTTP(list, httptest.NewRequest(http.MethodGet, "/api/v1/tasks?status=pending", nil))
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), `"title":"Buy milk"`) {
		t.Fatalf("list response status/body = %d/%s", list.Code, list.Body.String())
	}
}
