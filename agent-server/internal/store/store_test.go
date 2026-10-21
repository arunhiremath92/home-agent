package store

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/aruhiremath92/agent-server/internal/location"
	"github.com/aruhiremath92/agent-server/internal/notes"
	"github.com/aruhiremath92/agent-server/internal/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type fakeDbConn struct {
	queryRowFn func(string, ...interface{}) pgx.Row
	queryFn    func(string, ...interface{}) (pgx.Rows, error)
	execFn     func(string, ...interface{}) (pgconn.CommandTag, error)
	query      string
	args       []interface{}
}

func (db *fakeDbConn) Connect(context.Context) error { return nil }
func (db *fakeDbConn) Close(context.Context) error   { return nil }
func (db *fakeDbConn) Ping(context.Context) error    { return nil }
func (db *fakeDbConn) Migrate(context.Context) error { return nil }
func (db *fakeDbConn) Exec(_ context.Context, query string, args ...interface{}) (pgconn.CommandTag, error) {
	db.query, db.args = query, args
	if db.execFn == nil {
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	return db.execFn(query, args...)
}
func (db *fakeDbConn) Query(_ context.Context, query string, args ...interface{}) (pgx.Rows, error) {
	db.query, db.args = query, args
	if db.queryFn == nil {
		return nil, errors.New("unexpected query")
	}
	return db.queryFn(query, args...)
}
func (db *fakeDbConn) QueryRow(_ context.Context, query string, args ...interface{}) pgx.Row {
	db.query, db.args = query, args
	return db.queryRowFn(query, args...)
}

type fakeRow struct {
	scanFn func(...interface{}) error
}

func (r fakeRow) Scan(dest ...interface{}) error { return r.scanFn(dest...) }

type fakeRows struct {
	count  int
	index  int
	scanFn func(int, ...interface{}) error
	err    error
}

func (r *fakeRows) Close()                                       {}
func (r *fakeRows) Err() error                                   { return r.err }
func (r *fakeRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *fakeRows) Next() bool                                   { return r.index < r.count }
func (r *fakeRows) Values() ([]interface{}, error)               { return nil, nil }
func (r *fakeRows) RawValues() [][]byte                          { return nil }
func (r *fakeRows) Conn() *pgx.Conn                              { return nil }
func (r *fakeRows) TypeMap() *pgtype.Map                         { return nil }
func (r *fakeRows) Scan(dest ...interface{}) error {
	index := r.index
	r.index++
	return r.scanFn(index, dest...)
}

func TestCreateNote(t *testing.T) {
	created := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	db := &fakeDbConn{queryRowFn: func(_ string, args ...interface{}) pgx.Row {
		if !reflect.DeepEqual(args, []interface{}{"A note"}) {
			t.Errorf("insert args = %#v", args)
		}
		return fakeRow{scanFn: func(dest ...interface{}) error {
			*dest[0].(*int) = 11
			*dest[1].(*string) = "A note"
			*dest[2].(*time.Time) = created
			return nil
		}}
	}}
	got, err := (&Store{db: db}).CreateNote("A note")
	if err != nil || got.ID != 11 || got.Content != "A note" || !got.CreatedAt.Equal(created) {
		t.Fatalf("CreateNote() = %+v, %v", got, err)
	}
}

func TestDeleteNoteReturnsNotFound(t *testing.T) {
	db := &fakeDbConn{execFn: func(string, ...interface{}) (pgconn.CommandTag, error) {
		return pgconn.NewCommandTag("DELETE 0"), nil
	}}
	if err := (&Store{db: db}).DeleteNote(999); !errors.Is(err, notes.ErrNotFound) {
		t.Fatalf("DeleteNote() error = %v, want ErrNotFound", err)
	}
}

func TestAddTask(t *testing.T) {
	dueAt := time.Date(2026, 10, 17, 9, 30, 0, 0, time.UTC)
	db := &fakeDbConn{queryRowFn: func(_ string, args ...interface{}) pgx.Row {
		if len(args) != 3 || args[0] != "Buy milk" || args[1] != "2%" || args[2] != &dueAt {
			t.Errorf("insert args = %#v", args)
		}
		return fakeRow{scanFn: func(dest ...interface{}) error {
			*dest[0].(*int) = 42
			*dest[1].(*string) = "Buy milk"
			*dest[2].(*string) = "2%"
			*dest[3].(*sql.NullTime) = sql.NullTime{Time: dueAt, Valid: true}
			*dest[4].(*string) = "pending"
			*dest[5].(*time.Time) = dueAt
			return nil
		}}
	}}
	got, err := (&Store{db: db}).AddTask(tasks.Task{Title: " Buy milk ", Description: "2%", DueAt: &dueAt})
	if err != nil || got.ID != 42 || got.Title != "Buy milk" || got.DueAt == nil || !got.DueAt.Equal(dueAt) {
		t.Fatalf("AddTask() = %+v, %v", got, err)
	}
}

func TestGetLocationMemories(t *testing.T) {
	timestamp := time.Date(2026, 10, 17, 8, 0, 0, 0, time.UTC)
	db := &fakeDbConn{queryFn: func(query string, args ...interface{}) (pgx.Rows, error) {
		if !strings.Contains(query, "FROM location_based_info") || !reflect.DeepEqual(args, []interface{}{"restaurant", "Cafe"}) {
			t.Errorf("query/args = %q %#v", query, args)
		}
		return &fakeRows{count: 1, scanFn: func(_ int, dest ...interface{}) error {
			*dest[0].(*int) = 5
			*dest[1].(*string) = "restaurant"
			*dest[2].(*string) = "Cafe"
			*dest[3].(*float64) = 40.7
			*dest[4].(*float64) = -74.0
			*dest[5].(*time.Time) = timestamp
			*dest[6].(*string) = "great coffee"
			return nil
		}}, nil
	}}
	got, err := (&Store{db: db}).GetLocationMemories("restaurant", "Cafe")
	if err != nil || len(got) != 1 || got[0].ID != 5 || got[0].Message != "great coffee" || !got[0].Timestamp.Equal(timestamp) {
		t.Fatalf("GetLocationMemories() = %+v, %v", got, err)
	}
}

func TestSaveShortcut(t *testing.T) {
	updated := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	db := &fakeDbConn{queryRowFn: func(query string, args ...interface{}) pgx.Row {
		if !strings.Contains(query, "ON CONFLICT (name)") ||
			!reflect.DeepEqual(args, []interface{}{"Start Roomba", "Start the robot vacuum"}) {
			t.Errorf("query/args = %q %#v", query, args)
		}
		return fakeRow{scanFn: func(dest ...interface{}) error {
			*dest[0].(*string) = "Start Roomba"
			*dest[1].(*string) = "Start the robot vacuum"
			*dest[2].(*time.Time) = updated
			return nil
		}}
	}}
	got, err := (&Store{db: db}).SaveShortcut(location.Shortcut{
		Name: "Start Roomba", Description: "Start the robot vacuum",
	})
	if err != nil || got.Name != "Start Roomba" || !got.UpdatedAt.Equal(updated) {
		t.Fatalf("SaveShortcut() = %+v, %v", got, err)
	}
}

func TestSaveHomeLocation(t *testing.T) {
	updated := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	db := &fakeDbConn{queryRowFn: func(query string, args ...interface{}) pgx.Row {
		if !strings.Contains(query, "ON CONFLICT (id)") ||
			!reflect.DeepEqual(args, []interface{}{40.7, -74.0, 150}) {
			t.Errorf("query/args = %q %#v", query, args)
		}
		return fakeRow{scanFn: func(dest ...interface{}) error {
			*dest[0].(*float64) = 40.7
			*dest[1].(*float64) = -74.0
			*dest[2].(*int) = 150
			*dest[3].(*time.Time) = updated
			return nil
		}}
	}}
	got, err := (&Store{db: db}).SaveHomeLocation(location.HomeLocation{
		Latitude: 40.7, Longitude: -74.0, RadiusMeters: 150,
	})
	if err != nil || got.Latitude != 40.7 || got.Longitude != -74.0 ||
		got.RadiusMeters != 150 || !got.UpdatedAt.Equal(updated) {
		t.Fatalf("SaveHomeLocation() = %+v, %v", got, err)
	}
}

func TestSaveAndDeleteLocationHistory(t *testing.T) {
	created := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	db := &fakeDbConn{queryRowFn: func(query string, args ...interface{}) pgx.Row {
		if !strings.Contains(query, "INSERT INTO location_update_history") ||
			args[0] != 40.7 || args[1] != -74.0 ||
			!reflect.DeepEqual(args[2], []byte(`{"message":"hello"}`)) {
			t.Errorf("query/args = %q %#v", query, args)
		}
		return fakeRow{scanFn: func(dest ...interface{}) error {
			*dest[0].(*int) = 9
			*dest[1].(*time.Time) = created
			return nil
		}}
	}}
	got, err := (&Store{db: db}).SaveLocationHistory(location.LocationHistory{
		Latitude: 40.7, Longitude: -74.0, Response: []byte(`{"message":"hello"}`),
	})
	if err != nil || got.ID != 9 || !got.CreatedAt.Equal(created) {
		t.Fatalf("SaveLocationHistory() = %+v, %v", got, err)
	}

	db.execFn = func(query string, args ...interface{}) (pgconn.CommandTag, error) {
		if !strings.Contains(query, "DELETE FROM location_update_history") ||
			!reflect.DeepEqual(args, []interface{}{9}) {
			t.Errorf("query/args = %q %#v", query, args)
		}
		return pgconn.NewCommandTag("DELETE 1"), nil
	}
	if err := (&Store{db: db}).DeleteLocationHistory(9); err != nil {
		t.Fatalf("DeleteLocationHistory() error = %v", err)
	}
}
