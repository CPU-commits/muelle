package usecases

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/admintest"
	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	adminrepo "github.com/CPU-commits/muelle/src/internal/admin/infra/out/repository/sqlite"
	adminsvc "github.com/CPU-commits/muelle/src/internal/admin/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

type env struct {
	db  *sqlite.DB
	clk *clock.Manual
	rec *sqlite.AuditRecorder
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := sqlitetest.New(t)
	clk := clock.NewManual(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC))
	return &env{db: db, clk: clk, rec: sqlite.NewAuditRecorder(db, clk, nil)}
}

func (e *env) patch() *PatchSettings {
	return NewPatchSettings(adminrepo.NewSettingsRepository(e.db), adminsvc.NewTOTPStatus(e.db), e.rec, e.db)
}

func asUser(id int64) context.Context {
	return entity.WithPrincipal(context.Background(), entity.Principal{UserID: id, Username: "admin"})
}

func ptr[T any](v T) *T { return &v }

func TestSettingsRequire2FA(t *testing.T) {
	e := newEnv(t)
	repo := adminrepo.NewSettingsRepository(e.db)
	get := NewGetSettings(repo)
	patch := e.patch()
	noTOTP := admintest.InsertUser(t, e.db, "sin", false)
	withTOTP := admintest.InsertUser(t, e.db, "con", true)

	out, err := get.Execute(context.Background())
	if err != nil || out.Require2FA {
		t.Fatalf("require_2fa debe empezar en false: %+v err=%v", out, err)
	}

	// Sin TOTP propio no puede activarlo y no cambia nada.
	_, err = patch.Execute(asUser(noTOTP), dto.PatchSettingsInput{Require2FA: ptr(true)})
	if !errors.Is(err, service.ErrTOTPRequiredFirst) {
		t.Fatalf("err=%v", err)
	}
	if out, _ := get.Execute(context.Background()); out.Require2FA {
		t.Fatal("no debía guardarse")
	}

	// Sin Principal tampoco.
	if _, err := patch.Execute(context.Background(), dto.PatchSettingsInput{Require2FA: ptr(true)}); !errors.Is(err, entity.ErrUnauthenticated) {
		t.Fatalf("err=%v", err)
	}

	// Con TOTP propio sí, se guarda como "true" y deja auditoría.
	out, err = patch.Execute(asUser(withTOTP), dto.PatchSettingsInput{Require2FA: ptr(true)})
	if err != nil || !out.Require2FA {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	raw, enc, _ := repo.Get(context.Background(), "require_2fa")
	if string(raw) != "true" || enc {
		t.Fatalf("raw=%q enc=%v", raw, enc)
	}
	events, _ := NewListAudit(adminrepo.NewAuditRepository(e.db)).Execute(context.Background(), dto.ListAuditInput{})
	if len(events) != 1 || events[0].Action != "settings.update" || events[0].Target != "settings" ||
		string(events[0].Details) != `{"require_2fa":true}` || *events[0].ActorID != withTOTP {
		t.Fatalf("events=%+v", events)
	}

	// Desactivarlo no exige TOTP.
	out, err = patch.Execute(asUser(noTOTP), dto.PatchSettingsInput{Require2FA: ptr(false)})
	if err != nil || out.Require2FA {
		t.Fatalf("out=%+v err=%v", out, err)
	}

	// Un patch vacío no toca nada ni audita.
	before, _ := NewListAudit(adminrepo.NewAuditRepository(e.db)).Execute(context.Background(), dto.ListAuditInput{})
	if _, err := patch.Execute(asUser(noTOTP), dto.PatchSettingsInput{}); err != nil {
		t.Fatal(err)
	}
	after, _ := NewListAudit(adminrepo.NewAuditRepository(e.db)).Execute(context.Background(), dto.ListAuditInput{})
	if len(after) != len(before) {
		t.Fatal("un patch vacío no debe auditar")
	}
}

func TestListAuditValidation(t *testing.T) {
	e := newEnv(t)
	uc := NewListAudit(adminrepo.NewAuditRepository(e.db))
	for i := 0; i < 3; i++ {
		e.rec.Record(context.Background(), "a", "t", nil)
	}
	for _, in := range []dto.ListAuditInput{
		{Limit: ptr(0)}, {Limit: ptr(501)}, {Before: ptr(int64(0))}, {Before: ptr(int64(-1))},
	} {
		if _, err := uc.Execute(context.Background(), in); err == nil {
			t.Errorf("%+v debía fallar", in)
		} else if _, ok := errors.AsType[*validate.Error](err); !ok {
			t.Errorf("%+v: err=%v no es de validación", in, err)
		}
	}
	all, err := uc.Execute(context.Background(), dto.ListAuditInput{Limit: ptr(500)})
	if err != nil || len(all) != 3 {
		t.Fatalf("all=%v err=%v", all, err)
	}
	page, _ := uc.Execute(context.Background(), dto.ListAuditInput{Limit: ptr(1), Before: &all[0].ID})
	if len(page) != 1 || page[0].ID != all[1].ID {
		t.Fatalf("page=%+v", page)
	}
}

func TestPurgeAudit(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	e.rec.Record(ctx, "viejo", "t", nil) // hoy - 181 días al avanzar el reloj
	e.clk.Advance(100 * 24 * time.Hour)
	e.rec.Record(ctx, "medio", "t", nil) // 81 días de antigüedad al final
	e.clk.Advance(81 * 24 * time.Hour)
	e.rec.Record(ctx, "nuevo", "t", nil)

	n, err := NewPurgeAudit(adminrepo.NewAuditRepository(e.db), e.clk).Execute(ctx)
	if err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	left, _ := NewListAudit(adminrepo.NewAuditRepository(e.db)).Execute(ctx, dto.ListAuditInput{})
	if len(left) != 2 || left[0].Action != "nuevo" || left[1].Action != "medio" {
		t.Fatalf("left=%+v", left)
	}
}

func TestBackup(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	admintest.InsertUser(t, e.db, "ana", false)
	dir := filepath.Join(t.TempDir(), "backups")
	uc := NewBackup(adminsvc.NewBackupStore(e.db, dir), e.clk)
	files := func() []string {
		f, _ := filepath.Glob(filepath.Join(dir, "*"))
		return f
	}

	out, err := uc.Execute(ctx)
	if err != nil || !out.Created || out.Name != "muelle-2026-10-10.db" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	// La segunda llamada del mismo día no crea otro archivo.
	e.clk.Advance(5 * time.Hour)
	out, err = uc.Execute(ctx)
	if err != nil || out.Created || len(files()) != 1 {
		t.Fatalf("out=%+v err=%v files=%v", out, err, files())
	}

	// Conserva solo 7.
	for i := 0; i < 9; i++ {
		e.clk.Advance(24 * time.Hour)
		if _, err := uc.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	got := files()
	if len(got) != BackupsToKeep {
		t.Fatalf("files=%v", got)
	}
	if filepath.Base(got[len(got)-1]) != "muelle-2026-10-19.db" || filepath.Base(got[0]) != "muelle-2026-10-13.db" {
		t.Fatalf("files=%v", got)
	}
	// La copia es una base válida con los datos.
	cp, err := sqlite.Open(got[0])
	if err != nil {
		t.Fatal(err)
	}
	defer cp.Close()
	var n int
	if err := cp.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestBackupFailureLeavesNoFile(t *testing.T) {
	e := newEnv(t)
	// El "directorio" de backups es un archivo: crear la copia falla.
	blocker := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	uc := NewBackup(adminsvc.NewBackupStore(e.db, filepath.Join(blocker, "backups")), e.clk)
	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatal("debía fallar")
	}
}

func TestVerifyMasterKey(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	repo := adminrepo.NewSettingsRepository(e.db)
	keyA, _ := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	keyB, _ := crypto.New(bytes.Repeat([]byte{8}, crypto.KeySize))

	// Primer arranque: guarda el valor cifrado.
	if err := NewVerifyMasterKey(repo, keyA).Execute(ctx); err != nil {
		t.Fatal(err)
	}
	stored, enc, err := repo.Get(ctx, "master_key_check")
	if err != nil || !enc || len(stored) == 0 || bytes.Contains(stored, []byte("muelle")) {
		t.Fatalf("stored=%q enc=%v err=%v", stored, enc, err)
	}
	// Segundo arranque con la misma clave: pasa y no reescribe.
	if err := NewVerifyMasterKey(repo, keyA).Execute(ctx); err != nil {
		t.Fatal(err)
	}
	again, _, _ := repo.Get(ctx, "master_key_check")
	if !bytes.Equal(stored, again) {
		t.Fatal("el valor de control no debe reescribirse")
	}
	// Con otra clave: ErrWrongMasterKey.
	err = NewVerifyMasterKey(repo, keyB).Execute(ctx)
	if !errors.Is(err, service.ErrWrongMasterKey) || err.Error() != "clave maestra incorrecta" {
		t.Fatalf("err=%v", err)
	}
	// Un valor de control sin cifrar tampoco se acepta.
	repo.Set(ctx, "master_key_check", []byte("muelle"), false)
	if err := NewVerifyMasterKey(repo, keyA).Execute(ctx); !errors.Is(err, service.ErrWrongMasterKey) {
		t.Fatalf("err=%v", err)
	}
}
