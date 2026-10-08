package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

func Open(ctx context.Context, path, migrationsDir string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(err error) (*sql.DB, error) { db.Close(); return nil, err }
	for _, q := range []string{"PRAGMA busy_timeout=5000", "PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON"} {
		if _, err = db.ExecContext(ctx, q); err != nil {
			return fail(err)
		}
	}
	var version int
	if err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(fmt.Errorf("unsupported database version %d", version))
	}
	if version == 0 {
		var exists int
		if err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='jobs'").Scan(&exists); err != nil {
			return fail(err)
		}
		if exists != 0 {

			rows, e := db.QueryContext(ctx, "PRAGMA table_info(jobs)")
			if e != nil {
				return fail(e)
			}
			columns := map[string]bool{}
			for rows.Next() {
				var cid, notnull, pk int
				var name, typ string
				var def any
				if e = rows.Scan(&cid, &name, &typ, &notnull, &def, &pk); e != nil {
					rows.Close()
					return fail(e)
				}
				columns[name] = true
			}
			e = rows.Err()
			rows.Close()
			if e != nil {
				return fail(e)
			}
			for _, name := range []string{"id", "original_filename", "source_path", "size_bytes", "duration_seconds", "status", "progress", "error_code", "error_message", "created_at", "updated_at", "finished_at", "removed_at"} {
				if !columns[name] {
					return fail(fmt.Errorf("incompatible jobs schema: missing %s", name))
				}
			}
			if _, err = db.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
				return fail(err)
			}
		} else {
			schema, e := os.ReadFile(filepath.Join(migrationsDir, "001_init.up.sql"))
			if e != nil {
				return fail(e)
			}
			tx, e := db.BeginTx(ctx, nil)
			if e != nil {
				return fail(e)
			}
			if _, e = tx.ExecContext(ctx, string(schema)); e == nil {
				_, e = tx.ExecContext(ctx, "PRAGMA user_version=1")
			}
			if e != nil {
				tx.Rollback()
				return fail(e)
			}
			if e = tx.Commit(); e != nil {
				return fail(e)
			}
		}
	}
	return db, nil
}
