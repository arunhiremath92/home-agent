package store

import (
	"context"
	"fmt"
)

const schema = `
CREATE TABLE IF NOT EXISTS notes (
	id SERIAL PRIMARY KEY,
	content TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS tasks (
	id SERIAL PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	due_at TIMESTAMPTZ,
	status TEXT NOT NULL DEFAULT 'pending',
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS location_based_info (
	id SERIAL PRIMARY KEY,
	store_type TEXT NOT NULL DEFAULT '',
	store_name TEXT NOT NULL DEFAULT '',
	latitude DOUBLE PRECISION NOT NULL,
	longitude DOUBLE PRECISION NOT NULL,
	timestamp TIMESTAMPTZ NOT NULL,
	ms TEXT NOT NULL
);

ALTER TABLE location_based_info ADD COLUMN IF NOT EXISTS store_type TEXT NOT NULL DEFAULT '';
ALTER TABLE location_based_info ADD COLUMN IF NOT EXISTS store_name TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS shortcuts (
	name TEXT PRIMARY KEY,
	description TEXT NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS home_location (
	id SMALLINT PRIMARY KEY CHECK (id = 1),
	latitude DOUBLE PRECISION NOT NULL,
	longitude DOUBLE PRECISION NOT NULL,
	radius_meters INTEGER NOT NULL,
	updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS location_update_history (
	id BIGSERIAL PRIMARY KEY,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
	latitude DOUBLE PRECISION NOT NULL,
	longitude DOUBLE PRECISION NOT NULL,
	response JSONB NOT NULL
);
`

func (s *Store) setup() error {
	if _, err := s.db.Exec(context.Background(), schema); err != nil {
		return fmt.Errorf("failed to create application schema: %w", err)
	}
	return nil
}
