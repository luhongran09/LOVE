package main

import (
	"database/sql"
	_ "github.com/mattn/go-sqlite3"
	"os"
	"path/filepath"
	"time"
)

func openApp(dir string) (*App, error) {
	if err := os.MkdirAll(filepath.Join(dir, "photos"), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", filepath.Join(dir, "album.db")+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS spaces(id TEXT PRIMARY KEY, invite_hash TEXT, invite_expires INTEGER);
 CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY, name TEXT NOT NULL UNIQUE, password TEXT NOT NULL, space_id TEXT NOT NULL REFERENCES spaces(id));
 CREATE TABLE IF NOT EXISTS sessions(hash TEXT PRIMARY KEY,user_id TEXT NOT NULL REFERENCES users(id),expires INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS photos(id TEXT PRIMARY KEY,space_id TEXT NOT NULL REFERENCES spaces(id),owner_id TEXT NOT NULL REFERENCES users(id),filename TEXT NOT NULL,title TEXT NOT NULL,note TEXT NOT NULL,place TEXT NOT NULL,taken_at TEXT NOT NULL,latitude REAL,longitude REAL,created_at TEXT NOT NULL);
 CREATE INDEX IF NOT EXISTS photos_space_date ON photos(space_id,taken_at);`)
	if err != nil {
		db.Close()
		return nil, err
	}
	var version, count int
	if err = db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		db.Close()
		return nil, err
	}
	if version < 2 {
		if err = db.QueryRow(`SELECT count(*) FROM photos`).Scan(&count); err != nil {
			db.Close()
			return nil, err
		}
		if count > 0 {
			snapshot := filepath.Join(dir, "before-memory-migration-"+time.Now().UTC().Format("20060102T150405.000000000")+".sqlite3")
			if _, err = db.Exec(`VACUUM INTO ?`, snapshot); err != nil {
				db.Close()
				return nil, err
			}
		}
	}
	if err = migrateMemories(db); err != nil {
		db.Close()
		return nil, err
	}
	return &App{db: db, dir: dir, secure: os.Getenv("COOKIE_SECURE") == "true", origin: os.Getenv("APP_ORIGIN"), attempts: map[string][]time.Time{}}, nil
}
