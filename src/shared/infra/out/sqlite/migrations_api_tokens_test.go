package sqlite

import (
	"context"
	"regexp"
	"strings"
	"testing"
)

// beforeTokenScopes es la última versión antes de la 0007, que reconstruye
// api_tokens. Los tests migran "hasta antes de la 0007" y no "hasta la 0006",
// así siguen valiendo con la 0006 de Origin CA en el mismo directorio.
const beforeTokenScopes = 6

const insertToken = `INSERT INTO api_tokens (id, name, token_hash, project_id, stage_id, created_by, created_at, last_used_at, expires_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

// seedTokenScope deja el proyecto 1 (auditfy) con el stage 1 (staging) y el
// usuario 1 (felipe).
func seedTokenScope(t *testing.T, d *DB) {
	t.Helper()
	for _, q := range []string{
		`INSERT INTO projects (id, slug, name, created_at) VALUES (1, 'auditfy', 'Auditfy', 0)`,
		`INSERT INTO stages (id, project_id, slug, created_at) VALUES (1, 1, 'staging', 0)`,
		`INSERT INTO users (id, username, password_hash, created_at) VALUES (1, 'felipe', 'x', 0)`,
	} {
		mustExec(t, d, q)
	}
}

// newTokenID crea un token de proyecto sin id y devuelve el que le dio la base.
func newTokenID(t *testing.T, d *DB, hash string) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := d.Conn(ctx).ExecContext(ctx,
		`INSERT INTO api_tokens (name, token_hash, project_id, created_at) VALUES ('nuevo', ?, 1, 0)`, hash)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// La 0007 conserva la secuencia de AUTOINCREMENT: el id de un token borrado
// no vuelve (va en la auditoría y en DELETE /tokens/{id}).
func TestAPITokensMigrationKeepsSequence(t *testing.T) {
	t.Run("borrado el más nuevo", func(t *testing.T) {
		d := openAt(t, beforeTokenScopes)
		seedTokenScope(t, d)
		for i := 1; i <= 3; i++ {
			mustExec(t, d, insertToken, i, "t", "h"+string(rune('0'+i)), 1, nil, 1, 0, nil, nil)
		}
		mustExec(t, d, `DELETE FROM api_tokens WHERE id = 3`)
		if err := d.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		if id := newTokenID(t, d, "nuevo"); id != 4 {
			t.Fatalf("el próximo token es el %d, esperaba 4", id)
		}
	})
	t.Run("sin tokens y la secuencia en 7", func(t *testing.T) {
		d := openAt(t, beforeTokenScopes)
		seedTokenScope(t, d)
		mustExec(t, d, insertToken, 7, "t", "h7", 1, nil, nil, 0, nil, nil)
		mustExec(t, d, `DELETE FROM api_tokens`)
		if err := d.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		if id := newTokenID(t, d, "nuevo"); id != 8 {
			t.Fatalf("el próximo token es el %d, esperaba 8", id)
		}
	})
	t.Run("nunca hubo tokens", func(t *testing.T) {
		d := openAt(t, beforeTokenScopes)
		seedTokenScope(t, d)
		if err := d.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		if id := newTokenID(t, d, "nuevo"); id != 1 {
			t.Fatalf("el primer token es el %d, esperaba 1", id)
		}
	})
	t.Run("un solo registro en sqlite_sequence", func(t *testing.T) {
		d := openAt(t, beforeTokenScopes)
		seedTokenScope(t, d)
		mustExec(t, d, insertToken, 5, "t", "h5", 1, nil, nil, 0, nil, nil)
		if err := d.Migrate(context.Background()); err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		var n int
		var seq int64
		if err := d.Conn(ctx).QueryRowContext(ctx,
			`SELECT COUNT(*), MAX(seq) FROM sqlite_sequence WHERE name LIKE 'api_tokens%'`).Scan(&n, &seq); err != nil {
			t.Fatal(err)
		}
		if n != 1 || seq != 5 {
			t.Fatalf("sqlite_sequence de api_tokens: %d filas, seq %d", n, seq)
		}
	})
}

// Las filas que copia la 0007 quedan iguales, de solo escritura y de un
// proyecto (read_only = 0, all_projects = 0).
func TestAPITokensMigrationCopiesRows(t *testing.T) {
	d := openAt(t, beforeTokenScopes)
	seedTokenScope(t, d)
	mustExec(t, d, insertToken, 1, "ci", "h1", 1, nil, 1, 1000, 2000, 3000)
	mustExec(t, d, insertToken, 2, "ci-staging", "h2", 1, 1, nil, 1001, nil, nil)
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}

	type row struct {
		id                        int64
		name, hash                string
		project, stage, createdBy any
		created                   int64
		used, expires             any
		readOnly, allProjects     int
	}
	want := []row{
		{1, "ci", "h1", int64(1), nil, int64(1), 1000, int64(2000), int64(3000), 0, 0},
		{2, "ci-staging", "h2", int64(1), int64(1), nil, 1001, nil, nil, 0, 0},
	}
	ctx := context.Background()
	rows, err := d.Conn(ctx).QueryContext(ctx, `SELECT id, name, token_hash, project_id, stage_id, created_by,
		created_at, last_used_at, expires_at, read_only, all_projects FROM api_tokens ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.name, &r.hash, &r.project, &r.stage, &r.createdBy, &r.created, &r.used, &r.expires,
			&r.readOnly, &r.allProjects); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("%d filas, esperaba %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("fila %d: %+v, esperaba %+v", i+1, got[i], want[i])
		}
	}
}

// Los CHECK de la 0007: todos los proyectos si y solo si no hay proyecto, y
// entonces sin stage y de solo lectura (contrato §8.1).
func TestAPITokensScopeChecks(t *testing.T) {
	d := newTestDB(t)
	seedTokenScope(t, d)
	const ins = `INSERT INTO api_tokens (name, token_hash, project_id, stage_id, read_only, all_projects, created_at)
		VALUES ('t', ?, ?, ?, ?, ?, 0)`

	mustExec(t, d, ins, "ok-proyecto", 1, nil, 0, 0)
	mustExec(t, d, ins, "ok-stage-ro", 1, 1, 1, 0)
	mustExec(t, d, ins, "ok-todos", nil, nil, 1, 1)

	mustFail(t, d, "todos los proyectos sin solo lectura", ins, "h1", nil, nil, 0, 1)
	mustFail(t, d, "todos los proyectos con proyecto", ins, "h2", 1, nil, 1, 1)
	mustFail(t, d, "sin proyecto y sin todos los proyectos", ins, "h3", nil, nil, 1, 0)
	mustFail(t, d, "todos los proyectos con stage", ins, "h4", nil, 1, 1, 1)
	mustFail(t, d, "read_only fuera de 0 y 1", ins, "h5", 1, nil, 2, 0)
	mustFail(t, d, "all_projects fuera de 0 y 1", ins, "h6", nil, nil, 1, 2)
	mustFail(t, d, "read_only NULL", ins, "h7", 1, nil, nil, 0)

	// Por defecto, un token es de escritura y de un proyecto.
	mustExec(t, d, `INSERT INTO api_tokens (name, token_hash, project_id, created_at) VALUES ('t', 'def', 1, 0)`)
	ctx := context.Background()
	var ro, all int
	if err := d.Conn(ctx).QueryRowContext(ctx,
		`SELECT read_only, all_projects FROM api_tokens WHERE token_hash = 'def'`).Scan(&ro, &all); err != nil {
		t.Fatal(err)
	}
	if ro != 0 || all != 0 {
		t.Fatalf("por defecto: read_only=%d all_projects=%d", ro, all)
	}
}

// Las claves foráneas siguen: un token se borra con su stage o su proyecto,
// y la base migrada no tiene referencias rotas.
func TestAPITokensMigrationForeignKeys(t *testing.T) {
	d := openAt(t, beforeTokenScopes)
	seedTokenScope(t, d)
	mustExec(t, d, insertToken, 1, "ci", "h1", 1, nil, 1, 0, nil, nil)
	mustExec(t, d, insertToken, 2, "ci-staging", "h2", 1, 1, 1, 0, nil, nil)
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rows, err := d.Conn(ctx).QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	if rows.Next() {
		t.Error("PRAGMA foreign_key_check no está vacío")
	}
	rows.Close()

	mustExec(t, d, `INSERT INTO api_tokens (name, token_hash, read_only, all_projects, created_at) VALUES ('todos', 'h3', 1, 1, 0)`)
	mustExec(t, d, `DELETE FROM users WHERE id = 1`)
	var createdBy any
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT created_by FROM api_tokens WHERE id = 1`).Scan(&createdBy); err != nil || createdBy != nil {
		t.Fatalf("created_by después de borrar al usuario: %v %v", createdBy, err)
	}
	mustExec(t, d, `DELETE FROM stages WHERE id = 1`)
	if n := count(t, d, "api_tokens"); n != 2 {
		t.Fatalf("después de borrar el stage quedan %d tokens, esperaba 2", n)
	}
	mustExec(t, d, `DELETE FROM projects WHERE id = 1`)
	if n := count(t, d, "api_tokens"); n != 1 {
		t.Fatalf("después de borrar el proyecto quedan %d tokens, esperaba 1 (el de todos los proyectos)", n)
	}
}

// El esquema que queda es el de la 0007, con el nombre final, y sigue con
// AUTOINCREMENT y el UNIQUE de token_hash.
func TestAPITokensSchema(t *testing.T) {
	d := newTestDB(t)
	ctx := context.Background()
	var got string
	if err := d.Conn(ctx).QueryRowContext(ctx,
		`SELECT sql FROM sqlite_schema WHERE type = 'table' AND name = 'api_tokens'`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	const want = `CREATE TABLE api_tokens (
  id           INTEGER PRIMARY KEY AUTOINCREMENT,
  name         TEXT    NOT NULL,
  token_hash   TEXT    NOT NULL UNIQUE,
  project_id   INTEGER REFERENCES projects(id) ON DELETE CASCADE,
  stage_id     INTEGER REFERENCES stages(id) ON DELETE CASCADE,
  read_only    INTEGER NOT NULL DEFAULT 0 CHECK (read_only IN (0, 1)),
  all_projects INTEGER NOT NULL DEFAULT 0 CHECK (all_projects IN (0, 1)),
  created_by   INTEGER REFERENCES users(id) ON DELETE SET NULL,
  created_at   INTEGER NOT NULL,
  last_used_at INTEGER,
  expires_at   INTEGER,
  CHECK ((all_projects = 1) = (project_id IS NULL)),
  CHECK (all_projects = 0 OR (read_only = 1 AND stage_id IS NULL))
)`
	if normalizeSQL(got) != normalizeSQL(want) {
		t.Fatalf("esquema de api_tokens:\n%s\nesperaba:\n%s", normalizeSQL(got), normalizeSQL(want))
	}
	if !usesAutoincrement(t, d, "api_tokens") {
		t.Error("api_tokens perdió AUTOINCREMENT")
	}
	var others int
	if err := d.Conn(ctx).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_schema WHERE tbl_name = 'api_tokens' AND type IN ('trigger', 'view')`).Scan(&others); err != nil || others != 0 {
		t.Fatalf("triggers o vistas sobre api_tokens: %d %v", others, err)
	}
	var leftovers int
	if err := d.Conn(ctx).QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_schema WHERE name LIKE 'api_tokens_new%'`).Scan(&leftovers); err != nil || leftovers != 0 {
		t.Fatalf("quedó api_tokens_new: %d %v", leftovers, err)
	}
}

var (
	sqlComment = regexp.MustCompile(`--[^\n]*`)
	sqlSpace   = regexp.MustCompile(`\s+`)
)

// normalizeSQL quita los comentarios, las comillas del nombre (RENAME las
// pone) y junta los espacios.
func normalizeSQL(s string) string {
	s = sqlComment.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, `"`, "")
	s = sqlSpace.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "( ", "(")
	s = strings.ReplaceAll(s, " )", ")")
	return strings.TrimSpace(s)
}
