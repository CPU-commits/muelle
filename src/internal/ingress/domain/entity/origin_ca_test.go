package entity

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"
)

const testKeyPEM = "-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgSecretoDeLaClave\n-----END PRIVATE KEY-----\n"

// La clave de un certificado de Origin CA nunca sale en un %v, en un log ni
// en un JSON (contrato §3.7), ni sola ni dentro de otro valor: solo la
// necesita Caddy.
func TestOriginCertHidesKey(t *testing.T) {
	exp := time.Date(2027, 1, 19, 12, 0, 0, 0, time.UTC)
	c := OriginCert{ID: 7, Hostname: "api.auditfy.cl", ZoneID: "z1", CFCertID: "123456", CertPEM: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n",
		KeyPEM: testKeyPEM, ExpiresAt: exp, State: OriginActive}
	nested := struct {
		C OriginCert
		P *OriginCert
	}{c, &c}
	var out []string
	for _, v := range []any{c, &c, nested, &nested} {
		out = append(out, fmt.Sprintf("%v", v), fmt.Sprintf("%+v", v), fmt.Sprintf("%#v", v), fmt.Sprint(v))
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	var buf bytes.Buffer
	for _, h := range []slog.Handler{slog.NewJSONHandler(&buf, nil), slog.NewTextHandler(&buf, nil)} {
		slog.New(h).Info("cert", "c", c, "p", &c, "nested", nested)
	}
	out = append(out, buf.String())
	for _, o := range out {
		if strings.Contains(o, "SecretoDeLaClave") || strings.Contains(o, "PRIVATE KEY") {
			t.Fatalf("filtra la clave: %s", o)
		}
	}
	if s := c.String(); !strings.Contains(s, "api.auditfy.cl") || !strings.Contains(s, "123456") || !strings.Contains(s, "***") {
		t.Fatalf("String esconde solo la clave: %s", s)
	}
	if s := (OriginCert{Hostname: "a.cl"}).String(); strings.Contains(s, "***") {
		t.Fatalf("sin clave no hay nada que esconder: %s", s)
	}
	if c.KeyPEM != testKeyPEM {
		t.Fatal("ocultarla no cambia el valor")
	}
}

func TestOriginEnums(t *testing.T) {
	for _, o := range []Observation{ObservedProxied, ObservedDNSOnly, ObservedMissing, ObservedUnreadable} {
		if !o.Valid() {
			t.Errorf("%q no es válida", o)
		}
	}
	if Observation("").Valid() || Observation("grey").Valid() {
		t.Error("una observación desconocida es válida")
	}
	for _, c := range []OriginErrorCode{OriginPermission, OriginCloudflare, OriginRejected} {
		if !c.Valid() {
			t.Errorf("%q no es válido", c)
		}
	}
	if OriginErrorCode("").Valid() || OriginErrorCode("certificate").Valid() {
		t.Error("un código desconocido es válido")
	}
	h := OriginHost{Hostname: "a.cl"}
	if h.Failed() {
		t.Error("sin fallas")
	}
	h.Failures = 1
	if !h.Failed() {
		t.Error("con fallas")
	}
}
