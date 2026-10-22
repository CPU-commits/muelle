package sqlite

import (
	"context"
	"testing"
)

// 0006 (contrato §6, §5.4 del diseño de Origin CA): origin_ca_certs, los
// certificados de Cloudflare Origin CA que emitió muelle, con la clave
// cifrada y su estado, y origin_ca_hosts, lo que vio de cada hostname en
// Cloudflare y la última falla con su reintento.

const (
	insertOriginCert = `INSERT INTO origin_ca_certs (hostname, zone_id, cf_cert_id, certificate, private_key, expires_at, state, created_at, retired_at)
	VALUES (?, ?, ?, 'PEM', x'01', 0, ?, 0, ?)`
	insertOriginHost = `INSERT INTO origin_ca_hosts (hostname, observed, failures, last_error_code, last_error, next_attempt_at, updated_at)
	VALUES (?, ?, ?, ?, ?, ?, 0)`
)

func TestOriginCACertsSchema(t *testing.T) {
	d := newTestDB(t)

	mustExec(t, d, insertOriginCert, "api.auditfy.cl", "z1", "100", "active", nil)
	// Un activo y un retirado del mismo hostname conviven (la renovación).
	mustExec(t, d, insertOriginCert, "api.auditfy.cl", "z1", "101", "retired", 5)
	mustExec(t, d, insertOriginCert, "api.auditfy.cl", "z2", "102", "retired", 6)
	// A lo más un activo por hostname.
	if err := mustFail(t, d, "dos activos del mismo hostname", insertOriginCert, "api.auditfy.cl", "z1", "103", "active", nil); !IsUnique(err) {
		t.Errorf("dos activos: IsUnique=false (%v)", err)
	}
	mustExec(t, d, insertOriginCert, "panel.auditfy.cl", "z1", "104", "active", nil)
	// El id de Cloudflare es único: es el que se revoca.
	if err := mustFail(t, d, "cf_cert_id repetido", insertOriginCert, "web.auditfy.cl", "z1", "100", "active", nil); !IsUnique(err) {
		t.Errorf("cf_cert_id repetido: IsUnique=false (%v)", err)
	}
	mustFail(t, d, "estado desconocido", insertOriginCert, "web.auditfy.cl", "z1", "105", "revoked", nil)
	mustFail(t, d, "retirado sin retired_at", insertOriginCert, "web.auditfy.cl", "z1", "105", "retired", nil)
	mustFail(t, d, "activo con retired_at", insertOriginCert, "web.auditfy.cl", "z1", "105", "active", 7)
	for _, col := range []string{"hostname", "zone_id", "cf_cert_id", "certificate", "private_key", "expires_at", "created_at"} {
		mustFail(t, d, col+" NULL", `UPDATE origin_ca_certs SET `+col+` = NULL WHERE cf_cert_id = '100'`)
	}

	// Retirar uno: el estado, la hora y la clave vacía.
	mustExec(t, d, `UPDATE origin_ca_certs SET state = 'retired', retired_at = 9, private_key = x'' WHERE cf_cert_id = '104'`)
	var failures int
	var next any
	ctx := context.Background()
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT revoke_failures, next_revoke_at FROM origin_ca_certs WHERE cf_cert_id = '104'`).Scan(&failures, &next); err != nil {
		t.Fatal(err)
	}
	if failures != 0 || next != nil {
		t.Fatalf("por defecto: revoke_failures=%d next_revoke_at=%v", failures, next)
	}

	// El id va en la AAD de la clave (origin_ca_certs:private_key:<id>): no se reutiliza.
	if !usesAutoincrement(t, d, "origin_ca_certs") {
		t.Error("origin_ca_certs debe usar AUTOINCREMENT")
	}
}

func TestOriginCAHostsSchema(t *testing.T) {
	d := newTestDB(t)

	// Sin mirar todavía, sin falla.
	mustExec(t, d, insertOriginHost, "api.auditfy.cl", nil, 0, nil, nil, nil)
	for _, obs := range []string{"proxied", "dns_only", "missing", "unreadable"} {
		mustExec(t, d, insertOriginHost, obs+".auditfy.cl", obs, 0, nil, nil, nil)
	}
	// Una falla trae su código, su causa y su reintento.
	for _, code := range []string{"permission", "cloudflare", "rejected"} {
		mustExec(t, d, insertOriginHost, code+".falla.cl", "proxied", 2, code, "la causa", 1000)
	}
	err := mustFail(t, d, "hostname repetido", insertOriginHost, "api.auditfy.cl", nil, 0, nil, nil, nil)
	if !IsUnique(err) {
		t.Errorf("hostname repetido: IsUnique=false (%v)", err)
	}
	mustFail(t, d, "observed desconocido", insertOriginHost, "a.auditfy.cl", "grey", 0, nil, nil, nil)
	mustFail(t, d, "código desconocido", insertOriginHost, "a.auditfy.cl", nil, 1, "certificate", "x", 1)
	mustFail(t, d, "falla sin código", insertOriginHost, "a.auditfy.cl", nil, 1, nil, nil, 1)
	mustFail(t, d, "código sin causa", insertOriginHost, "a.auditfy.cl", nil, 1, "cloudflare", nil, 1)
	mustFail(t, d, "causa sin código", insertOriginHost, "a.auditfy.cl", nil, 0, nil, "x", nil)
	mustFail(t, d, "código sin fallas", insertOriginHost, "a.auditfy.cl", nil, 0, "cloudflare", "x", nil)
	// retry_at nunca es null con error_code (contrato §3.7).
	mustFail(t, d, "falla sin reintento", insertOriginHost, "a.auditfy.cl", nil, 1, "cloudflare", "x", nil)
	mustFail(t, d, "reintento sin falla", insertOriginHost, "a.auditfy.cl", nil, 0, nil, nil, 1)
	mustFail(t, d, "updated_at NULL", `UPDATE origin_ca_hosts SET updated_at = NULL WHERE hostname = 'api.auditfy.cl'`)
}

// La 0006 corre sobre una base que ya tiene dominios (como la de una versión
// anterior), sin tocarlos.
func TestOriginCAMigrationOverExistingDomains(t *testing.T) {
	d := openAt(t, 5)
	app := seedApp(t, d)
	mustExec(t, d, insertDomain, app, "api.auditfy.cl", "api", 3000, 1, "cloudflare", "z1", "pending", nil)
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := count(t, d, "domains"); n != 1 {
		t.Fatalf("dominios después de la 0006: %d", n)
	}
	if n := count(t, d, "origin_ca_certs") + count(t, d, "origin_ca_hosts"); n != 0 {
		t.Fatalf("las tablas nuevas empiezan vacías: %d", n)
	}
	var v int
	ctx := context.Background()
	if err := d.Conn(ctx).QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&v); err != nil || v < 6 {
		t.Fatalf("versión: %d %v", v, err)
	}
}
