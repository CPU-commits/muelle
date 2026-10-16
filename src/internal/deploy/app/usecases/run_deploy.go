package usecases

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// failedTaskLines son las líneas del log de la última tarea fallida que van
// al log del deploy (§7 paso 6).
const failedTaskLines = 50

// Los motivos fijos con que cierra un deploy.
const (
	reasonAppRemoved     = "la app se borró durante el deploy"
	reasonQueuedRemoved  = "la app se borró"
	reasonInternal       = "error interno de muelle"
	reasonEnv            = "no se pudieron leer las variables de la app"
	reasonTimeout        = "timeout esperando convergencia"
	reasonNeverApplied   = "muelle se reinició antes de aplicar el deploy; Swarm no cambió"
	reasonPartialApplied = "muelle se reinició a mitad de docker stack deploy"
	lineRestarted        = "muelle se reinició durante el deploy"
)

// RunDeploy corre un deploy ya reclamado (running) hasta cerrarlo (§7 pasos
// 2 a 8), y cierra los que quedaron running de un arranque anterior (Resume,
// contrato §1.2). Gate es el despachador (service.Queue): se fija después de
// armarlo.
type RunDeploy struct {
	Apps       service.Apps
	Deploys    repository.DeployRepository
	Registries repository.RegistryRepository
	Env        service.EnvResolver
	Domains    service.DomainTargets
	Ingress    service.Ingress
	Images     service.Images
	Stacks     service.Stacks
	Cluster    service.Cluster
	Manager    service.ManagerNode
	Gate       service.Queue
	Log        *LogWriter
	Hub        *Hub
	Tx         srepo.Transactor
	Clock      clock.Clock
	Timing     Timing
}

// Execute corre los pasos 2 a 8 de d, que ya está running (con StartedAt).
// Si ctx se cancela por el apagado, vuelve con ctx.Err() sin cerrar el
// deploy: queda running para la reconciliación (contrato §1.2). Si se cancela
// con la causa service.ErrAppRemoved, lo cierra failed con "la app se borró
// durante el deploy". Cualquier otro error es interno (la base): intenta
// cerrarlo failed con "error interno de muelle" y lo devuelve.
//
// Los errores que devuelve pueden traer valores de variables (un error de la
// base que envuelve uno de render): nunca van crudos a slog (decisión 18).
func (u *RunDeploy) Execute(ctx context.Context, d entity.Deploy) error {
	r := u.newRun(d)
	return r.end(ctx, r.steps(ctx))
}

// run es un deploy en curso: la app, el enmascarado y su log.
type run struct {
	u     *RunDeploy
	d     entity.Deploy
	app   service.App
	m     service.Masker
	log   *DeployLog
	creds map[string]entity.Registry // host → credencial (paso 2), para el paso 5
	// logErr es el primer error al escribir una línea: el deploy sigue (la
	// línea es solo informativa) y Execute lo devuelve al final.
	logErr error
}

func (u *RunDeploy) newRun(d entity.Deploy) *run {
	r := &run{u: u, d: d}
	r.setMasker(nil)
	return r
}

func (r *run) setMasker(secrets []string) {
	r.m = service.NewMasker(secrets)
	r.log = r.u.Log.For(r.d.ID, r.m)
}

// line escribe una línea (enmascarada); un error queda en logErr.
func (r *run) line(format string, args ...any) {
	if err := r.log.Line(format, args...); err != nil && r.logErr == nil {
		r.logErr = err
	}
}

// lines escribe varias líneas tal cual, en una sola escritura.
func (r *run) lines(texts []string) {
	if err := r.log.Lines(texts); err != nil && r.logErr == nil {
		r.logErr = err
	}
}

// failure es un deploy que falla por un motivo que va al log y a
// deploys.error (sin enmascarar todavía: close lo enmascara).
type failure struct{ msg string }

func (f *failure) Error() string { return f.msg }

func failf(format string, args ...any) error { return &failure{msg: fmt.Sprintf(format, args...)} }

// end cierra el deploy según cómo terminaron sus pasos: nil es succeeded;
// una failure, failed con su motivo; ctx cancelado por RemoveApp, failed; por
// el apagado, queda running; cualquier otro error, failed con un motivo fijo
// (el error puede traer valores, no va a deploys.error).
func (r *run) end(ctx context.Context, err error) error {
	switch {
	case err == nil:
		return errors.Join(r.close(ctx, entity.StatusSucceeded, ""), r.logErr)
	case isFailure(err):
		f, _ := errors.AsType[*failure](err)
		return errors.Join(r.close(ctx, entity.StatusFailed, f.msg), r.logErr)
	case ctx.Err() != nil && errors.Is(context.Cause(ctx), service.ErrAppRemoved):
		return errors.Join(r.close(ctx, entity.StatusFailed, reasonAppRemoved), r.logErr)
	case ctx.Err() != nil:
		return ctx.Err()
	}
	return errors.Join(err, r.close(ctx, entity.StatusFailed, reasonInternal))
}

func isFailure(err error) bool {
	_, ok := errors.AsType[*failure](err)
	return ok
}

// close cierra el deploy (paso 8 o una falla): en una transacción, la línea
// de cierre (en la reserva del log), el estado con su error enmascarado y,
// si fue succeeded, last_deployed_at hasta started_at; después del commit
// avisa a los streams. Escribe aunque ctx esté cancelado: el resultado ya se
// decidió. La retención la aplica el despachador.
func (r *run) close(ctx context.Context, status entity.Status, msg string) error {
	ctx = context.WithoutCancel(ctx)
	now := r.u.Clock.Now()
	var text string
	var errMsg *string
	if status == entity.StatusSucceeded {
		text = fmt.Sprintf("succeeded en %d s", r.elapsed(now))
	} else {
		text = "failed: " + msg
		e := service.ErrorLine(r.m, msg)
		errMsg = &e
	}
	err := r.u.Tx.InTx(ctx, func(ctx context.Context) error {
		lines, marker := r.log.Format(text)
		if err := r.u.Deploys.AppendLog(ctx, r.d.ID, lines, true, marker); err != nil {
			return err
		}
		if err := r.u.Deploys.Finish(ctx, r.d.ID, status, errMsg, now); err != nil {
			return err
		}
		if status == entity.StatusSucceeded {
			return r.u.Apps.MarkDeployed(ctx, r.d.AppID, *r.d.StartedAt)
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.u.Hub.Notify(r.d.ID)
	return nil
}

// elapsed son los segundos desde started_at, redondeados.
func (r *run) elapsed(now time.Time) int64 {
	if r.d.StartedAt == nil {
		return 0
	}
	return int64(math.Round(now.Sub(*r.d.StartedAt).Seconds()))
}

// steps son los pasos 2 a 7 (§7, contrato §2.11). Devuelve nil si el deploy
// convergió (end lo cierra succeeded), una failure, o un error interno o del ctx.
func (r *run) steps(ctx context.Context) error {
	u, d := r.u, r.d
	if d.StartedAt == nil {
		return errors.New("deploy: el deploy no está reclamado (sin started_at)")
	}
	app, err := u.Apps.GetByID(ctx, d.AppID)
	if err != nil {
		return err
	}
	r.app = app
	r.line("deploy #%d de %s/%s/%s (%s · %s)", d.ID, app.Project, app.Stage, app.Slug, d.Trigger, d.ActorName)
	vars, secrets, err := u.Env.ResolveEnv(ctx, app.ID)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return failf("%s", reasonEnv)
	}
	r.setMasker(secrets)

	// Paso 2: la configuración y las imágenes.
	r.line("resolviendo imágenes")
	cfg, err := service.ReadConfig(ctx, app, vars)
	if err != nil {
		if re, ok := errors.AsType[*service.RenderError](err); ok {
			return failf("%s", re.Error())
		}
		return err
	}
	for _, svc := range slices.Sorted(maps.Keys(d.Images)) {
		if _, ok := cfg.Images[svc]; !ok {
			r.line("  %s: ya no está en la configuración de la app; se omite (%s)", svc, d.Images[svc])
		}
	}
	current, err := u.Deploys.Current(ctx, app.ID)
	if err != nil {
		return err
	}
	requested := service.ChooseImages(cfg.Images, d.Images, current)
	resolved, err := r.resolve(ctx, requested)
	if err != nil {
		return err
	}
	if err := u.Deploys.SetImages(ctx, d.ID, requested, cfg.Images, resolved); err != nil {
		return err
	}

	// Paso 3: el nodo del manager (contrato §1.2: lo reintenta cada deploy) y
	// el render.
	needNode := cfg.HasVolumes && app.StorageNodeID == ""
	node, err := r.managerNode(ctx, needNode)
	if err != nil {
		return err
	}
	storage := app.StorageNodeID
	if needNode {
		if storage, err = u.Apps.SetStorageNode(ctx, app.ID, node); err != nil {
			return err
		}
	}
	domains, err := u.Domains.ByApp(ctx, app.ID)
	if err != nil {
		return err
	}
	rendered, err := service.Render(cfg, service.RenderInput{
		App: app, DeployID: d.ID, Images: resolved, StorageNodeID: storage, Domains: domains,
	})
	if err != nil {
		return failf("%s", err.Error())
	}
	if n := len(rendered.Services); n == 1 {
		r.line("render: 1 servicio")
	} else {
		r.line("render: %d servicios", n)
	}

	// Paso 4: la red del stage.
	network := naming.Network(app.Project, app.Stage)
	nctx, cancel := context.WithTimeout(ctx, u.Timing.NetworkTimeout)
	err = u.Ingress.EnsureStageNetwork(nctx, app.Project, app.Stage)
	cancel()
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return failf("no se pudo asegurar la red %s: %s", network, dockerMessage(err))
	}
	r.line("red %s lista", network)

	// Paso 5: docker stack deploy, salvo que la app se empezó a borrar
	// mientras corrían los pasos 2 a 4 (decisión 8).
	if !u.Gate.Accepting(app.ID) {
		return failf("%s", reasonAppRemoved)
	}
	stack := naming.Stack(app.Project, app.Stage, app.Slug)
	var creds []service.Credential
	for _, host := range rendered.Hosts {
		if reg, ok := r.creds[host]; ok {
			creds = append(creds, service.Credential{Host: reg.Host, Username: reg.Username, Password: reg.Password})
		}
	}
	appliedAt := u.Clock.Now() // el umbral de un UpdateStatus de este deploy
	r.line("docker stack deploy %s", stack)
	sctx, cancel := context.WithTimeout(ctx, u.Timing.StackTimeout)
	out, err := u.Stacks.Deploy(sctx, stack, rendered.Compose, creds)
	timedOut := errors.Is(sctx.Err(), context.DeadlineExceeded)
	cancel()
	settleFrom := u.Clock.Now() // desde acá corre el margen (decisión 6)
	// La salida entera se enmascara antes de partirla: un secreto de varias
	// líneas cortas solo se reconoce entero, con sus \n.
	r.lines(service.SplitOutput(r.m.Mask(out)))
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case timedOut:
			return failf("docker stack deploy no terminó en %d s", int(u.Timing.StackTimeout.Seconds()))
		}
		return failf("docker stack deploy falló: %s", dockerMessage(err))
	}

	// Paso 6: la convergencia.
	expected := make([]string, len(rendered.Services))
	for i, svc := range rendered.Services {
		expected[i] = naming.Service(app.Project, app.Stage, app.Slug, svc)
	}
	if err := r.converge(ctx, expected, appliedAt, settleFrom); err != nil {
		return err
	}
	// Paso 7: las rutas de Caddy.
	return r.routes(ctx)
}

// resolve consulta el registry por cada ref pedida, en orden de servicio,
// con la credencial de su host exacto (contrato §2.10), también si ya trae un
// digest (decisión 1). Devuelve las refs resueltas.
func (r *run) resolve(ctx context.Context, requested map[string]string) (map[string]string, error) {
	svcs := slices.Sorted(maps.Keys(requested))
	hostOf := make(map[string]string, len(svcs))
	var hosts []string
	for _, svc := range svcs {
		h, err := service.ImageHost(requested[svc])
		if err != nil {
			return nil, failf("la imagen del servicio %s no es una ref válida: %s", svc, requested[svc])
		}
		hostOf[svc] = h
		if !slices.Contains(hosts, h) {
			hosts = append(hosts, h)
		}
	}
	creds, err := r.u.Registries.Credentials(ctx, hosts)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, failf("no se pudieron leer las credenciales de los registries: %v", err)
	}
	r.creds = creds
	resolved := make(map[string]string, len(svcs))
	for _, svc := range svcs {
		ref, host := requested[svc], hostOf[svc]
		var cred *service.Credential
		if reg, ok := creds[host]; ok {
			cred = &service.Credential{Host: reg.Host, Username: reg.Username, Password: reg.Password}
		}
		rctx, cancel := context.WithTimeout(ctx, r.u.Timing.ResolveTimeout)
		digest, err := r.u.Images.Digest(rctx, ref, cred)
		timedOut := errors.Is(rctx.Err(), context.DeadlineExceeded)
		cancel()
		if err != nil {
			switch {
			case ctx.Err() != nil:
				return nil, ctx.Err()
			case errors.Is(err, service.ErrImageNotFound):
				return nil, failf("la imagen %s no existe en el registry", ref)
			case errors.Is(err, service.ErrImageDenied):
				return nil, failf("sin acceso a %s: revisa la credencial de %s en Ajustes → Registries", ref, host)
			case errors.Is(err, shared.ErrDockerUnavailable):
				return nil, failf("Docker no responde")
			case timedOut:
				return nil, failf("el registry no respondió en %d s al resolver %s", int(r.u.Timing.ResolveTimeout.Seconds()), ref)
			}
			return nil, failf("no se pudo resolver %s: %v", ref, err)
		}
		if pinned := service.PinnedDigest(ref); pinned != "" && pinned != digest {
			return nil, failf("el registry devolvió otro digest para %s", ref)
		}
		full, err := service.ResolvedRef(ref, digest)
		if err != nil {
			return nil, failf("no se pudo resolver %s: %v", ref, err)
		}
		resolved[svc] = full
		r.line("  %s: %s → %s", svc, ref, shortDigest(digest))
	}
	return resolved, nil
}

// shortDigest es el digest con sus primeros 12 caracteres (contrato §2.11).
func shortDigest(d string) string {
	const n = len("sha256:") + 12
	if len(d) <= n {
		return d
	}
	return d[:n] + "…"
}

// managerNode es manager_node_id; si falta, guarda el del nodo local
// (contrato §1.2: Run lo reintenta en cada deploy). Con Docker caído es una
// advertencia, salvo que el deploy lo necesite para los volúmenes (need).
func (r *run) managerNode(ctx context.Context, need bool) (string, error) {
	id, err := r.u.Manager.Get(ctx)
	if err != nil || id != "" {
		return id, err
	}
	local, err := r.u.Cluster.LocalNodeID(ctx)
	if err != nil {
		switch {
		case ctx.Err() != nil:
			return "", ctx.Err()
		case need:
			return "", failf("no se pudo leer el nodo del manager, donde quedan los volúmenes: %s", dockerMessage(err))
		}
		r.line("advertencia: no se pudo guardar el nodo del manager: %s", dockerMessage(err))
		return "", nil
	}
	return r.u.Manager.SetIfMissing(ctx, local)
}

// dockerMessage es el motivo de un error de Docker para el log: "Docker no
// responde" si está caído; si no, el error (el log lo enmascara).
func dockerMessage(err error) string {
	if errors.Is(err, shared.ErrDockerUnavailable) {
		return "Docker no responde"
	}
	return err.Error()
}

// converge espera la convergencia de los servicios esperados (§7 paso 6),
// con el deploy_timeout_s de la app contado desde ahora: una línea cada vez
// que cambia el progreso de un servicio; con Docker caído, una línea y se
// sigue intentando hasta el timeout.
func (r *run) converge(ctx context.Context, expected []string, appliedAt, settleFrom time.Time) error {
	u, app := r.u, r.app
	r.line("esperando convergencia (hasta %d s)", app.DeployTimeoutS)
	deadline := u.Clock.Now().Add(time.Duration(app.DeployTimeoutS) * time.Second)
	progress := map[string]string{}
	var last service.Verdict
	var lastServices []service.SwarmService
	down := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		services, err := u.Cluster.AppServices(ctx, app.Project, app.Stage, app.Slug)
		switch {
		case err != nil && ctx.Err() != nil:
			return ctx.Err()
		case err != nil:
			if !down {
				r.line("%s; reintentando", dockerMessage(err))
				down = true
			}
		default:
			down = false
			v := service.Evaluate(expected, services, appliedAt, settleFrom, u.Clock.Now(), u.Timing.Settle)
			last, lastServices = v, services
			for _, svc := range slices.Sorted(maps.Keys(v.Progress)) {
				if p := v.Progress[svc]; progress[svc] != p {
					progress[svc] = p
					r.line("%s: %s", svc, p)
				}
			}
			if v.Failed {
				r.taskDetails(ctx, v.Service, v.ServiceID, appliedAt, v.Reason, true)
				return failf("%s: %s", v.Service, v.Reason)
			}
			if v.Done {
				return nil
			}
		}
		now := u.Clock.Now()
		if !now.Before(deadline) {
			return r.timeout(ctx, last, lastServices, appliedAt)
		}
		t := time.NewTimer(min(u.Timing.Poll, deadline.Sub(now)))
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// timeout dice el estado de cada servicio que no llegó y el Err de su última
// tarea fallida, y falla (§7 paso 6).
func (r *run) timeout(ctx context.Context, v service.Verdict, services []service.SwarmService, appliedAt time.Time) error {
	app := r.app
	byName := make(map[string]service.SwarmService, len(services))
	for _, s := range services {
		byName[s.Name] = s
	}
	for _, short := range v.Pending {
		r.line("%s: %s", short, v.Progress[short])
		if s, ok := byName[naming.Service(app.Project, app.Stage, app.Slug, short)]; ok {
			r.taskDetails(ctx, short, s.ID, appliedAt, "", false)
		}
	}
	return failf("%s", reasonTimeout)
}

// taskDetails escribe el Err de la última tarea fallida del servicio y, con
// logs, sus últimas 50 líneas de log ("    | …"), con los pedazos de una
// línea larga unidos antes de enmascarar (contrato §2.5).
//
// La tarea es la que nombra Swarm en el Message de la actualización (hint:
// "…early termination of task <id>"); si no nombra ninguna, la más nueva de
// este deploy (creada desde appliedAt−2s) que falló o fue rechazada, o que
// trae un Err. Una tarea que no pasó el healthcheck termina complete y sin
// Err; una rechazada antes de este deploy (Swarm la reintentó) no cuenta.
func (r *run) taskDetails(ctx context.Context, short, serviceID string, appliedAt time.Time, hint string, logs bool) {
	tasks, err := r.u.Cluster.Tasks(ctx, serviceID)
	if err != nil {
		if ctx.Err() == nil {
			r.line("%s: no se pudieron leer sus tareas: %s", short, dockerMessage(err))
		}
		return
	}
	failed := failedTask(tasks, appliedAt, hint)
	if failed == nil {
		return
	}
	if failed.Err != "" {
		r.line("%s: última tarea fallida: %s", short, failed.Err)
	} else {
		r.line("%s: última tarea fallida: terminó en %s, sin un motivo de Swarm", short, failed.State)
	}
	if !logs {
		return
	}
	lines, err := r.u.Cluster.TaskLogs(ctx, failed.ID, failedTaskLines)
	if err != nil {
		if ctx.Err() == nil {
			r.line("%s: no se pudo leer el log de su última tarea fallida: %s", short, dockerMessage(err))
		}
		return
	}
	joined := service.JoinPieces(lines)
	if len(joined) > failedTaskLines {
		joined = joined[len(joined)-failedTaskLines:]
	}
	texts := make([]string, len(joined))
	for i, l := range joined {
		texts[i] = "    | " + l.Text
	}
	r.lines(texts)
}

// failedTask elige la tarea de taskDetails; tasks va de la más nueva a la más vieja.
func failedTask(tasks []service.SwarmTask, appliedAt time.Time, hint string) *service.SwarmTask {
	if hint != "" {
		for i, t := range tasks {
			if t.ID != "" && strings.Contains(hint, t.ID) {
				return &tasks[i]
			}
		}
	}
	since := appliedAt.Add(-2 * time.Second)
	for i, t := range tasks {
		if t.CreatedAt.Before(since) {
			continue
		}
		if t.State == "failed" || t.State == "rejected" || t.Err != "" {
			return &tasks[i]
		}
	}
	return nil
}

// routes es el paso 7: un error de SyncRoutes es solo una advertencia (§13).
func (r *run) routes(ctx context.Context) error {
	rctx, cancel := context.WithTimeout(ctx, r.u.Timing.RoutesTimeout)
	err := r.u.Ingress.SyncRoutes(rctx)
	cancel()
	if err == nil {
		r.line("rutas de Caddy actualizadas")
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	r.line("advertencia: no se pudo actualizar Caddy: %v", err)
	return nil
}
