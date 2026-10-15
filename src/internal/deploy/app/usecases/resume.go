package usecases

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// Resume cierra un deploy que quedó running de un arranque anterior
// (contrato §1.2), según lo que llegó a Swarm, con la línea "muelle se
// reinició durante el deploy" antes de su resultado:
//   - sin requested_images (se cortó antes del paso 2), o si ningún servicio
//     esperado lleva su muelle.deploy: failed, Swarm no cambió;
//   - si un servicio esperado tiene un rollback (o una pausa) de Swarm de
//     este deploy: failed como en el paso 6. Un rollback de Swarm devuelve
//     las etiquetas anteriores, así que esto se mira antes que las etiquetas;
//   - si solo algunos llevan su muelle.deploy: failed, el CLI se cortó a mitad;
//   - si todos: el paso 6 (con appliedAt = started_at y el margen contado
//     desde ahora: no se sabe cuándo volvió el CLI cortado), el 7 y el cierre.
//
// Con Docker caído queda running y vuelve a mirar cada RetryEvery. Devuelve
// como Execute.
func (u *RunDeploy) Resume(ctx context.Context, d entity.Deploy) error {
	r := u.newRun(d)
	return r.end(ctx, r.resume(ctx))
}

func (r *run) resume(ctx context.Context) error {
	u, d := r.u, r.d
	if d.StartedAt == nil {
		return failf("%s", reasonNeverApplied)
	}
	app, err := u.Apps.GetByID(ctx, d.AppID)
	if err != nil {
		return err
	}
	r.app = app
	_, secrets, err := u.Env.ResolveEnv(ctx, app.ID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return failf("%s", reasonEnv)
	}
	r.setMasker(secrets)
	r.line("%s", lineRestarted)
	if len(d.RequestedImages) == 0 {
		return failf("%s", reasonNeverApplied)
	}
	svcs := slices.Sorted(maps.Keys(d.RequestedImages))
	expected := make([]string, len(svcs))
	for i, svc := range svcs {
		expected[i] = naming.Service(app.Project, app.Stage, app.Slug, svc)
	}

	var services []service.SwarmService
	for announced := false; ; {
		services, err = u.Cluster.AppServices(ctx, app.Project, app.Stage, app.Slug)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !announced {
			r.line("%s; se vuelve a intentar cada %d s", dockerMessage(err), int(u.Timing.RetryEvery.Seconds()))
			announced = true
		}
		if err := sleepCtx(ctx, u.Timing.RetryEvery); err != nil {
			return err
		}
	}

	now := u.Clock.Now()
	if v := service.Evaluate(expected, services, *d.StartedAt, now, now, u.Timing.Settle); v.Failed {
		r.taskDetails(ctx, v.Service, v.ServiceID, *d.StartedAt, v.Reason, true)
		return failf("%s: %s", v.Service, v.Reason)
	}
	switch service.AppliedState(expected, services, d.ID) {
	case service.AppliedNone:
		return failf("%s", reasonNeverApplied)
	case service.AppliedPartial:
		return failf("%s", reasonPartialApplied)
	}
	if err := r.converge(ctx, expected, *d.StartedAt, u.Clock.Now()); err != nil {
		return err
	}
	return r.routes(ctx)
}

// sleepCtx espera dur o hasta que ctx se cancela (devuelve ctx.Err()).
func sleepCtx(ctx context.Context, dur time.Duration) error {
	t := time.NewTimer(dur)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
