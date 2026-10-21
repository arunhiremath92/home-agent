package notes

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeRepository struct {
	items []Note
	err   error
}

func (r *fakeRepository) CreateNote(content string) (Note, error) {
	note := Note{ID: 1, Content: content}
	r.items = append(r.items, note)
	return note, r.err
}
func (r *fakeRepository) ListNotes() ([]Note, error) { return r.items, r.err }
func (r *fakeRepository) DeleteNote(id int) error {
	if r.err != nil {
		return r.err
	}
	if id != 1 || len(r.items) == 0 {
		return ErrNotFound
	}
	r.items = nil
	return nil
}

func TestNotesRESTEndpoints(t *testing.T) {
	repository := &fakeRepository{}
	mux := http.NewServeMux()
	NewHandler(repository).RegisterRoutes(mux)
	tests := []struct {
		method string
		path   string
		body   string
		status int
	}{
		{http.MethodPost, "/api/v1/notes", `{"content":"Remember milk"}`, http.StatusCreated},
		{http.MethodGet, "/api/v1/notes", "", http.StatusOK},
		{http.MethodDelete, "/api/v1/notes/1", "", http.StatusNoContent},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		if response.Code != test.status {
			t.Errorf("%s %s status=%d body=%s, want %d", test.method, test.path, response.Code, response.Body.String(), test.status)
		}
	}
}

func TestCreateNoteValidatesAndReportsRepositoryFailure(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
		err  error
		want int
	}{
		{"blank note", `{"content":" "}`, nil, http.StatusBadRequest},
		{"invalid JSON", `{`, nil, http.StatusBadRequest},
		{"repository error", `{"content":"hello"}`, errors.New("database down"), http.StatusInternalServerError},
	} {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			NewHandler(&fakeRepository{err: test.err}).RegisterRoutes(mux)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/notes", strings.NewReader(test.body))
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d", response.Code, test.want)
			}
			if response.Header().Get("Content-Type") != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", response.Header().Get("Content-Type"))
			}
		})
	}
}
