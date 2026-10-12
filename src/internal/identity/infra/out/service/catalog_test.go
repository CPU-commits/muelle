package service

import (
	"context"
	"errors"
	"testing"

	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	sqlitedb "github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func exec(t *testing.T, db *sqlitedb.DB, query string, args ...any) int64 {
	t.Helper()
	res, err := db.Conn(context.Background()).ExecContext(context.Background(), query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

func TestScopeResolver(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	auditfy := exec(t, db, `INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`)
	otro := exec(t, db, `INSERT INTO projects (slug, name, created_at) VALUES ('otro', 'Otro', 0)`)
	staging := exec(t, db, `INSERT INTO stages (project_id, slug, created_at) VALUES (?, 'staging', 0)`, auditfy)
	exec(t, db, `INSERT INTO stages (project_id, slug, created_at) VALUES (?, 'production', 0)`, otro)
	r := NewScopeResolver(db)

	p, s, err := r.Resolve(ctx, "auditfy", "")
	if err != nil || p != auditfy || s != nil {
		t.Fatalf("proyecto: p=%d s=%v err=%v", p, s, err)
	}
	p, s, err = r.Resolve(ctx, "auditfy", "staging")
	if err != nil || p != auditfy || s == nil || *s != staging {
		t.Fatalf("stage: p=%d s=%v err=%v", p, s, err)
	}
	cases := []struct{ project, stage string }{
		{"nope", ""},
		{"", ""},
		{"auditfy", "nope"},
		{"auditfy", "production"}, // existe, pero en otro proyecto
	}
	for _, c := range cases {
		if _, _, err := r.Resolve(ctx, c.project, c.stage); !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%s/%s: esperaba ErrNotFound, got %v", c.project, c.stage, err)
		}
	}
}

func TestSettingsReader(t *testing.T) {
	db := sqlitetest.New(t)
	ctx := context.Background()
	r := NewSettingsReader(db)

	if on, err := r.Require2FA(ctx); err != nil || on {
		t.Fatalf("sin configurar: on=%v err=%v", on, err)
	}
	exec(t, db, `INSERT INTO settings (key, value) VALUES ('require_2fa', 'true')`)
	if on, err := r.Require2FA(ctx); err != nil || !on {
		t.Fatalf("true: on=%v err=%v", on, err)
	}
	exec(t, db, `UPDATE settings SET value = 'false' WHERE key = 'require_2fa'`)
	if on, err := r.Require2FA(ctx); err != nil || on {
		t.Fatalf("false: on=%v err=%v", on, err)
	}
	// admin puede guardarlo como BLOB: se lee igual.
	exec(t, db, `UPDATE settings SET value = ? WHERE key = 'require_2fa'`, []byte("true"))
	if on, err := r.Require2FA(ctx); err != nil || !on {
		t.Fatalf("BLOB true: on=%v err=%v", on, err)
	}
}
