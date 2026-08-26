package db

import (
	"database/sql"
	_ "embed"
	"strings"

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
	if _, err := conn.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		conn.Close()
		return nil, err
	}
	if _, err := conn.Exec(schemaSQL); err != nil {
		conn.Close()
		return nil, err
	}
	if err := migrate(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	if _, err := conn.Exec(`ALTER TABLE daily_checkin ADD COLUMN water_intake REAL`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	if _, err := conn.Exec(`ALTER TABLE blocks ADD COLUMN block_type TEXT NOT NULL DEFAULT 'deep'`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	if _, err := conn.Exec(`ALTER TABLE weekly_goals ADD COLUMN sort_order INTEGER`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	if _, err := conn.Exec(`UPDATE weekly_goals SET sort_order = id WHERE sort_order IS NULL`); err != nil {
		return err
	}
	return nil
}
