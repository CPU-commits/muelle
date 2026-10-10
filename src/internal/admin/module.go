// Package admin es el módulo de administración: settings, listado y retención
// de la auditoría, backup diario y verificación de la clave maestra.
package admin

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/admin/app/usecases"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	incron "github.com/CPU-commits/muelle/src/internal/admin/infra/in/cron"
	inhttp "github.com/CPU-commits/muelle/src/internal/admin/infra/in/http"
	adminrepo "github.com/CPU-commits/muelle/src/internal/admin/infra/out/repository/sqlite"
	adminsvc "github.com/CPU-commits/muelle/src/internal/admin/infra/out/service"
	"github.com/CPU-commits/muelle/src/package/audit"
	"github.com/CPU-commits/muelle/src/package/cipher"
	"github.com/CPU-commits/muelle/src/package/clock"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// ErrWrongMasterKey es el error de VerifyMasterKey con otra clave maestra.
var ErrWrongMasterKey = service.ErrWrongMasterKey

type Deps struct {
	DB      *sqlite.DB
	Cipher  cipher.Cipher
	Clock   clock.Clock
	Audit   audit.Recorder
	Log     *slog.Logger
	DataDir string // backups en <DataDir>/backups
}

type Module struct {
	handler *inhttp.Handler
	jobs    []cron.Job
	verify  *usecases.VerifyMasterKey
}

func New(d Deps) *Module {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	settings := adminrepo.NewSettingsRepository(d.DB)
	auditRepo := adminrepo.NewAuditRepository(d.DB)
	totp := adminsvc.NewTOTPStatus(d.DB)
	backups := adminsvc.NewBackupStore(d.DB, filepath.Join(d.DataDir, "backups"))

	getSettings := usecases.NewGetSettings(settings)
	patchSettings := usecases.NewPatchSettings(settings, totp, d.Audit, d.DB)
	listAudit := usecases.NewListAudit(auditRepo)
	purge := usecases.NewPurgeAudit(auditRepo, d.Clock)
	backup := usecases.NewBackup(backups, d.Clock)

	return &Module{
		handler: inhttp.NewHandler(getSettings, patchSettings, listAudit),
		jobs:    incron.Jobs(purge, backup, d.Log),
		verify:  usecases.NewVerifyMasterKey(settings, d.Cipher),
	}
}

// RegisterRoutes: GET/PATCH /api/v1/settings y GET /api/v1/audit, solo con sesión.
func (m *Module) RegisterRoutes(k *utils.Kit) { m.handler.Register(k) }

// Jobs: "retención de auditoría" y "backup diario", cada hora.
func (m *Module) Jobs() []cron.Job { return m.jobs }

// VerifyMasterKey: en el primer arranque guarda en settings (clave "master_key_check",
// encrypted=1, AAD "settings:master_key_check") un valor de control cifrado; en los
// siguientes lo descifra. Si falla devuelve service.ErrWrongMasterKey ("clave maestra incorrecta").
func (m *Module) VerifyMasterKey(ctx context.Context) error { return m.verify.Execute(ctx) }
