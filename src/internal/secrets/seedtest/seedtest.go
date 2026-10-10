// Package seedtest crea con SQL directo los proyectos, stages y apps que usan
// los tests de secrets. Esas tablas son de catalog y secrets no lo importa,
// así que los tests insertan las filas a mano. Solo lo importan los tests.
package seedtest

import (
	"context"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// T0 es la hora de creación de lo que siembran los tests.
var T0 = time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

func insert(t testing.TB, db *sqlite.DB, query string, args ...any) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := db.Conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("seedtest: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("seedtest: %v", err)
	}
	return id
}

// Project inserta un proyecto y devuelve su id.
func Project(t testing.TB, db *sqlite.DB, slug string) int64 {
	t.Helper()
	return insert(t, db, `INSERT INTO projects (slug, name, created_at) VALUES (?, ?, ?)`,
		slug, slug, sqlite.Millis(T0))
}

// Stage inserta un stage del proyecto y devuelve su id.
func Stage(t testing.TB, db *sqlite.DB, projectID int64, slug string) int64 {
	t.Helper()
	return insert(t, db, `INSERT INTO stages (project_id, slug, created_at) VALUES (?, ?, ?)`,
		projectID, slug, sqlite.Millis(T0))
}

// App inserta una app de formulario del stage, con config_changed_at = T0, y devuelve su id.
func App(t testing.TB, db *sqlite.DB, stageID int64, slug string) int64 {
	t.Helper()
	return insert(t, db,
		`INSERT INTO apps (stage_id, slug, kind, spec, config_changed_at, created_at) VALUES (?, ?, 'service', '{}', ?, ?)`,
		stageID, slug, sqlite.Millis(T0), sqlite.Millis(T0))
}

// ProjectID devuelve el id del proyecto con ese slug.
func ProjectID(t testing.TB, db *sqlite.DB, slug string) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT id FROM projects WHERE slug = ?`, slug).Scan(&id); err != nil {
		t.Fatalf("seedtest: proyecto %s: %v", slug, err)
	}
	return id
}

// Auditfy crea el proyecto auditfy con el stage staging y su app api.
func Auditfy(t testing.TB, db *sqlite.DB) (stageID, appID int64) {
	t.Helper()
	stageID = Stage(t, db, Project(t, db, "auditfy"), "staging")
	return stageID, App(t, db, stageID, "api")
}

// ConfigChangedAt lee el config_changed_at de una app.
func ConfigChangedAt(t testing.TB, db *sqlite.DB, appID int64) time.Time {
	t.Helper()
	ctx := context.Background()
	var ms int64
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT config_changed_at FROM apps WHERE id = ?`, appID).Scan(&ms); err != nil {
		t.Fatalf("seedtest: config_changed_at de la app %d: %v", appID, err)
	}
	return sqlite.Time(ms)
}

// Count devuelve el resultado de un SELECT COUNT(*).
func Count(t testing.TB, db *sqlite.DB, query string, args ...any) int {
	t.Helper()
	ctx := context.Background()
	var n int
	if err := db.Conn(ctx).QueryRowContext(ctx, query, args...).Scan(&n); err != nil {
		t.Fatalf("seedtest: %v", err)
	}
	return n
}
