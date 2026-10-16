package sqlite

import (
	"context"
	"strings"
	"testing"
)

// Esquemas de las migraciones reservadas para las fases 2 (0002: deploys y
// registries) y 3 (0003: domains), §5.1 del spec. Los módulos deploy e ingress
// los usan desde sus repositorios; estos tests fijan lo que ambos lados acuerdan.

// seedApp crea un proyecto, un stage y una app, y devuelve el id de la app.
func seedApp(t *testing.T, d *DB) int64 {
	t.Helper()
	ctx := context.Background()
	for _, q := range []string{
		`INSERT INTO projects (slug, name, created_at) VALUES ('auditfy', 'Auditfy', 0)`,
		`INSERT INTO stages (project_id, slug, created_at) SELECT id, 'staging', 0 FROM projects`,
		`INSERT INTO apps (stage_id, slug, kind, spec, config_changed_at, created_at) SELECT id, 'api', 'service', '{}', 0, 0 FROM stages`,
	} {
		if _, err := d.Conn(ctx).ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	var id int64
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT id FROM apps`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func mustExec(t *testing.T, d *DB, q string, args ...any) {
	t.Helper()
	ctx := context.Background()
	if _, err := d.Conn(ctx).ExecContext(ctx, q, args...); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

func mustFail(t *testing.T, d *DB, why, q string, args ...any) error {
	t.Helper()
	ctx := context.Background()
	_, err := d.Conn(ctx).ExecContext(ctx, q, args...)
	if err == nil {
		t.Errorf("%s: la base aceptó %s %v", why, q, args)
	}
	return err
}

func count(t *testing.T, d *DB, table string) int {
	t.Helper()
	var n int
	ctx := context.Background()
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func usesAutoincrement(t *testing.T, d *DB, table string) bool {
	t.Helper()
	var sqlText string
	ctx := context.Background()
	if err := d.Conn(ctx).QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&sqlText); err != nil {
		t.Fatal(err)
	}
	return strings.Contains(strings.ToUpper(sqlText), "AUTOINCREMENT")
}

const insertDeploy = `INSERT INTO deploys (app_id, status, images, trigger, actor_type, actor_id, actor_name, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)`

func TestDeploysSchema(t *testing.T) {
	d := newTestDB(t)
	app := seedApp(t, d)

	mustExec(t, d, insertDeploy, app, "queued", `{"api":"ghcr.io/x/api:sha-1"}`, "api", "token", 7, "ci", 1)
	// Los valores por defecto: sin log (ni truncado), sin imágenes pedidas ni
	// de la configuración, sin error, sin tiempos de inicio y fin.
	var log, configImages, requestedImages string
	var truncated int
	var errText, superseded, started, finished any
	ctx := context.Background()
	if err := d.Conn(ctx).QueryRowContext(ctx,
		`SELECT log, log_truncated, requested_images, config_images, error, superseded_by, started_at, finished_at FROM deploys`).Scan(
		&log, &truncated, &requestedImages, &configImages, &errText, &superseded, &started, &finished); err != nil {
		t.Fatal(err)
	}
	if log != "" || truncated != 0 || requestedImages != "{}" || configImages != "{}" || errText != nil || superseded != nil || started != nil || finished != nil {
		t.Fatalf("valores por defecto: log=%q log_truncated=%d requested_images=%q config_images=%q error=%v superseded_by=%v started=%v finished=%v",
			log, truncated, requestedImages, configImages, errText, superseded, started, finished)
	}
	// log_truncated es una marca: 0 o 1 (contrato §2.5).
	mustExec(t, d, `UPDATE deploys SET log_truncated = 1 WHERE id = 1`)
	mustFail(t, d, "log_truncated fuera de 0 y 1", `UPDATE deploys SET log_truncated = 2 WHERE id = 1`)
	mustExec(t, d, `UPDATE deploys SET log_truncated = 0 WHERE id = 1`)

	for _, status := range []string{"queued", "running", "succeeded", "failed", "superseded"} {
		mustExec(t, d, insertDeploy, app, status, "{}", "panel", "user", 1, "ana", 2)
	}
	for _, trigger := range []string{"panel", "api", "rollback"} {
		mustExec(t, d, insertDeploy, app, "queued", "{}", trigger, "user", 1, "ana", 3)
	}
	mustFail(t, d, "status desconocido", insertDeploy, app, "done", "{}", "panel", "user", 1, "ana", 4)
	mustFail(t, d, "trigger desconocido", insertDeploy, app, "queued", "{}", "cron", "user", 1, "ana", 4)
	mustFail(t, d, "actor desconocido", insertDeploy, app, "queued", "{}", "panel", "system", nil, "", 4)
	mustFail(t, d, "app inexistente", insertDeploy, app+100, "queued", "{}", "panel", "user", 1, "ana", 4)

	// superseded_by apunta al deploy que lo reemplazó (la action de GitHub lo informa).
	mustExec(t, d, `UPDATE deploys SET status = 'superseded', superseded_by = (SELECT MAX(id) FROM deploys) WHERE id = 1`)

	// El id de un deploy sale en la API y en la auditoría: no se reutiliza.
	if !usesAutoincrement(t, d, "deploys") {
		t.Error("deploys debe usar AUTOINCREMENT")
	}

	// Borrar la app borra su historial de deploys.
	before := count(t, d, "deploys")
	if before < 9 {
		t.Fatalf("hay %d deploys", before)
	}
	mustExec(t, d, `DELETE FROM apps WHERE id = ?`, app)
	if n := count(t, d, "deploys"); n != 0 {
		t.Fatalf("quedan %d deploys de una app borrada", n)
	}
}

func TestRegistriesSchema(t *testing.T) {
	d := newTestDB(t)
	const insert = `INSERT INTO registries (host, username, password, created_at, updated_at) VALUES (?, ?, ?, 0, 0)`
	mustExec(t, d, insert, "ghcr.io", "bot", []byte{1, 2, 3})
	err := mustFail(t, d, "host repetido", insert, "ghcr.io", "otro", []byte{4})
	if !IsUnique(err) {
		t.Errorf("host repetido: IsUnique=false (%v)", err)
	}
	// El id va en la AAD de la contraseña (registries:password:<id>).
	if !usesAutoincrement(t, d, "registries") {
		t.Error("registries debe usar AUTOINCREMENT")
	}
}

const insertDomain = `INSERT INTO domains (app_id, hostname, target_service, target_port, proxied, tls_mode, cf_zone_id, status, last_error_code, created_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`

func TestDomainsSchema(t *testing.T) {
	d := newTestDB(t)
	app := seedApp(t, d)

	mustExec(t, d, insertDomain, app, "api.auditfy.cl", "api", 3000, 1, "cloudflare", "z1", "pending", nil)
	mustExec(t, d, insertDomain, app, "www.auditfy.cl", "api", 3000, 0, "http", nil, "error", "record_exists")
	err := mustFail(t, d, "hostname repetido", insertDomain, app, "api.auditfy.cl", "api", 3000, 0, "http", nil, "pending", nil)
	if !IsUnique(err) {
		t.Errorf("hostname repetido: IsUnique=false (%v)", err)
	}

	mustFail(t, d, "puerto 0", insertDomain, app, "a.auditfy.cl", "api", 0, 0, "http", nil, "pending", nil)
	mustFail(t, d, "puerto 65536", insertDomain, app, "a.auditfy.cl", "api", 65536, 0, "http", nil, "pending", nil)
	mustFail(t, d, "tls_mode desconocido", insertDomain, app, "a.auditfy.cl", "api", 80, 0, "acme", nil, "pending", nil)
	mustFail(t, d, "status desconocido", insertDomain, app, "a.auditfy.cl", "api", 80, 0, "http", nil, "ok", nil)
	mustFail(t, d, "proxied no booleano", insertDomain, app, "a.auditfy.cl", "api", 80, 2, "http", nil, "pending", nil)
	mustFail(t, d, "last_error_code desconocido", insertDomain, app, "a.auditfy.cl", "api", 80, 0, "http", nil, "error", "boom")
	// status error va siempre con su last_error_code (los del panel), y al revés.
	mustFail(t, d, "error sin last_error_code", insertDomain, app, "a.auditfy.cl", "api", 80, 0, "http", nil, "error", nil)
	mustFail(t, d, "last_error_code sin status error", insertDomain, app, "a.auditfy.cl", "api", 80, 0, "http", nil, "active", "cloudflare")
	for _, code := range []string{"record_exists", "cloudflare", "certificate"} {
		mustExec(t, d, insertDomain, app, code+".auditfy.cl", "api", 80, 0, "http", nil, "error", code)
	}

	// Los invariantes del DNS (contrato §3.1): el TLS por Cloudflare necesita
	// la zona, un registro siempre está en una zona, y muelle maneja el AAAA
	// solo si maneja el A.
	mustFail(t, d, "tls_mode cloudflare sin zona", insertDomain, app, "tls.auditfy.cl", "api", 80, 0, "cloudflare", nil, "pending", nil)
	const insertRecords = `INSERT INTO domains (app_id, hostname, target_service, target_port, tls_mode, cf_zone_id, cf_record_id, cf_record_id_aaaa, created_at, updated_at)
	VALUES (?, ?, 'api', 80, ?, ?, ?, ?, 0, 0)`
	mustFail(t, d, "cf_record_id sin zona", insertRecords, app, "zona.auditfy.cl", "http", nil, "r1", nil)
	mustFail(t, d, "cf_record_id_aaaa sin cf_record_id", insertRecords, app, "aaaa.auditfy.cl", "cloudflare", "z1", nil, "r6")
	mustExec(t, d, insertRecords, app, "full.auditfy.cl", "cloudflare", "z1", "r1", "r6")

	// Una app con dominios no se borra: primero se borran sus dominios, que
	// limpian sus registros de Cloudflare (§9).
	err = mustFail(t, d, "borrar una app con dominios", `DELETE FROM apps WHERE id = ?`, app)
	if !IsForeignKey(err) {
		t.Errorf("borrar una app con dominios: IsForeignKey=false (%v)", err)
	}
	mustExec(t, d, `DELETE FROM domains`)
	mustExec(t, d, `DELETE FROM apps WHERE id = ?`, app)
}
