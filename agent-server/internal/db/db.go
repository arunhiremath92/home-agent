package db

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Db struct {
	conn *pgxpool.Pool
}

func NewDb() *Db {
	return &Db{}
}

func connectionURLFromEnvironment() (string, error) {
	user := strings.TrimSpace(os.Getenv("POSTGRES_USER"))
	password := os.Getenv("POSTGRES_PASSWORD")
	host := strings.TrimSpace(os.Getenv("POSTGRES_HOST"))
	port := strings.TrimSpace(os.Getenv("POSTGRES_PORT"))
	database := strings.TrimSpace(os.Getenv("POSTGRES_DB"))
	if user == "" || password == "" || database == "" {
		return "", fmt.Errorf("POSTGRES_USER, POSTGRES_PASSWORD, and POSTGRES_DB must be set")
	}
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "5432"
	}
	connectionURL := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, password),
		Host:   net.JoinHostPort(host, port),
		Path:   database,
	}
	return connectionURL.String(), nil
}

func (db *Db) Connect(ctx context.Context) error {
	connectionURL, err := connectionURLFromEnvironment()
	if err != nil {
		return err
	}
	conn, err := pgxpool.New(ctx, connectionURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	if err := conn.Ping(ctx); err != nil {
		conn.Close()
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	db.conn = conn
	return nil
}

func (db *Db) Close(ctx context.Context) error {
	if db.conn == nil {
		return fmt.Errorf("database connection is not established")
	}
	db.conn.Close()
	return nil
}

func (db *Db) Ping(ctx context.Context) error {
	if db.conn == nil {
		return fmt.Errorf("database connection is not established")
	}
	return db.conn.Ping(ctx)
}

func (db *Db) Exec(ctx context.Context, statement string, arguments ...interface{}) (pgconn.CommandTag, error) {
	if db.conn == nil {
		return pgconn.CommandTag{}, fmt.Errorf("database connection is not established")
	}
	return db.conn.Exec(ctx, statement, arguments...)
}

func (db *Db) Query(ctx context.Context, statement string, args ...interface{}) (pgx.Rows, error) {
	if db.conn == nil {
		return nil, fmt.Errorf("database connection is not established")
	}
	return db.conn.Query(ctx, statement, args...)
}

func (db *Db) QueryRow(ctx context.Context, statement string, args ...interface{}) pgx.Row {
	if db.conn == nil {
		panic("database connection is not established")
	}
	return db.conn.QueryRow(ctx, statement, args...)
}

func (db *Db) Migrate(context.Context) error {
	return nil
}
