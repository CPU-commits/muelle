package sqlite

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

const (
	keyA = "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgClaveA\n-----END PRIVATE KEY-----\n"
	keyB = "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgClaveB\n-----END PRIVATE KEY-----\n"
)

func testCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.New(bytes.Repeat([]byte{9}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func originCert(host, zone, cfID, key string, created time.Time) entity.OriginCert {
	return entity.OriginCert{
		Hostname: host, ZoneID: zone, CFCertID: cfID, CertPEM: "-----BEGIN CERTIFICATE-----\n" + cfID + "\n-----END CERTIFICATE-----\n",
		KeyPEM: key, ExpiresAt: created.Add(90 * 24 * time.Hour), State: entity.OriginActive, CreatedAt: created,
	}
}

// Ida y vuelta: la clave va cifrada (con la AAD de su id) y solo
// ActiveWithKeys la descifra. Insert retira el active anterior del mismo
// hostname en la misma transacción, con la clave borrada.
func TestOriginCertsInsertAndActive(t *testing.T) {
	db := sqlitetest.New(t)
	c := testCipher(t)
	repo := NewOriginCerts(db, c)
	ctx := context.Background()

	if m, err := repo.Active(ctx); err != nil || len(m) != 0 {
		t.Fatalf("vacío: %v %v", m, err)
	}
	if a, err := repo.ActiveFor(ctx, "api.auditfy.cl"); err != nil || a != nil {
		t.Fatalf("ActiveFor vacío: %v %v", a, err)
	}
	first := originCert("api.auditfy.cl", "z1", "100", keyA, t0)
	if err := repo.Insert(ctx, &first); err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 {
		t.Fatal("sin id")
	}
	var raw []byte
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT private_key FROM origin_ca_certs WHERE id = ?`, first.ID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("ClaveA")) || len(raw) == 0 {
		t.Fatalf("la clave no está cifrada: %q", raw)
	}
	if pt, err := c.Decrypt(raw, entity.OriginKeyAAD(first.ID)); err != nil || string(pt) != keyA {
		t.Fatalf("la AAD es origin_ca_certs:private_key:<id>: %q %v", pt, err)
	}

	panel := originCert("panel.auditfy.cl", "z1", "101", keyB, t0)
	if err := repo.Insert(ctx, &panel); err != nil {
		t.Fatal(err)
	}
	m, err := repo.Active(ctx)
	if err != nil || len(m) != 2 {
		t.Fatalf("Active: %v %v", m, err)
	}
	got := m["api.auditfy.cl"]
	if got.ID != first.ID || got.KeyPEM != "" || got.CFCertID != "100" || got.CertPEM != first.CertPEM || got.ZoneID != "z1" ||
		!got.ExpiresAt.Equal(first.ExpiresAt) || got.State != entity.OriginActive || !got.CreatedAt.Equal(t0) || got.RetiredAt != nil {
		t.Fatalf("Active sin clave: %+v", got)
	}
	keys, err := repo.ActiveWithKeys(ctx)
	if err != nil || keys["api.auditfy.cl"].KeyPEM != keyA || keys["panel.auditfy.cl"].KeyPEM != keyB {
		t.Fatalf("ActiveWithKeys: %v %v", keys, err)
	}
	if a, err := repo.ActiveFor(ctx, "panel.auditfy.cl"); err != nil || a == nil || a.ID != panel.ID || a.KeyPEM != "" {
		t.Fatalf("ActiveFor: %+v %v", a, err)
	}

	// La renovación: el nuevo es el active y el anterior queda retirado, sin
	// la clave, con retired_at = la hora del nuevo.
	t1 := t0.Add(60 * 24 * time.Hour)
	renewed := originCert("api.auditfy.cl", "z1", "102", keyB, t1)
	if err := repo.Insert(ctx, &renewed); err != nil {
		t.Fatal(err)
	}
	if a, _ := repo.ActiveFor(ctx, "api.auditfy.cl"); a == nil || a.ID != renewed.ID {
		t.Fatalf("el nuevo no es el active: %+v", a)
	}
	retired, err := repo.Retired(ctx)
	if err != nil || len(retired) != 1 || retired[0].ID != first.ID || retired[0].State != entity.OriginRetired ||
		retired[0].RetiredAt == nil || !retired[0].RetiredAt.Equal(t1) || retired[0].KeyPEM != "" {
		t.Fatalf("retirado: %+v %v", retired, err)
	}
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT private_key FROM origin_ca_certs WHERE id = ?`, first.ID).Scan(&raw); err != nil || len(raw) != 0 {
		t.Fatalf("la clave del retirado no se borró: %q %v", raw, err)
	}

	// Un cf_cert_id repetido falla y no retira el active.
	dup := originCert("api.auditfy.cl", "z1", "101", keyA, t1)
	if err := repo.Insert(ctx, &dup); err == nil {
		t.Fatal("cf_cert_id repetido")
	}
	if a, _ := repo.ActiveFor(ctx, "api.auditfy.cl"); a == nil || a.ID != renewed.ID {
		t.Fatalf("un Insert que falla no cambia el active: %+v", a)
	}
}

// Una clave copiada a otra fila no descifra (la AAD lleva el id), y el
// error nombra el hostname, nunca la clave.
func TestOriginCertsAADBoundToID(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewOriginCerts(db, testCipher(t))
	ctx := context.Background()
	a := originCert("a.auditfy.cl", "z1", "1", keyA, t0)
	b := originCert("b.auditfy.cl", "z1", "2", keyB, t0)
	for _, c := range []*entity.OriginCert{&a, &b} {
		if err := repo.Insert(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Conn(ctx).ExecContext(ctx, `UPDATE origin_ca_certs SET private_key = (SELECT private_key FROM origin_ca_certs WHERE id = ?) WHERE id = ?`, a.ID, b.ID); err != nil {
		t.Fatal(err)
	}
	_, err := repo.ActiveWithKeys(ctx)
	if !errors.Is(err, cipher.ErrDecrypt) || !strings.Contains(err.Error(), "b.auditfy.cl") || strings.Contains(err.Error(), "Clave") {
		t.Fatalf("clave de otra fila: %v", err)
	}
}

// Retire deja el certificado retirado, sin la clave; uno que no está active
// es ErrNotFound. InsertRetired guarda uno ya retirado (la prueba del
// permiso cuyo DELETE falló).
func TestOriginCertsRetire(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewOriginCerts(db, testCipher(t))
	ctx := context.Background()
	c := originCert("api.auditfy.cl", "z1", "100", keyA, t0)
	if err := repo.Insert(ctx, &c); err != nil {
		t.Fatal(err)
	}
	at := t0.Add(time.Hour)
	if err := repo.Retire(ctx, c.ID, at); err != nil {
		t.Fatal(err)
	}
	if err := repo.Retire(ctx, c.ID, at); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("retirar dos veces: %v", err)
	}
	if m, _ := repo.Active(ctx); len(m) != 0 {
		t.Fatalf("sigue active: %v", m)
	}
	var raw []byte
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT private_key FROM origin_ca_certs WHERE id = ?`, c.ID).Scan(&raw); err != nil || len(raw) != 0 {
		t.Fatalf("la clave: %q %v", raw, err)
	}
	probe := originCert("auditfy.cl", "z1", "200", "", t0)
	if err := repo.InsertRetired(ctx, &probe, at); err != nil {
		t.Fatal(err)
	}
	r, err := repo.Retired(ctx)
	if err != nil || len(r) != 2 || r[1].ID != probe.ID || r[1].Hostname != "auditfy.cl" || r[1].RetiredAt == nil || !r[1].RetiredAt.Equal(at) {
		t.Fatalf("retirados: %+v %v", r, err)
	}

	if err := repo.Delete(ctx, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Delete(ctx, c.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("borrar dos veces: %v", err)
	}
}

// RetiredDue son los retirados que ya se pueden revocar (retired_at antes
// de lastGoodBuild, con grace de margen, y sin un reintento por delante), y
// los vencidos, que se borran sin revocar.
func TestOriginCertsRetiredDue(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewOriginCerts(db, testCipher(t))
	ctx := context.Background()
	retire := func(cfID string, at time.Time, expires time.Time) int64 {
		t.Helper()
		c := originCert(cfID+".auditfy.cl", "z1", cfID, keyA, t0)
		c.ExpiresAt = expires
		if err := repo.Insert(ctx, &c); err != nil {
			t.Fatal(err)
		}
		if err := repo.Retire(ctx, c.ID, at); err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	now := t0.Add(10 * time.Minute)
	lgb := t0.Add(5 * time.Minute) // el último Sync bueno empezó a leer a los 5 min
	grace := time.Minute
	far := t0.Add(90 * 24 * time.Hour)
	due := retire("due", t0, far)                                              // retirado antes del Sync y hace más de 1 min
	retire("after", t0.Add(6*time.Minute), far)                                // retirado después de que empezó el Sync
	retire("equal", lgb, far)                                                  // justo cuando empezó: no
	retire("grace", now.Add(-30*time.Second), far)                             // dentro del margen (y después del Sync)
	expired := retire("expired", t0.Add(9*time.Minute), now.Add(-time.Second)) // vencido: se borra igual
	backoff := retire("backoff", t0, far)
	if err := repo.RevokeFailed(ctx, backoff, 3, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	ready := retire("ready", t0, far)
	if err := repo.RevokeFailed(ctx, ready, 1, now); err != nil {
		t.Fatal(err)
	}

	got, err := repo.RetiredDue(ctx, now, lgb, grace)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[int64]entity.OriginCert{}
	for _, c := range got {
		ids[c.ID] = c
	}
	if len(got) != 3 || ids[due].ID == 0 || ids[expired].ID == 0 || ids[ready].RevokeFailures != 1 {
		t.Fatalf("due: %+v", got)
	}
	if ids[ready].NextRevokeAt == nil || !ids[ready].NextRevokeAt.Equal(now) {
		t.Fatalf("next_revoke_at: %+v", ids[ready])
	}
	// Sin un Sync bueno todavía (lastGoodBuild cero), solo los vencidos.
	got, err = repo.RetiredDue(ctx, now, time.Time{}, grace)
	if err != nil || len(got) != 1 || got[0].ID != expired {
		t.Fatalf("sin Sync: %+v %v", got, err)
	}
	if err := repo.RevokeFailed(ctx, 999, 1, now); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("RevokeFailed de uno que no está: %v", err)
	}
}

// origin_ca_hosts: lo observado, la falla con su reintento, el reinicio del
// backoff y el olvido.
func TestOriginHosts(t *testing.T) {
	db := sqlitetest.New(t)
	repo := NewOriginHosts(db)
	ctx := context.Background()

	if h, err := repo.Get(ctx, "api.auditfy.cl"); err != nil || h.Hostname != "api.auditfy.cl" || h.Observed != "" || h.Failed() {
		t.Fatalf("sin fila: %+v %v", h, err)
	}
	t1 := t0.Add(time.Minute)
	if err := repo.SetObserved(ctx, "api.auditfy.cl", entity.ObservedProxied, t1); err != nil {
		t.Fatal(err)
	}
	h, err := repo.Get(ctx, "api.auditfy.cl")
	if err != nil || h.Observed != entity.ObservedProxied || h.ObservedAt == nil || !h.ObservedAt.Equal(t1) || !h.UpdatedAt.Equal(t1) || h.Failed() {
		t.Fatalf("observado: %+v %v", h, err)
	}
	next := t1.Add(6 * time.Hour)
	if err := repo.RecordFailure(ctx, "api.auditfy.cl", 2, entity.OriginPermission, "el token no tiene el permiso", next, t1); err != nil {
		t.Fatal(err)
	}
	// Una falla de un host que todavía no tiene fila también se guarda.
	if err := repo.RecordFailure(ctx, "panel.auditfy.cl", 1, entity.OriginCloudflare, "Cloudflare no respondió", t1.Add(time.Minute), t1); err != nil {
		t.Fatal(err)
	}
	all, err := repo.All(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("All: %v %v", all, err)
	}
	h = all["api.auditfy.cl"]
	if h.Observed != entity.ObservedProxied || h.Failures != 2 || h.ErrorCode != entity.OriginPermission ||
		h.LastError != "el token no tiene el permiso" || h.NextAttemptAt == nil || !h.NextAttemptAt.Equal(next) {
		t.Fatalf("falla: %+v", h)
	}
	if p := all["panel.auditfy.cl"]; p.Observed != "" || p.Failures != 1 || p.ErrorCode != entity.OriginCloudflare {
		t.Fatalf("falla sin fila: %+v", p)
	}

	// ResetBackoff: next_attempt_at = la hora del pedido, solo en los que
	// tienen una falla; la falla sigue.
	t2 := t1.Add(time.Minute)
	if err := repo.SetObserved(ctx, "web.auditfy.cl", entity.ObservedDNSOnly, t1); err != nil {
		t.Fatal(err)
	}
	if err := repo.ResetBackoff(ctx, "api.auditfy.cl", t2); err != nil {
		t.Fatal(err)
	}
	all, _ = repo.All(ctx)
	if h := all["api.auditfy.cl"]; h.Failures != 2 || !h.NextAttemptAt.Equal(t2) || h.LastError == "" {
		t.Fatalf("reinicio de uno: %+v", h)
	}
	if p := all["panel.auditfy.cl"]; !p.NextAttemptAt.Equal(t1.Add(time.Minute)) {
		t.Fatalf("el reinicio de uno no toca a otro: %+v", p)
	}
	t3 := t2.Add(time.Minute)
	if err := repo.ResetBackoff(ctx, "", t3); err != nil {
		t.Fatal(err)
	}
	all, _ = repo.All(ctx)
	if !all["api.auditfy.cl"].NextAttemptAt.Equal(t3) || !all["panel.auditfy.cl"].NextAttemptAt.Equal(t3) || all["web.auditfy.cl"].NextAttemptAt != nil {
		t.Fatalf("reinicio de todos: %+v", all)
	}

	// ForgetObserved vuelve observed a NULL y deja la falla.
	if err := repo.ForgetObserved(ctx, "api.auditfy.cl", t3); err != nil {
		t.Fatal(err)
	}
	if err := repo.ForgetObserved(ctx, "nada.auditfy.cl", t3); err != nil {
		t.Fatalf("olvidar uno sin fila: %v", err)
	}
	if h, _ := repo.Get(ctx, "api.auditfy.cl"); h.Observed != "" || h.ObservedAt != nil || h.Failures != 2 {
		t.Fatalf("olvidado: %+v", h)
	}
	// ClearFailure borra la falla y su reintento.
	if err := repo.ClearFailure(ctx, "api.auditfy.cl", t3); err != nil {
		t.Fatal(err)
	}
	if h, _ := repo.Get(ctx, "api.auditfy.cl"); h.Failed() || h.ErrorCode != "" || h.LastError != "" || h.NextAttemptAt != nil {
		t.Fatalf("sin falla: %+v", h)
	}
	if err := repo.Delete(ctx, "api.auditfy.cl"); err != nil {
		t.Fatal(err)
	}
	if all, _ := repo.All(ctx); len(all) != 2 {
		t.Fatalf("borrado: %v", all)
	}
	if err := repo.DeleteAll(ctx); err != nil {
		t.Fatal(err)
	}
	if all, _ := repo.All(ctx); len(all) != 0 {
		t.Fatalf("DeleteAll: %v", all)
	}
}
