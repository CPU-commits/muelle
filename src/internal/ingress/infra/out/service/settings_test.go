package service

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/ingresstest"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

func newCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func setRaw(t *testing.T, db *sqlite.DB, key string, value any, encrypted bool) {
	t.Helper()
	enc := 0
	if encrypted {
		enc = 1
	}
	ctx := context.Background()
	if _, err := db.Conn(ctx).ExecContext(ctx, `INSERT INTO settings (key, value, encrypted) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, encrypted = excluded.encrypted`, key, value, enc); err != nil {
		t.Fatal(err)
	}
}

// Lo que admin guarda (Encode y el token cifrado con settings:cloudflare_token)
// es lo que ingress lee.
func TestProxySettingsReader(t *testing.T) {
	db := sqlitetest.New(t)
	c := newCipher(t)
	r := NewProxySettingsReader(db, c)
	ctx := context.Background()

	got, err := r.ProxySettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, entity.DefaultProxySettings()) {
		t.Fatalf("base vacía: %+v", got)
	}

	want := entity.ProxySettings{
		CloudflareToken: "cf-token", PublicIP: "203.0.113.7", PublicIPv6: "2001:db8::7",
		ACMEEmail: "tu@correo.cl", PanelDomain: "panel.tudominio.cl",
		PanelAllowedCIDRs: []string{"203.0.113.0/24"}, HTTPPort: 8080, HTTPSPort: 8443,
	}
	for k, v := range want.Encode() {
		setRaw(t, db, k, v, false)
	}
	setRaw(t, db, entity.SettingCloudflareToken, c.Encrypt([]byte("cf-token"), entity.SettingCloudflareTokenAAD), true)
	got, err = r.ProxySettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("leído:\n%+v\nesperaba\n%+v", got, want)
	}
}

// Un token cifrado con otra AAD (copiado de otra fila) o sin cifrar no se usa.
func TestProxySettingsReaderRejectsBadToken(t *testing.T) {
	db := sqlitetest.New(t)
	c := newCipher(t)
	r := NewProxySettingsReader(db, c)
	ctx := context.Background()

	setRaw(t, db, entity.SettingCloudflareToken, c.Encrypt([]byte("cf-token"), "settings:otra"), true)
	if _, err := r.ProxySettings(ctx); err == nil || !strings.Contains(err.Error(), entity.SettingCloudflareToken) {
		t.Fatalf("token con otra AAD: %v", err)
	}
	setRaw(t, db, entity.SettingCloudflareToken, "en-claro", false)
	if _, err := r.ProxySettings(ctx); err == nil || !strings.Contains(err.Error(), entity.SettingCloudflareToken) {
		t.Fatalf("token sin cifrar: %v", err)
	}
}

func TestProxySettingsReaderRejectsGarbage(t *testing.T) {
	db := sqlitetest.New(t)
	setRaw(t, db, entity.SettingProxyHTTPPort, "ochenta", false)
	if _, err := NewProxySettingsReader(db, newCipher(t)).ProxySettings(context.Background()); err == nil {
		t.Fatal("un puerto que no es número debió fallar")
	}
}

func countKey(t *testing.T, db *sqlite.DB, key string) int {
	t.Helper()
	var n int
	ctx := context.Background()
	if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key = ?`, key).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPanelDNSRoundTrip(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewProxySettingsReader(db, newCipher(t))
	ctx := context.Background()

	if got, err := r.PanelDNS(ctx); err != nil || got != (service.PanelDNS{}) {
		t.Fatalf("vacío: %+v %v", got, err)
	}
	full := service.PanelDNS{ZoneID: "z1", RecordID: "r4", RecordIDAAAA: "r6"}
	if err := r.SavePanelDNS(ctx, full); err != nil {
		t.Fatal(err)
	}
	if got, err := r.PanelDNS(ctx); err != nil || got != full {
		t.Fatalf("los tres: %+v %v", got, err)
	}
	// Guardar otra vez pisa (ON CONFLICT), y una vacía borra su fila.
	noAAAA := service.PanelDNS{ZoneID: "z2", RecordID: "r5"}
	if err := r.SavePanelDNS(ctx, noAAAA); err != nil {
		t.Fatal(err)
	}
	if got, err := r.PanelDNS(ctx); err != nil || got != noAAAA {
		t.Fatalf("sin AAAA: %+v %v", got, err)
	}
	if n := countKey(t, db, entity.SettingPanelCFRecordIDAAAA); n != 0 {
		t.Fatalf("la clave vacía debió borrarse: %d filas", n)
	}
	if err := r.SavePanelDNS(ctx, service.PanelDNS{}); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{entity.SettingPanelCFZoneID, entity.SettingPanelCFRecordID, entity.SettingPanelCFRecordIDAAAA} {
		if n := countKey(t, db, k); n != 0 {
			t.Fatalf("%s debió borrarse: %d filas", k, n)
		}
	}
	// Las claves del panel no son ajustes del proxy.
	if err := r.SavePanelDNS(ctx, full); err != nil {
		t.Fatal(err)
	}
	if got, err := r.ProxySettings(ctx); err != nil || !reflect.DeepEqual(got, entity.DefaultProxySettings()) {
		t.Fatalf("ProxySettings con las claves del panel: %+v %v", got, err)
	}
}

func TestSaveZonesCompareAndSet(t *testing.T) {
	db := sqlitetest.New(t)
	c := newCipher(t)
	r := NewProxySettingsReader(db, c)
	ctx := context.Background()
	zones := func() string {
		t.Helper()
		var v string
		if err := db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, entity.SettingCloudflareZones).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}

	// Sin token guardado no se guarda nada.
	if saved, err := r.SaveZones(ctx, "t1", 3); err != nil || saved {
		t.Fatalf("sin token: saved=%v %v", saved, err)
	}
	if n := countKey(t, db, entity.SettingCloudflareZones); n != 0 {
		t.Fatalf("sin token: %d filas", n)
	}

	p := entity.DefaultProxySettings()
	p.CloudflareToken = "t1"
	ingresstest.SetProxySettings(t, db, c, p)
	if saved, err := r.SaveZones(ctx, "t1", 3); err != nil || !saved {
		t.Fatalf("mismo token: saved=%v %v", saved, err)
	}
	if v := zones(); v != "3" {
		t.Fatalf("cloudflare_zones = %q", v)
	}
	// Otro token (el guardado cambió mientras se listaban las zonas): no se pisa.
	if saved, err := r.SaveZones(ctx, "t2", 5); err != nil || saved {
		t.Fatalf("otro token: saved=%v %v", saved, err)
	}
	if v := zones(); v != "3" {
		t.Fatalf("cloudflare_zones con otro token = %q", v)
	}
	if saved, err := r.SaveZones(ctx, "", 5); err != nil || saved {
		t.Fatalf("token vacío: saved=%v %v", saved, err)
	}
	// Un token guardado que no se puede descifrar es un error, y no se guarda.
	setRaw(t, db, entity.SettingCloudflareToken, c.Encrypt([]byte("t1"), "settings:otra"), true)
	if saved, err := r.SaveZones(ctx, "t1", 7); err == nil || saved {
		t.Fatalf("token con otra AAD: saved=%v %v", saved, err)
	}
	if v := zones(); v != "3" {
		t.Fatalf("cloudflare_zones con un token ilegible = %q", v)
	}
}
