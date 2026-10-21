package usecases

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// Los límites de GET …/logs (contrato §2.9).
const (
	// appLogsMaxTail es el máximo de tail, y lo que se pide con since.
	appLogsMaxTail     = 5000
	appLogsDefaultTail = 200
	// appLogsPieceWait es cuánto espera una línea a que llegue el pedazo
	// que la sigue (una línea de más de 16 KiB llega partida) antes de salir.
	appLogsPieceWait = 200 * time.Millisecond
)

// AppLogs son los logs de un servicio de una app, en vivo (contrato §2.9).
// No se guarda nada.
type AppLogs struct {
	apps    service.Apps
	cluster service.Cluster
	env     service.EnvResolver
}

func NewAppLogs(apps service.Apps, cluster service.Cluster, env service.EnvResolver) *AppLogs {
	return &AppLogs{apps: apps, cluster: cluster, env: env}
}

// appLogsQuery es la query de GET …/logs ya validada.
type appLogsQuery struct {
	service string
	opts    service.LogOptions
}

// parseAppLogsQuery valida la query en el orden de la tabla del contrato
// §2.9: service, tail (solo sin since: since lo reemplaza y tail no se
// mira), since y follow.
func parseAppLogsQuery(in dto.AppLogsRequest) (appLogsQuery, error) {
	q := appLogsQuery{service: in.Service, opts: service.LogOptions{Tail: appLogsDefaultTail}}
	if in.Service == "" {
		return q, validate.Errorf("service", "es obligatorio")
	}
	if err := validate.VolumeName("service", in.Service); err != nil {
		return q, err
	}
	if in.Since == nil && in.Tail != nil {
		n, err := strconv.Atoi(*in.Tail)
		if err != nil || strings.Trim(*in.Tail, "0123456789") != "" || n > appLogsMaxTail {
			return q, validate.Errorf("tail", "tiene que ser un entero de 0 a %d", appLogsMaxTail)
		}
		q.opts.Tail = n
	}
	if in.Since != nil {
		t, err := time.Parse(time.RFC3339Nano, *in.Since)
		if err != nil {
			return q, validate.Errorf("since", "tiene que ser una fecha RFC 3339 (2026-10-10T14:31:40.118Z)")
		}
		q.opts.Since, q.opts.Tail = t.UTC(), appLogsMaxTail
	}
	if in.Follow != nil {
		switch *in.Follow {
		case "", "0":
		case "1":
			q.opts.Follow = true
		default:
			return q, validate.Errorf("follow", "tiene que ser 0 o 1")
		}
	}
	return q, nil
}

// Open valida y abre el log, en el orden del contrato §2.9: la app (404), la
// query (422) y el servicio en Swarm (404 si no está o no es de la app; 503
// con Docker caído). El servicio se busca por su nombre completo de Swarm
// (naming.Service, con "_": nunca es un id ni un prefijo de id) y tiene que
// llevar todas las etiquetas de la app; ServiceLogs recibe su ID. Cerrar el
// stream (Close) siempre.
func (u *AppLogs) Open(ctx context.Context, in dto.AppLogsRequest) (*AppLogStream, error) {
	app, err := u.apps.Get(ctx, in.Project, in.Stage, in.App)
	if err != nil {
		return nil, err
	}
	q, err := parseAppLogsQuery(in)
	if err != nil {
		return nil, err
	}
	svc, err := u.cluster.Service(ctx, naming.Service(app.Project, app.Stage, app.Slug, q.service))
	if errors.Is(err, service.ErrSwarmNotFound) {
		return nil, srepo.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	for k, v := range naming.AppLabels(app.Project, app.Stage, app.Slug) {
		if svc.Labels[k] != v {
			return nil, srepo.ErrNotFound // uno con ese nombre que no creó muelle
		}
	}
	_, secrets, err := u.env.ResolveEnv(ctx, app.ID)
	if err != nil {
		return nil, err
	}
	tasks, err := u.cluster.Tasks(ctx, svc.ID)
	if errors.Is(err, service.ErrSwarmNotFound) {
		return nil, srepo.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	st, err := u.cluster.ServiceLogs(ctx, svc.ID, q.opts)
	if errors.Is(err, service.ErrSwarmNotFound) {
		return nil, srepo.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	sctx, cancel := context.WithCancel(ctx)
	s := &AppLogStream{
		cluster: u.cluster, serviceID: svc.ID, short: q.service, masker: service.NewMasker(secrets),
		slots: map[string]int{}, unknown: map[string]bool{}, global: svc.Replicas == nil,
		st: st, cancel: cancel, out: make(chan dto.AppLogLine),
	}
	s.setSlots(tasks)
	raw := make(chan service.LogLine)
	s.wg.Go(func() { s.read(sctx, raw) })
	s.wg.Go(func() { s.process(sctx, raw) })
	return s, nil
}

// AppLogStream es el log abierto de un servicio. Una goroutine lee el log de
// Docker y otra une los pedazos, nombra la tarea, enmascara y corta cada
// línea, y la entrega en Lines.
type AppLogStream struct {
	cluster   service.Cluster
	serviceID string
	short     string
	masker    service.Masker
	global    bool

	// slots es el slot de cada tarea conocida; unknown, las que se buscaron
	// y no están (no se vuelve a preguntar). Solo los usa process.
	slots   map[string]int
	unknown map[string]bool

	st        service.LogStream
	cancel    context.CancelFunc
	out       chan dto.AppLogLine
	wg        sync.WaitGroup
	closeOnce sync.Once

	err error // por qué terminó; se escribe antes de cerrar out
}

// Lines entrega las líneas; se cierra cuando el log termina (Err dice por qué).
func (s *AppLogStream) Lines() <-chan dto.AppLogLine { return s.out }

// Err es por qué terminó el log, después de que Lines se cerró: io.EOF si
// Docker lo terminó (sin follow, o el servicio se borró); otro error si se
// cortó o se cerró.
func (s *AppLogStream) Err() error { return s.err }

// Close corta el log y espera a sus goroutines. Se puede llamar varias veces.
func (s *AppLogStream) Close() {
	s.closeOnce.Do(func() {
		s.cancel()
		s.st.Close()
		s.wg.Wait()
	})
}

func (s *AppLogStream) setSlots(tasks []service.SwarmTask) {
	for _, t := range tasks {
		s.slots[t.ID] = t.Slot
	}
}

// read pasa cada línea de Docker a process. Al terminar escribe s.err (el
// error de Next, o el del ctx) y cierra raw: process lo lee recién después.
func (s *AppLogStream) read(ctx context.Context, raw chan<- service.LogLine) {
	defer close(raw)
	for {
		l, err := s.st.Next()
		if err != nil {
			s.err = err
			return
		}
		select {
		case raw <- l:
		case <-ctx.Done():
			s.err = ctx.Err()
			return
		}
	}
}

// process une los pedazos de una línea larga (service.JoinPieces) con una
// línea pendiente, que sale cuando llega la siguiente o a los 200 ms sin
// nada. Al terminar espera a que read cierre raw (read termina con el ctx o
// con Close) y recién entonces cierra Lines: así s.err ya está escrito.
func (s *AppLogStream) process(ctx context.Context, raw <-chan service.LogLine) {
	defer close(s.out)
	defer func() {
		for range raw {
		}
	}()
	var pending *service.LogLine
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	flush := func() bool {
		if pending == nil {
			return true
		}
		l := s.line(ctx, *pending)
		pending = nil
		select {
		case s.out <- l:
			return true
		case <-ctx.Done():
			return false
		}
	}
	for {
		select {
		case l, ok := <-raw:
			if !ok {
				if ctx.Err() == nil {
					flush()
				}
				return
			}
			if pending != nil {
				if joined := service.JoinPieces([]service.LogLine{*pending, l}); len(joined) == 1 {
					pending = &joined[0]
					timer.Reset(appLogsPieceWait)
					continue
				}
				if !flush() {
					return
				}
			}
			pending = &l
			timer.Reset(appLogsPieceWait)
		case <-timer.C:
			if !flush() {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// line arma la línea de §2.9: la hora truncada al milisegundo, la tarea, y
// el texto enmascarado y cortado a 16 KiB.
func (s *AppLogStream) line(ctx context.Context, l service.LogLine) dto.AppLogLine {
	out := dto.AppLogLine{Stream: l.Stream, Line: service.CutLine(s.masker.Mask(l.Text), service.AppLineBytes)}
	if !l.Time.IsZero() {
		out.Time = new(l.Time.UTC().Truncate(time.Millisecond))
	}
	if l.TaskID != "" {
		out.Task = new(s.taskName(ctx, l.TaskID))
	}
	return out
}

// taskName es <servicio>.<slot>, o <servicio>.<7 del id> en un servicio
// global o en una tarea que no está. Una tarea desconocida (un deploy creó
// tareas nuevas mientras el stream seguía) vuelve a leer las tareas una sola
// vez por id.
func (s *AppLogStream) taskName(ctx context.Context, id string) string {
	slot, ok := s.slots[id]
	if !ok && !s.unknown[id] {
		if tasks, err := s.cluster.Tasks(ctx, s.serviceID); err == nil {
			s.setSlots(tasks)
		}
		if slot, ok = s.slots[id]; !ok {
			s.unknown[id] = true
		}
	}
	if !ok || s.global || slot == 0 {
		short := id
		if len(short) > 7 {
			short = short[:7]
		}
		return s.short + "." + short
	}
	return s.short + "." + strconv.Itoa(slot)
}
