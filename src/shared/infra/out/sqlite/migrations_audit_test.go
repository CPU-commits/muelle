package sqlite

import (
	"context"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// openAt abre una base nueva con las migraciones embebidas hasta last
// incluida, como la dejaría una versión anterior de muelle. Después, Migrate
// aplica solo las que siguen.
func openAt(t *testing.T, last int) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "muelle.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	mustExec(t, d, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`)
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries { // ReadDir las devuelve ordenadas por nombre
		prefix, _, _ := strings.Cut(e.Name(), "_")
		v, err := strconv.Atoi(prefix)
		if err != nil {
			t.Fatal(err)
		}
		if v > last {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		mustExec(t, d, string(body))
		mustExec(t, d, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, 0)`, v)
	}
	return d
}

// 0004 suma audit_events.actor_name y lo llena en los eventos que ya había:
// con el username o el nombre del token si todavía existen, con el username
// que dejó en la auditoría un usuario ya borrado (su auth.login o su
// user.create), con "system" en los del sistema, y si no, "".
func TestAuditActorNameMigration(t *testing.T) {
	d := openAt(t, 3)
	for _, q := range []string{
		`INSERT INTO projects (id, slug, name, created_at) VALUES (1, 'auditfy', 'Auditfy', 0)`,
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (1, 'felipe', 'x', 0)`,
		`INSERT INTO api_tokens (id, name, token_hash, project_id, created_at) VALUES (5, 'ci', 'h5', 1, 0)`,
	} {
		mustExec(t, d, q)
	}
	const ins = `INSERT INTO audit_events (id, actor_type, actor_id, action, target, at) VALUES (?, ?, ?, ?, ?, 0)`
	events := []struct {
		id     int
		typ    string
		actor  any
		action string
		target string
		want   string
	}{
		{1, "system", nil, "settings.init", "settings", "system"},
		{2, "user", 1, "auth.login", "user:felipe", "felipe"},
		{3, "user", 1, "project.create", "project:auditfy", "felipe"},
		// ana (2) ya no está: su nombre sale de su user.create y su auth.login.
		{4, "user", 2, "user.create", "user:ana", "ana"},
		{5, "user", 2, "auth.login", "user:ana", "ana"},
		{6, "user", 2, "env.patch", "app:auditfy/staging/api", "ana"},
		// 3 ya no está y no dejó su nombre: el user.delete nombra a otro.
		{7, "user", 3, "user.delete", "user:zoe", ""},
		{8, "user", 3, "auth.logout", "user:zoe", ""},
		{9, "token", 5, "env.replace", "stage:auditfy/staging", "ci"},
		// El token 6 ya no está: su nombre no quedó en ninguna parte.
		{10, "token", 6, "env.replace", "stage:auditfy/staging", ""},
		// Un evento del sistema con un username en el target sigue siendo del sistema.
		{11, "system", nil, "auth.login_failed", "user:felipe", "system"},
	}
	for _, e := range events {
		mustExec(t, d, ins, e.id, e.typ, e.actor, e.action, e.target)
	}

	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	for _, e := range events {
		var got string
		if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT actor_name FROM audit_events WHERE id = ?`, e.id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != e.want {
			t.Errorf("evento %d (%s %v %s): actor_name = %q, esperaba %q", e.id, e.typ, e.actor, e.action, got, e.want)
		}
	}

	// Un INSERT que no lo nombra recibe "", nunca NULL.
	mustExec(t, d, `INSERT INTO audit_events (actor_type, action, target, at) VALUES ('system', 'x', 'y', 0)`)
	mustFail(t, d, "actor_name NULL", `INSERT INTO audit_events (actor_type, actor_name, action, target, at) VALUES ('system', NULL, 'x', 'y', 0)`)
	var empty int
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events WHERE action = 'x' AND actor_name = ''`).Scan(&empty); err != nil || empty != 1 {
		t.Fatalf("por defecto: %d %v", empty, err)
	}
}

// En una base nueva, 0004 corre sobre una tabla vacía y deja la columna.
func TestAuditActorNameColumnOnNewDB(t *testing.T) {
	d := newTestDB(t)
	mustExec(t, d, `INSERT INTO audit_events (actor_type, actor_id, actor_name, action, target, at) VALUES ('user', 1, 'felipe', 'x', 'y', 0)`)
	if n := count(t, d, "audit_events"); n != 1 {
		t.Fatalf("hay %d eventos", n)
	}
}
