package db

import (
	"database/sql"
	_ "embed"

	_ "github.com/glebarez/go-sqlite"
)

//go:embed schema.sql
var schemaSQL string

// Open connects to the sqlite file at path and ensures tables exist.
func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}
