package usecases

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	outservice "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/service"
)

func (f *fixture) seed(regs repository.RegistryRepository) *SeedInitRegistry {
	if regs == nil {
		regs = f.regs
	}
	return NewSeedInitRegistry(f.db, outservice.NewDeploySettings(f.db), regs, f.clock, f.audit)
}

func (f *fixture) marked(t *testing.T) bool {
	t.Helper()
	var n int
	ctx := context.Background()
	if err := f.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key = ?`, entity.SettingInitRegistryApplied).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

const installPassword = "ghp_s3cr3t-de-install"

var installRegistry = &dto.InitRegistry{Host: "ghcr.io", Username: "bot", Password: installPassword}

// El primer arranque guarda el registry de install, cifrado y con su evento;
// la marca queda, y un registry que el admin borró no vuelve.
func TestSeedInitRegistryFirstBoot(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background() // el arranque no trae un principal
	if err := f.seed(nil).Execute(ctx, installRegistry); err != nil {
		t.Fatal(err)
	}
	if !f.marked(t) {
		t.Fatal("no quedó la marca")
	}
	var stored []byte
	if err := f.db.Conn(ctx).QueryRowContext(ctx, `SELECT password FROM registries WHERE host = 'ghcr.io'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored), installPassword) {
		t.Fatal("la contraseña quedó en claro")
	}
	creds, err := f.regs.Credentials(ctx, []string{"ghcr.io"})
	if err != nil || creds["ghcr.io"].Username != "bot" || creds["ghcr.io"].Password != installPassword {
		t.Fatalf("Credentials: %v, %v", creds, err)
	}
	events := auditEvents(t, f.db)
	if len(events) != 1 {
		t.Fatalf("eventos: %+v", events)
	}
	e := events[0]
	if e.ActorType != "system" || e.ActorID != nil || e.Action != "registry.create" || e.Target != "registry:ghcr.io" ||
		e.Details != `{"source":"install","username":"bot"}` {
		t.Fatalf("evento: %+v", e)
	}

	// Un segundo arranque con otra contraseña no cambia nada.
	again := &dto.InitRegistry{Host: "ghcr.io", Username: "bot2", Password: "otra-contraseña"}
	if err := f.seed(nil).Execute(ctx, again); err != nil {
		t.Fatal(err)
	}
	if creds, _ := f.regs.Credentials(ctx, []string{"ghcr.io"}); creds["ghcr.io"].Password != installPassword {
		t.Fatalf("el segundo arranque cambió la credencial: %v", creds)
	}

	// El admin la borra en el panel (se filtró): el próximo arranque, con el
	// mismo secreto montado, no la vuelve a crear.
	if err := NewDeleteRegistry(f.db, f.regs, f.audit).Execute(sessionCtx(), "ghcr.io"); err != nil {
		t.Fatal(err)
	}
	if err := f.seed(nil).Execute(ctx, installRegistry); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.regs.List(ctx); len(list) != 0 {
		t.Fatalf("volvió el registry borrado: %+v", list)
	}
}

// Un primer arranque sin registry también deja la marca: una reinstalación
// sobre la misma base no agrega credenciales.
func TestSeedInitRegistryWithoutRegistry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if err := f.seed(nil).Execute(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if !f.marked(t) {
		t.Fatal("sin registry, la marca igual queda")
	}
	if err := f.seed(nil).Execute(ctx, installRegistry); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.regs.List(ctx); len(list) != 0 {
		t.Fatalf("la reinstalación creó %+v", list)
	}
	if events := auditEvents(t, f.db); len(events) != 0 {
		t.Fatalf("eventos: %+v", events)
	}
}

// Si ese host ya tiene una credencial (una base restaurada sin la marca), no
// la toca, y la marca queda.
func TestSeedInitRegistryExistingHost(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := NewCreateRegistry(f.db, f.regs, f.clock, f.audit).Execute(sessionCtx(), dto.RegistryInput{Host: "ghcr.io", Username: "admin", Password: "la-del-admin"}); err != nil {
		t.Fatal(err)
	}
	if err := f.seed(nil).Execute(ctx, installRegistry); err != nil {
		t.Fatal(err)
	}
	if creds, _ := f.regs.Credentials(ctx, []string{"ghcr.io"}); creds["ghcr.io"].Password != "la-del-admin" {
		t.Fatalf("pisó la credencial: %v", creds)
	}
	if !f.marked(t) {
		t.Fatal("no quedó la marca")
	}
}

// failingRegistries falla al crear, con un error de la base.
type failingRegistries struct{ repository.RegistryRepository }

var errDisk = errors.New("disk I/O error")

func (failingRegistries) Create(ctx context.Context, r *entity.Registry) error { return errDisk }

// Si guardar el registry falla, la marca se deshace (el próximo arranque
// reintenta) y el error no trae la contraseña.
func TestSeedInitRegistryFailureRollsBack(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	err := f.seed(failingRegistries{f.regs}).Execute(ctx, installRegistry)
	if !errors.Is(err, errDisk) {
		t.Fatalf("Execute: %v", err)
	}
	if strings.Contains(err.Error(), installPassword) {
		t.Fatalf("el error trae la contraseña: %v", err)
	}
	if f.marked(t) {
		t.Fatal("la marca quedó aunque falló")
	}
	if err := f.seed(nil).Execute(ctx, installRegistry); err != nil {
		t.Fatal(err)
	}
	if list, _ := f.regs.List(ctx); len(list) != 1 {
		t.Fatalf("el reintento no lo creó: %+v", list)
	}
}

// Un registry que no cumple las reglas no se guarda (settings.Load ya lo
// validó; esto es la última barrera), y el error no trae la contraseña.
func TestSeedInitRegistryValidates(t *testing.T) {
	f := newFixture(t)
	err := f.seed(nil).Execute(context.Background(), &dto.InitRegistry{Host: "index.docker.io", Username: "bot", Password: installPassword})
	if msg := validationMessage(t, err); strings.Contains(msg, installPassword) {
		t.Fatalf("el error trae la contraseña: %s", msg)
	}
	if f.marked(t) {
		t.Fatal("la marca quedó con un registry inválido")
	}
}
