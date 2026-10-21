package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aruhiremath92/agent-server/internal/db"
	"github.com/aruhiremath92/agent-server/internal/location"
	"github.com/aruhiremath92/agent-server/internal/notes"
	"github.com/aruhiremath92/agent-server/internal/tasks"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type DbConn interface {
	Connect(ctx context.Context) error
	Close(ctx context.Context) error
	Ping(ctx context.Context) error
	Exec(ctx context.Context, sql string, arguments ...interface{}) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Migrate(ctx context.Context) error
}

type Store struct {
	db DbConn
}

func NewStore(ctx context.Context) (*Store, error) {
	connection := db.NewDb()
	if err := connection.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	store := &Store{db: connection}
	if err := store.setup(); err != nil {
		_ = connection.Close(ctx)
		return nil, fmt.Errorf("failed to setup database: %w", err)
	}
	if err := store.migrate(ctx); err != nil {
		_ = connection.Close(ctx)
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}
	return store, nil
}

func (s *Store) Close() {
	if err := s.db.Close(context.Background()); err != nil {
		slog.Error("failed to close database", "error", err)
	}
}

func (s *Store) Ping() error {
	if err := s.db.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	return s.db.Migrate(ctx)
}

func (s *Store) CreateNote(content string) (notes.Note, error) {
	var note notes.Note
	err := s.db.QueryRow(context.Background(),
		`INSERT INTO notes (content) VALUES ($1) RETURNING id, content, created_at`,
		content,
	).Scan(&note.ID, &note.Content, &note.CreatedAt)
	if err != nil {
		return notes.Note{}, fmt.Errorf("failed to insert note: %w", err)
	}
	return note, nil
}

func (s *Store) ListNotes() ([]notes.Note, error) {
	rows, err := s.db.Query(context.Background(),
		`SELECT id, content, created_at FROM notes ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query notes: %w", err)
	}
	defer rows.Close()
	result := make([]notes.Note, 0)
	for rows.Next() {
		var note notes.Note
		if err := rows.Scan(&note.ID, &note.Content, &note.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan note: %w", err)
		}
		result = append(result, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate notes: %w", err)
	}
	return result, nil
}

func (s *Store) DeleteNote(id int) error {
	result, err := s.db.Exec(context.Background(), `DELETE FROM notes WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete note: %w", err)
	}
	if result.RowsAffected() == 0 {
		return notes.ErrNotFound
	}
	return nil
}

func (s *Store) AddTask(task tasks.Task) (tasks.Task, error) {
	task.Title = strings.TrimSpace(task.Title)
	if task.Title == "" {
		return tasks.Task{}, fmt.Errorf("task title cannot be empty")
	}
	var created tasks.Task
	var dueAt sql.NullTime
	err := s.db.QueryRow(
		context.Background(),
		`INSERT INTO tasks (title, description, due_at)
		 VALUES ($1, $2, $3)
		 RETURNING id, title, description, due_at, status, created_at`,
		task.Title, task.Description, task.DueAt,
	).Scan(&created.ID, &created.Title, &created.Description, &dueAt, &created.Status, &created.CreatedAt)
	if err != nil {
		return tasks.Task{}, fmt.Errorf("failed to insert task: %w", err)
	}
	if dueAt.Valid {
		created.DueAt = &dueAt.Time
	}
	return created, nil
}

func (s *Store) ListTasks(status string) ([]tasks.Task, error) {
	query := `SELECT id, title, description, due_at, status, created_at FROM tasks`
	args := []any{}
	if status != "" {
		query += ` WHERE status = $1`
		args = append(args, status)
	}
	query += ` ORDER BY due_at ASC NULLS LAST, created_at ASC`
	rows, err := s.db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (s *Store) GetPendingTasks() ([]tasks.Task, error) {
	rows, err := s.db.Query(
		context.Background(),
		`SELECT id, title, description, due_at, status, created_at
		 FROM tasks
		 WHERE status = $1
		 ORDER BY due_at ASC NULLS LAST, created_at ASC`,
		"pending",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending tasks: %w", err)
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows pgx.Rows) ([]tasks.Task, error) {
	result := make([]tasks.Task, 0)
	for rows.Next() {
		var task tasks.Task
		var dueAt sql.NullTime
		if err := rows.Scan(&task.ID, &task.Title, &task.Description, &dueAt, &task.Status, &task.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan task: %w", err)
		}
		if dueAt.Valid {
			task.DueAt = &dueAt.Time
		}
		result = append(result, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate tasks: %w", err)
	}
	return result, nil
}

func (s *Store) AddLocationMemory(memory location.Memory) (location.Memory, error) {
	memory.StoreType = strings.TrimSpace(memory.StoreType)
	memory.StoreName = strings.TrimSpace(memory.StoreName)
	memory.Message = strings.TrimSpace(memory.Message)
	if memory.StoreType == "" || memory.StoreName == "" || memory.Message == "" {
		return location.Memory{}, errors.New("store type, store name, and message cannot be empty")
	}
	if memory.Timestamp.IsZero() {
		memory.Timestamp = time.Now().UTC()
	}
	err := s.db.QueryRow(context.Background(),
		`INSERT INTO location_based_info (store_type, store_name, latitude, longitude, timestamp, ms)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, timestamp`,
		memory.StoreType, memory.StoreName, memory.Latitude, memory.Longitude, memory.Timestamp, memory.Message,
	).Scan(&memory.ID, &memory.Timestamp)
	if err != nil {
		return location.Memory{}, fmt.Errorf("failed to insert location memory: %w", err)
	}
	return memory, nil
}

func (s *Store) GetLocationMemories(storeType, storeName string) ([]location.Memory, error) {
	if strings.TrimSpace(storeName) == "" {
		return nil, errors.New("store name cannot be empty")
	}
	query := `SELECT id, store_type, store_name, latitude, longitude, timestamp, ms
		FROM location_based_info WHERE store_type=$1 AND store_name=$2 ORDER BY timestamp DESC, id DESC`
	args := []any{storeType, storeName}
	if storeType == "" {
		query = `SELECT id, store_type, store_name, latitude, longitude, timestamp, ms
			FROM location_based_info WHERE store_name=$1 ORDER BY timestamp DESC, id DESC`
		args = []any{storeName}
	}
	rows, err := s.db.Query(context.Background(), query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query location memories: %w", err)
	}
	defer rows.Close()
	memories := make([]location.Memory, 0)
	for rows.Next() {
		var memory location.Memory
		if err := rows.Scan(&memory.ID, &memory.StoreType, &memory.StoreName, &memory.Latitude,
			&memory.Longitude, &memory.Timestamp, &memory.Message); err != nil {
			return nil, fmt.Errorf("failed to scan location memory: %w", err)
		}
		memories = append(memories, memory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate location memories: %w", err)
	}
	return memories, nil
}

func (s *Store) ListShortcuts() ([]location.Shortcut, error) {
	rows, err := s.db.Query(context.Background(),
		`SELECT name, description, updated_at FROM shortcuts ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("failed to query shortcuts: %w", err)
	}
	defer rows.Close()
	shortcuts := make([]location.Shortcut, 0)
	for rows.Next() {
		var shortcut location.Shortcut
		if err := rows.Scan(&shortcut.Name, &shortcut.Description, &shortcut.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan shortcut: %w", err)
		}
		shortcuts = append(shortcuts, shortcut)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate shortcuts: %w", err)
	}
	return shortcuts, nil
}

func (s *Store) SaveShortcut(shortcut location.Shortcut) (location.Shortcut, error) {
	err := s.db.QueryRow(context.Background(),
		`INSERT INTO shortcuts (name, description)
		 VALUES ($1, $2)
		 ON CONFLICT (name) DO UPDATE SET description = EXCLUDED.description, updated_at = NOW()
		 RETURNING name, description, updated_at`,
		shortcut.Name, shortcut.Description,
	).Scan(&shortcut.Name, &shortcut.Description, &shortcut.UpdatedAt)
	if err != nil {
		return location.Shortcut{}, fmt.Errorf("failed to save shortcut: %w", err)
	}
	return shortcut, nil
}

func (s *Store) DeleteShortcut(name string) error {
	result, err := s.db.Exec(context.Background(), `DELETE FROM shortcuts WHERE name=$1`, name)
	if err != nil {
		return fmt.Errorf("failed to delete shortcut: %w", err)
	}
	if result.RowsAffected() == 0 {
		return location.ErrShortcutNotFound
	}
	return nil
}

func (s *Store) GetHomeLocation() (location.HomeLocation, bool, error) {
	var home location.HomeLocation
	err := s.db.QueryRow(context.Background(),
		`SELECT latitude, longitude, radius_meters, updated_at FROM home_location WHERE id=1`,
	).Scan(&home.Latitude, &home.Longitude, &home.RadiusMeters, &home.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return location.HomeLocation{}, false, nil
	}
	if err != nil {
		return location.HomeLocation{}, false, fmt.Errorf("failed to get home location: %w", err)
	}
	return home, true, nil
}

func (s *Store) SaveHomeLocation(home location.HomeLocation) (location.HomeLocation, error) {
	err := s.db.QueryRow(context.Background(),
		`INSERT INTO home_location (id, latitude, longitude, radius_meters)
		 VALUES (1, $1, $2, $3)
		 ON CONFLICT (id) DO UPDATE SET latitude = EXCLUDED.latitude,
		 longitude = EXCLUDED.longitude, radius_meters = EXCLUDED.radius_meters, updated_at = NOW()
		 RETURNING latitude, longitude, radius_meters, updated_at`,
		home.Latitude, home.Longitude, home.RadiusMeters,
	).Scan(&home.Latitude, &home.Longitude, &home.RadiusMeters, &home.UpdatedAt)
	if err != nil {
		return location.HomeLocation{}, fmt.Errorf("failed to save home location: %w", err)
	}
	return home, nil
}

func (s *Store) SaveLocationHistory(event location.LocationHistory) (location.LocationHistory, error) {
	err := s.db.QueryRow(context.Background(),
		`INSERT INTO location_update_history (latitude, longitude, response)
		 VALUES ($1, $2, $3)
		 RETURNING id, created_at`,
		event.Latitude, event.Longitude, []byte(event.Response),
	).Scan(&event.ID, &event.CreatedAt)
	if err != nil {
		return location.LocationHistory{}, fmt.Errorf("failed to save location history: %w", err)
	}
	return event, nil
}

func (s *Store) ListLocationHistory(limit int) ([]location.LocationHistory, error) {
	rows, err := s.db.Query(context.Background(),
		`SELECT id, created_at, latitude, longitude, response
		 FROM location_update_history ORDER BY created_at DESC, id DESC LIMIT $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query location history: %w", err)
	}
	defer rows.Close()
	history := make([]location.LocationHistory, 0)
	for rows.Next() {
		var event location.LocationHistory
		if err := rows.Scan(&event.ID, &event.CreatedAt, &event.Latitude, &event.Longitude, &event.Response); err != nil {
			return nil, fmt.Errorf("failed to scan location history: %w", err)
		}
		history = append(history, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate location history: %w", err)
	}
	return history, nil
}

func (s *Store) DeleteLocationHistory(id int) error {
	result, err := s.db.Exec(context.Background(), `DELETE FROM location_update_history WHERE id=$1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete location history: %w", err)
	}
	if result.RowsAffected() == 0 {
		return location.ErrLocationHistoryNotFound
	}
	return nil
}
