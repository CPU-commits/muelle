package admin

import (
	"context"
	"encoding/json"
	"errors"

	adminrepo "github.com/CPU-commits/muelle/src/internal/admin/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/package/audit"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
)

// ResetResult es lo que hizo ResetPanelAllowedCIDRs.
type ResetResult struct {
	Changed  bool     // false: la lista ya estaba vacía y no se escribió nada
	Previous []string // los rangos que tenía; nil si el valor guardado no se entendía
}

// ResetPanelAllowedCIDRs deja panel_allowed_cidrs en [] (muelle settings
// reset-panel-ips, contrato §4.1 y §5.4), en una transacción con su evento
// settings.update (actor system: el ctx no trae Principal). No necesita la
// clave maestra: la lista no va cifrada. Un valor que no es una lista JSON
// también se reemplaza; uno vacío ("", null o []) es sin restricción, como
// lo lee entity.DecodeProxySettings, y no se toca.
//
// Vive acá y no en app/usecases solo por el alcance de la Fase 5: ver el plan, T3 Step 4.
func ResetPanelAllowedCIDRs(ctx context.Context, db *sqlite.DB, rec audit.Recorder) (ResetResult, error) {
	repo := adminrepo.NewSettingsRepository(db)
	var res ResetResult
	err := db.InTx(ctx, func(ctx context.Context) error {
		v, _, err := repo.Get(ctx, shared.SettingPanelAllowedCIDRs)
		switch {
		case errors.Is(err, sharedrepo.ErrNotFound):
			return nil
		case err != nil:
			return err
		}
		if len(v) > 0 {
			var list []string
			if err := json.Unmarshal(v, &list); err != nil {
				res.Changed = true
			} else if len(list) > 0 {
				res.Changed, res.Previous = true, list
			}
		}
		if !res.Changed {
			return nil
		}
		if err := repo.Set(ctx, shared.SettingPanelAllowedCIDRs, []byte("[]"), false); err != nil {
			return err
		}
		rec.Record(ctx, "settings.update", "settings", map[string]any{
			shared.SettingPanelAllowedCIDRs: []string{},
			"via":                           "cli",
		})
		return nil
	})
	if err != nil {
		return ResetResult{}, err
	}
	return res, nil
}
