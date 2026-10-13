// Package admin es el módulo de administración: settings, listado y retención
// de la auditoría, backup diario y verificación de la clave maestra.
package admin

import (
	"context"
	"log/slog"
	"path/filepath"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/cmd/http/utils"
	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
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

// InitialSettings es la configuración inicial de install (MUELLE_INIT_*, ver
// dto.InitialSettings) que recibe ApplyStartupSettings.
type InitialSettings = dto.InitialSettings

type Deps struct {
	DB      *sqlite.DB
	Cipher  cipher.Cipher
	Clock   clock.Clock
	Audit   audit.Recorder
	Log     *slog.Logger
	DataDir string // backups en <DataDir>/backups
	// ProxyHook es ingress.Module: verifica un token de Cloudflare nuevo y
	// aplica los ajustes del proxy cuando cambian (service.ProxyHook).
	// Opcional: con nil, los ajustes solo se guardan.
	ProxyHook service.ProxyHook
}

type Module struct {
	handler *inhttp.Handler
	jobs    []cron.Job
	verify  *usecases.VerifyMasterKey
	startup *usecases.ApplyStartupSettings
}

func New(d Deps) *Module {
	switch {
	case d.DB == nil:
		panic("admin.New: falta Deps.DB")
	case d.Cipher == nil:
		panic("admin.New: falta Deps.Cipher")
	case d.Audit == nil:
		panic("admin.New: falta Deps.Audit")
	case d.DataDir == "":
		panic("admin.New: falta Deps.DataDir (los backups van en <DataDir>/backups)")
	}
	if d.Clock == nil {
		d.Clock = clock.System{}
	}
	if d.Log == nil {
		d.Log = slog.Default()
	}
	settings := adminrepo.NewSettingsRepository(d.DB)
	auditRepo := adminrepo.NewAuditRepository(d.DB)
	totp := adminsvc.NewTOTPStatus(d.DB)
	backups := adminsvc.NewBackupStore(d.DB, filepath.Join(d.DataDir, "backups"))

	getSettings := usecases.NewGetSettings(settings, d.Cipher)
	patchSettings := usecases.NewPatchSettings(settings, totp, d.Cipher, d.ProxyHook, d.Audit, d.DB)
	listAudit := usecases.NewListAudit(auditRepo)
	purge := usecases.NewPurgeAudit(auditRepo, d.Clock)
	backup := usecases.NewBackup(backups, d.Clock)

	return &Module{
		handler: inhttp.NewHandler(getSettings, patchSettings, listAudit),
		jobs:    incron.Jobs(purge, backup, d.Log),
		verify:  usecases.NewVerifyMasterKey(settings, d.Cipher, d.DB),
		startup: usecases.NewApplyStartupSettings(settings, d.Cipher, d.Audit, d.DB),
	}
}

// RegisterRoutes: GET/PATCH /api/v1/settings y GET /api/v1/audit, solo con
// sesión. Los campos de /settings están en el contrato de las fases 2 y 3
// (docs/superpowers/specs/2026-10-10-contrato-http-fases-2-3.md).
func (m *Module) RegisterRoutes(k *utils.Kit) { m.handler.Register(k) }

// Jobs: "retención de auditoría" y "backup diario", cada hora.
func (m *Module) Jobs() []cron.Job { return m.jobs }

// VerifyMasterKey: en el primer arranque guarda en settings (clave "master_key_check",
// encrypted=1, AAD "settings:master_key_check") un valor de control cifrado; en los
// siguientes lo descifra. Si falla devuelve service.ErrWrongMasterKey ("clave maestra incorrecta").
func (m *Module) VerifyMasterKey(ctx context.Context) error { return m.verify.Execute(ctx) }

// ApplyStartupSettings lo llama el contenedor en cada arranque, después de
// VerifyMasterKey: escribe panel_domain (el host de MUELLE_PANEL_ORIGIN si es
// un dominio, si no "") y, solo en el primer arranque de la base (la clave
// init_applied), guarda la configuración inicial de install (MUELLE_INIT_*).
func (m *Module) ApplyStartupSettings(ctx context.Context, panelDomain string, init InitialSettings) error {
	return m.startup.Execute(ctx, dto.StartupSettings{PanelDomain: panelDomain, Init: init})
}
