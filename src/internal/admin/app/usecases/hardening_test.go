package usecases

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/admintest"
	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	adminrepo "github.com/CPU-commits/muelle/src/internal/admin/infra/out/repository/sqlite"
	adminsvc "github.com/CPU-commits/muelle/src/internal/admin/infra/out/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
)

func TestPatchSettingsRejectsTokens(t *testing.T) {
	e := newEnv(t)
	uid := admintest.InsertUser(t, e.db, "con", true)
	ctx := entity.WithPrincipal(context.Background(), entity.Principal{
		UserID: uid, Token: &entity.TokenScope{ID: 1, ProjectSlug: "p"},
	})
	for _, in := range []dto.PatchSettingsInput{{Require2FA: ptr(true)}, {Require2FA: ptr(false)}, {}} {
		if _, err := e.patch().Execute(ctx, in); !errors.Is(err, entity.ErrForbidden) {
			t.Fatalf("err=%v", err)
		}
	}
	if out, _ := NewGetSettings(adminrepo.NewSettingsRepository(e.db), testCipher()).Execute(ctx, dto.GetSettingsInput{}); out.Require2FA {
		t.Fatal("un token no debe poder cambiar ajustes")
	}
}

func TestBackupIgnoresStrayFilesInRetention(t *testing.T) {
	e := newEnv(t)
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(dir, "muelle-manual-antes-de-migrar.db")
	if err := os.WriteFile(stray, []byte("a mano"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := NewBackup(adminsvc.NewBackupStore(e.db, dir), e.clk)
	for range 10 {
		if _, err := uc.Execute(ctx); err != nil {
			t.Fatal(err)
		}
		e.clk.Advance(24 * time.Hour)
	}
	daily, _ := filepath.Glob(filepath.Join(dir, "muelle-2026-*.db"))
	if len(daily) != BackupsToKeep {
		t.Fatalf("diarias=%v, esperaba %d", daily, BackupsToKeep)
	}
	if b, err := os.ReadFile(stray); err != nil || string(b) != "a mano" {
		t.Fatalf("el archivo ajeno se tocó: %q err=%v", b, err)
	}
}

func TestBackupNameUsesUTCNearMidnight(t *testing.T) {
	e := newEnv(t)
	tz := time.FixedZone("UTC-5", -5*3600)
	e.clk.Set(time.Date(2026, 10, 10, 20, 30, 0, 0, tz)) // ya es 11 en UTC
	uc := NewBackup(adminsvc.NewBackupStore(e.db, t.TempDir()), e.clk)
	out, err := uc.Execute(context.Background())
	if err != nil || out.Name != "muelle-2026-10-11.db" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

func TestBackupConcurrentExecuteCreatesOnce(t *testing.T) {
	e := newEnv(t)
	uc := NewBackup(adminsvc.NewBackupStore(e.db, filepath.Join(t.TempDir(), "b")), e.clk)
	var wg sync.WaitGroup
	var mu sync.Mutex
	created := 0
	for range 4 {
		wg.Go(func() {
			out, err := uc.Execute(context.Background())
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if out.Created {
				created++
			}
		})
	}
	wg.Wait()
	if created != 1 {
		t.Fatalf("created=%d", created)
	}
}

func TestVerifyMasterKeyConcurrentFirstStart(t *testing.T) {
	e := newEnv(t)
	repo := adminrepo.NewSettingsRepository(e.db)
	keyA, _ := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	keyB, _ := crypto.New(bytes.Repeat([]byte{8}, crypto.KeySize))

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, k := range []*crypto.Cipher{keyA, keyB} {
		wg.Go(func() { errs[i] = NewVerifyMasterKey(repo, k, e.db).Execute(context.Background()) })
	}
	wg.Wait()

	// Exactamente una clave pasó el primer arranque; la otra fue rechazada.
	passed := 0
	for _, err := range errs {
		switch {
		case err == nil:
			passed++
		case !errors.Is(err, service.ErrWrongMasterKey):
			t.Fatalf("err inesperado: %v", err)
		}
	}
	if passed != 1 {
		t.Fatalf("pasaron %d claves: %v", passed, errs)
	}
	// Y después solo esa clave sigue pasando.
	okA := NewVerifyMasterKey(repo, keyA, e.db).Execute(context.Background()) == nil
	okB := NewVerifyMasterKey(repo, keyB, e.db).Execute(context.Background()) == nil
	if okA == okB {
		t.Fatalf("A=%v B=%v: debe pasar exactamente una", okA, okB)
	}
}
