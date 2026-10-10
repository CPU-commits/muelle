package usecases

import (
	"context"
	"slices"
	"sync"

	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
)

// BackupsToKeep es cuántas copias diarias se conservan (§12 del spec).
const BackupsToKeep = 7

type Backup struct {
	store service.BackupStore
	clock clock.Clock
	mu    sync.Mutex // una sola ejecución a la vez: el job y un llamado manual no se pisan
}

func NewBackup(store service.BackupStore, c clock.Clock) *Backup {
	return &Backup{store: store, clock: c}
}

// Execute crea la copia del día si todavía no existe (el job corre cada hora)
// y conserva solo las últimas 7. La fecha es la UTC del reloj inyectado.
func (u *Backup) Execute(ctx context.Context) (dto.BackupOutput, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	name := entity.BackupName(u.clock.Now())
	out := dto.BackupOutput{Name: name}

	names, err := u.store.List(ctx)
	if err != nil {
		return out, err
	}
	if !slices.Contains(names, name) {
		if err := u.store.Create(ctx, name); err != nil {
			return out, err
		}
		out.Created = true
		names = append(names, name)
		slices.Sort(names)
	}
	for len(names) > BackupsToKeep {
		if err := u.store.Remove(ctx, names[0]); err != nil {
			return out, err
		}
		names = names[1:]
		out.Removed++
	}
	return out, nil
}
