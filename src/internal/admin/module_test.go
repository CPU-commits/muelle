package admin

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	"github.com/CPU-commits/muelle/src/internal/admin/admintest"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

type env struct {
	m       *Module
	db      *sqlite.DB
	clk     *clock.Manual
	dataDir string
	c       *httptestutil.Client
	// Sesiones de prueba: "sin" no tiene TOTP, "con" sí.
	sinID, conID int64
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := sqlitetest.New(t)
	clk := clock.NewManual(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	ciph, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	e := &env{db: db, clk: clk, dataDir: t.TempDir()}
	e.m = New(Deps{
		DB: db, Cipher: ciph, Clock: clk,
		Audit:   sqlite.NewAuditRecorder(db, clk, nil),
		DataDir: e.dataDir,
	})
	e.sinID = admintest.InsertUser(t, db, "sin-totp", false)
	e.conID = admintest.InsertUser(t, db, "con-totp", true)
	auth := httptestutil.FakeAuth{
		Sessions: map[string]entity.Principal{
			"sin": {UserID: e.sinID, Username: "sin-totp"},
			"con": {UserID: e.conID, Username: "con-totp"},
		},
		Tokens: map[string]entity.Principal{
			"tok": {UserID: e.conID, Token: &entity.TokenScope{ID: 1, ProjectSlug: "p"}},
		},
	}
	e.c = httptestutil.New(t, auth, e.m.RegisterRoutes)
	return e
}

func TestSettingsAndAuditAPI(t *testing.T) {
	e := newEnv(t)
	sin, con := httptestutil.WithSession("sin"), httptestutil.WithSession("con")

	w := e.c.Do("GET", "/api/v1/settings", nil, sin)
	httptestutil.Expect(t, w, http.StatusOK)
	if httptestutil.Decode[map[string]bool](t, w)["require_2fa"] {
		t.Fatal("require_2fa debe empezar en false")
	}

	// Sin su propio 2FA no puede exigirlo.
	w = e.c.Do("PATCH", "/api/v1/settings", map[string]bool{"require_2fa": true}, sin)
	httptestutil.Expect(t, w, http.StatusConflict)
	if code := httptestutil.ErrorCode(t, w); code != "totp_required_first" {
		t.Fatalf("code=%s", code)
	}

	// Con su propio 2FA configurado, sí.
	w = e.c.Do("PATCH", "/api/v1/settings", map[string]bool{"require_2fa": true}, con)
	httptestutil.Expect(t, w, http.StatusOK)
	if !httptestutil.Decode[map[string]bool](t, w)["require_2fa"] {
		t.Fatal("require_2fa no se guardó")
	}
	w = e.c.Do("GET", "/api/v1/settings", nil, sin)
	if !httptestutil.Decode[map[string]bool](t, w)["require_2fa"] {
		t.Fatal("GET no refleja el cambio")
	}

	w = e.c.Do("GET", "/api/v1/audit?limit=1", nil, con)
	httptestutil.Expect(t, w, http.StatusOK)
	events := httptestutil.Decode[[]map[string]any](t, w)
	if len(events) != 1 || events[0]["action"] != "settings.update" || events[0]["actor_type"] != "user" {
		t.Fatalf("events=%v", events)
	}
	if d, _ := events[0]["details"].(map[string]any); d["require_2fa"] != true {
		t.Fatalf("details=%v", events[0]["details"])
	}
	httptestutil.Expect(t, e.c.Do("GET", "/api/v1/audit?limit=0", nil, con), http.StatusUnprocessableEntity)
}

func TestAuditPagination(t *testing.T) {
	e := newEnv(t)
	con := httptestutil.WithSession("con")
	for _, v := range []bool{true, false, true} {
		httptestutil.Expect(t, e.c.Do("PATCH", "/api/v1/settings", map[string]bool{"require_2fa": v}, con), http.StatusOK)
	}
	all := httptestutil.Decode[[]map[string]any](t, e.c.Do("GET", "/api/v1/audit", nil, con))
	if len(all) != 3 {
		t.Fatalf("all=%v", all)
	}
	id := int64(all[0]["id"].(float64))
	if id <= int64(all[1]["id"].(float64)) {
		t.Fatal("deben venir por id descendente")
	}
	w := e.c.Do("GET", "/api/v1/audit?limit=1&before="+strconv.FormatInt(id, 10), nil, con)
	page := httptestutil.Decode[[]map[string]any](t, w)
	if len(page) != 1 || page[0]["id"] != all[1]["id"] {
		t.Fatalf("page=%v", page)
	}
	for _, q := range []string{"limit=501", "limit=abc", "before=0", "before=x"} {
		httptestutil.Expect(t, e.c.Do("GET", "/api/v1/audit?"+q, nil, con), http.StatusUnprocessableEntity)
	}
}

func TestAdminEndpointsAccess(t *testing.T) {
	e := newEnv(t)
	for _, r := range [][2]string{{"GET", "/api/v1/settings"}, {"PATCH", "/api/v1/settings"}, {"GET", "/api/v1/audit"}} {
		// Sin sesión: 401.
		httptestutil.Expect(t, e.c.Do(r[0], r[1], nil), http.StatusUnauthorized)
		// Un token de API no entra: solo sesión.
		httptestutil.Expect(t, e.c.Do(r[0], r[1], nil, httptestutil.WithBearer("tok")), http.StatusForbidden)
	}
}

func TestPatchSettingsBadBody(t *testing.T) {
	e := newEnv(t)
	con := httptestutil.WithSession("con")
	for _, body := range []any{`{"require_2fa":`, `{"otra":true}`, `{"require_2fa":"si"}`} {
		w := e.c.Do("PATCH", "/api/v1/settings", body, con)
		httptestutil.Expect(t, w, http.StatusBadRequest)
		if code := httptestutil.ErrorCode(t, w); code != "invalid_json" {
			t.Fatalf("code=%s", code)
		}
	}
	// Un body vacío ({}) no cambia nada.
	httptestutil.Expect(t, e.c.Do("PATCH", "/api/v1/settings", map[string]bool{}, con), http.StatusOK)
}

func TestNewValidatesDeps(t *testing.T) {
	e := newEnv(t)
	ciph, _ := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	rec := sqlite.NewAuditRecorder(e.db, e.clk, nil)
	full := Deps{DB: e.db, Cipher: ciph, Audit: rec, DataDir: t.TempDir()}

	// Sin Clock usa el reloj del sistema.
	if m := New(full); m == nil {
		t.Fatal("New devolvió nil")
	}

	cases := map[string]func(d *Deps){
		"Deps.DB":      func(d *Deps) { d.DB = nil },
		"Deps.Cipher":  func(d *Deps) { d.Cipher = nil },
		"Deps.Audit":   func(d *Deps) { d.Audit = nil },
		"Deps.DataDir": func(d *Deps) { d.DataDir = "" },
	}
	for want, mutate := range cases {
		d := full
		mutate(&d)
		func() {
			defer func() {
				msg, _ := recover().(string)
				if !strings.Contains(msg, "admin.New: falta "+want) {
					t.Errorf("%s: panic=%q", want, msg)
				}
			}()
			New(d)
		}()
	}
}

func TestVerifyMasterKey(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	if err := e.m.VerifyMasterKey(ctx); err != nil {
		t.Fatal(err)
	}
	if err := e.m.VerifyMasterKey(ctx); err != nil {
		t.Fatal(err)
	}
	var enc int
	if err := e.db.Conn(ctx).QueryRowContext(ctx, `SELECT encrypted FROM settings WHERE key = 'master_key_check'`).Scan(&enc); err != nil || enc != 1 {
		t.Fatalf("enc=%d err=%v", enc, err)
	}

	other, _ := crypto.New(bytes.Repeat([]byte{9}, crypto.KeySize))
	m2 := New(Deps{DB: e.db, Cipher: other, Clock: e.clk, Audit: sqlite.NewAuditRecorder(e.db, e.clk, nil), DataDir: e.dataDir})
	err := m2.VerifyMasterKey(ctx)
	if !errors.Is(err, ErrWrongMasterKey) || err.Error() != "clave maestra incorrecta" {
		t.Fatalf("err=%v", err)
	}
}

func TestJobs(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	jobs := e.m.Jobs()
	if len(jobs) != 2 || jobs[0].Name != "retención de auditoría" || jobs[1].Name != "backup diario" {
		t.Fatalf("jobs=%+v", jobs)
	}
	for _, j := range jobs {
		if j.Every != time.Hour {
			t.Fatalf("%s corre cada %v", j.Name, j.Every)
		}
	}

	// Retención: un evento de hace 181 días se purga.
	rec := sqlite.NewAuditRecorder(e.db, e.clk, nil)
	rec.Record(ctx, "viejo", "t", nil)
	e.clk.Advance(181 * 24 * time.Hour)
	rec.Record(ctx, "nuevo", "t", nil)
	if err := jobs[0].Run(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	e.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_events`).Scan(&n)
	if n != 1 {
		t.Fatalf("quedan %d eventos", n)
	}

	// Backup: dos corridas el mismo día dejan una sola copia.
	for range 2 {
		if err := jobs[1].Run(ctx); err != nil {
			t.Fatal(err)
		}
	}
	files, _ := filepath.Glob(filepath.Join(e.dataDir, "backups", "muelle-*.db"))
	if len(files) != 1 || filepath.Base(files[0]) != "muelle-2027-04-09.db" {
		t.Fatalf("files=%v", files)
	}
}
