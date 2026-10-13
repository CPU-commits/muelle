package service

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

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
