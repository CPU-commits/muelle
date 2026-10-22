package usecases

import (
	"context"
	"errors"
	"fmt"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// LogPageBytes es lo más que devuelve una lectura del log (contrato §2.5).
const LogPageBytes = 64 << 10

// ListDeploys son los deploys de una app, del más nuevo al más viejo
// (contrato §2.3): los que guarda la retención.
type ListDeploys struct{ d Deps }

func NewListDeploys(d Deps) *ListDeploys { return &ListDeploys{d: d} }

func (u *ListDeploys) Execute(ctx context.Context, ref dto.AppRef) ([]entity.Deploy, error) {
	app, err := u.d.Apps.Get(ctx, ref.Project, ref.Stage, ref.App)
	if err != nil {
		return nil, err
	}
	return u.d.Deploys.List(ctx, app.ID)
}

// GetDeploy es un deploy de la app: uno de otra app es ErrNotFound (contrato §2.4).
type GetDeploy struct{ d Deps }

func NewGetDeploy(d Deps) *GetDeploy { return &GetDeploy{d: d} }

func (u *GetDeploy) Execute(ctx context.Context, ref dto.DeployRef) (entity.Deploy, error) {
	app, err := u.d.Apps.Get(ctx, ref.Project, ref.Stage, ref.App)
	if err != nil {
		return entity.Deploy{}, err
	}
	return u.d.Deploys.Get(ctx, app.ID, ref.ID)
}

// ReadLog es el polling del log (contrato §2.5): desde Offset, hasta 64 KiB
// en líneas completas. Un Offset negativo es uno inválido (422 offset).
type ReadLog struct{ d Deps }

func NewReadLog(d Deps) *ReadLog { return &ReadLog{d: d} }

func (u *ReadLog) Execute(ctx context.Context, in dto.ReadLog) (dto.LogPage, error) {
	app, err := u.d.Apps.Get(ctx, in.Project, in.Stage, in.App)
	if err != nil {
		return dto.LogPage{}, err
	}
	if in.Offset < 0 {
		// Primero el 404 de un deploy que no es de la app, después el 422.
		if _, err := u.d.Deploys.Get(ctx, app.ID, in.ID); err != nil {
			return dto.LogPage{}, err
		}
		return dto.LogPage{}, ErrBadOffset
	}
	c, err := u.d.Deploys.ReadLog(ctx, app.ID, in.ID, in.Offset, LogPageBytes)
	if errors.Is(err, service.ErrOffsetBeyond) {
		return dto.LogPage{}, validate.Errorf("offset", "pasa el largo del log")
	}
	if err != nil {
		return dto.LogPage{}, err
	}
	return dto.LogPage{Chunk: string(c.Chunk), NextOffset: in.Offset + len(c.Chunk), Length: c.Length,
		Status: c.Status, SupersededBy: c.SupersededBy}, nil
}

// ErrLogServiceNotFound es el not_found de get_app_logs por MCP cuando la app
// existe pero el servicio no (contrato §8.3 #7): su mensaje dice que en una
// app compose service es obligatorio. Envuelve repository.ErrNotFound.
var ErrLogServiceNotFound = fmt.Errorf("%w: el servicio no existe en la app", srepo.ErrNotFound)

// ErrBadOffset es el 422 de un offset que no es un entero ≥ 0 (contrato
// §2.5): lo usan el polling y el stream.
var ErrBadOffset = validate.Errorf("offset", "tiene que ser un entero ≥ 0")
