package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

func Open(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is empty")
	}
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create data directory: %w", err)
		}
	}
	snapshotTemp := path == ":memory:"
	var snapshotDir string
	var err error
	if snapshotTemp {
		snapshotDir, err = os.MkdirTemp("", "facesign-recognition-snapshots-")
	} else {
		snapshotDir = filepath.Join(filepath.Dir(path), "recognition_snapshots")
		err = os.MkdirAll(snapshotDir, 0o700)
	}
	if err != nil {
		return nil, fmt.Errorf("prepare recognition snapshot directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		if snapshotTemp {
			_ = os.RemoveAll(snapshotDir)
		}
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, snapshotDir: snapshotDir, snapshotTemp: snapshotTemp}
	if err := s.init(context.Background()); err != nil {
		_ = db.Close()
		if snapshotTemp {
			_ = os.RemoveAll(snapshotDir)
		}
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	closeErr := s.db.Close()
	if s.snapshotTemp {
		return errors.Join(closeErr, os.RemoveAll(s.snapshotDir))
	}
	return closeErr
}
