package usecases

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	outservice "github.com/CPU-commits/muelle/src/internal/deploy/infra/out/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	srepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// logsCluster envuelve al adaptador: anota las LogOptions de cada
// ServiceLogs y, si end no es nil, el stream termina con io.EOF cuando se
// cierra end (como el log de un servicio que se borra con follow).
type logsCluster struct {
	service.Cluster
	mu   sync.Mutex
	opts []service.LogOptions
	end  chan struct{}
}

func (c *logsCluster) ServiceLogs(ctx context.Context, id string, o service.LogOptions) (service.LogStream, error) {
	c.mu.Lock()
	c.opts = append(c.opts, o)
	c.mu.Unlock()
	st, err := c.Cluster.ServiceLogs(ctx, id, o)
	if err != nil || c.end == nil {
		return st, err
	}
	return &endingStream{LogStream: st, end: c.end}, nil
}

func (c *logsCluster) options() []service.LogOptions {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.opts)
}

// endingStream devuelve io.EOF cuando se cierra end.
type endingStream struct {
	service.LogStream
	end chan struct{}
}

func (s *endingStream) Next() (service.LogLine, error) {
	type res struct {
		l   service.LogLine
		err error
	}
	ch := make(chan res, 1)
	go func() { l, err := s.LogStream.Next(); ch <- res{l, err} }()
	select {
	case r := <-ch:
		return r.l, r.err
	case <-s.end:
		s.LogStream.Close()
		return service.LogLine{}, io.EOF
	}
}

type logsFixture struct {
	*fixture
	docker  *dockertest.Fake
	env     *fakeEnv
	cluster *logsCluster
	u       *AppLogs
}

func newLogsFixture(t *testing.T) *logsFixture {
	t.Helper()
	f := newFixture(t)
	d := dockertest.New()
	c := &logsCluster{Cluster: outservice.NewDocker(d)}
	env := newFakeEnv()
	return &logsFixture{fixture: f, docker: d, env: env, cluster: c, u: NewAppLogs(f.apps, c, env)}
}

var apiService = naming.Service("auditfy", "staging", "api", "api")

// putAPI deja el servicio api de la app, con dos tareas (slots 1 y 2).
func (f *logsFixture) putAPI() {
	two := uint64(2)
	f.docker.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: apiService, Labels: naming.Labels("auditfy", "staging", "api", 3), Replicas: &two,
	},
		docker.Task{ID: "t1aaaaaaaaaa", Slot: 1, State: "running", DesiredState: "running"},
		docker.Task{ID: "t2bbbbbbbbbb", Slot: 2, State: "running", DesiredState: "running"})
}

func str(s string) *string { return &s }

func logsReq(service string, kv ...string) dto.AppLogsRequest {
	in := dto.AppLogsRequest{AppRef: apiRef, Service: service}
	for i := 0; i+1 < len(kv); i += 2 {
		switch kv[i] {
		case "tail":
			in.Tail = str(kv[i+1])
		case "since":
			in.Since = str(kv[i+1])
		case "follow":
			in.Follow = str(kv[i+1])
		}
	}
	return in
}

// collect lee las líneas hasta que el stream termina (o pasan 5 s).
func collect(t *testing.T, s *AppLogStream) []dto.AppLogLine {
	t.Helper()
	var out []dto.AppLogLine
	timeout := time.After(5 * time.Second)
	for {
		select {
		case l, ok := <-s.Lines():
			if !ok {
				return out
			}
			out = append(out, l)
		case <-timeout:
			t.Fatalf("el stream no terminó; llegaron %+v", out)
		}
	}
}

func nextLine(t *testing.T, s *AppLogStream) dto.AppLogLine {
	t.Helper()
	select {
	case l, ok := <-s.Lines():
		if !ok {
			t.Fatalf("el stream terminó: %v", s.Err())
		}
		return l
	case <-time.After(5 * time.Second):
		t.Fatal("no llegó ninguna línea")
	}
	return dto.AppLogLine{}
}

// La query (contrato §2.9), después de la app: service obligatorio y slug;
// tail de 0 a 5000 (200 por defecto); since RFC 3339, que reemplaza a tail
// (tail no se mira); follow vacío, 0 o 1.
func TestAppLogsQuery(t *testing.T) {
	f := newLogsFixture(t)
	f.putAPI()
	ctx := context.Background()
	if _, err := f.u.Open(ctx, dto.AppLogsRequest{AppRef: dto.AppRef{Project: "auditfy", Stage: "staging", App: "nada"}}); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("la app antes que la query: %v", err)
	}
	for _, c := range []struct {
		in   dto.AppLogsRequest
		want string
	}{
		{logsReq(""), "service: es obligatorio"},
		{logsReq("API"), "service: "},
		{logsReq("api_api"), "service: "},
		{logsReq("abcdefghijklmnopq"), "service: "},
		{logsReq("", "tail", "abc"), "service: es obligatorio"},
		{logsReq("api", "tail", "abc"), "tail: "},
		{logsReq("api", "tail", ""), "tail: "},
		{logsReq("api", "tail", "-1"), "tail: "},
		{logsReq("api", "tail", "5001"), "tail: "},
		{logsReq("api", "tail", "+5"), "tail: "},
		{logsReq("api", "since", "ayer"), "since: "},
		{logsReq("api", "since", ""), "since: "},
		{logsReq("api", "since", "ayer", "tail", "abc"), "since: "},
		{logsReq("api", "follow", "2"), "follow: "},
		{logsReq("api", "follow", "true"), "follow: "},
		{logsReq("api", "tail", "abc", "follow", "2"), "tail: "},
	} {
		_, err := f.u.Open(ctx, c.in)
		if err == nil {
			t.Errorf("%+v: sin error", c.in)
			continue
		}
		if msg := validationMessage(t, err); !strings.HasPrefix(msg, c.want) {
			t.Errorf("%+v: %q, esperaba %q", c.in, msg, c.want)
		}
	}

	since := "2026-10-10T14:31:40.118Z"
	sinceT := time.Date(2026, 10, 10, 14, 31, 40, 118_000_000, time.UTC)
	for _, c := range []struct {
		in   dto.AppLogsRequest
		want service.LogOptions
	}{
		{logsReq("api"), service.LogOptions{Tail: 200}},
		{logsReq("api", "tail", "0", "follow", "1"), service.LogOptions{Tail: 0, Follow: true}},
		{logsReq("api", "tail", "5000", "follow", "0"), service.LogOptions{Tail: 5000}},
		{logsReq("api", "follow", ""), service.LogOptions{Tail: 200}},
		{logsReq("api", "since", since, "tail", "10"), service.LogOptions{Tail: 5000, Since: sinceT}},
		{logsReq("api", "since", since, "tail", "abc"), service.LogOptions{Tail: 5000, Since: sinceT}},
		{logsReq("api", "since", since, "tail", "99999", "follow", "1"), service.LogOptions{Tail: 5000, Since: sinceT, Follow: true}},
		{logsReq("api", "since", "2026-10-10T11:31:40.118-03:00"), service.LogOptions{Tail: 5000, Since: sinceT}},
	} {
		before := len(f.cluster.options())
		s, err := f.u.Open(ctx, c.in)
		if err != nil {
			t.Errorf("%+v: %v", c.in, err)
			continue
		}
		s.Close()
		opts := f.cluster.options()
		if len(opts) != before+1 || opts[before].Tail != c.want.Tail || opts[before].Follow != c.want.Follow || !opts[before].Since.Equal(c.want.Since) {
			t.Errorf("%+v: %+v, esperaba %+v", c.in, opts[len(opts)-1], c.want)
		}
	}
}

// El servicio se busca por su nombre completo de Swarm y tiene que llevar
// las etiquetas de la app: el parámetro nunca llega tal cual a Docker (podría
// ser un id o un prefijo de id de otro servicio).
func TestAppLogsLookup(t *testing.T) {
	f := newLogsFixture(t)
	ctx := context.Background()
	if _, err := f.u.Open(ctx, logsReq("api")); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("sin el servicio: %v", err)
	}
	// Uno con ese nombre que no creó muelle (sin las etiquetas de la app).
	f.docker.PutService(docker.Service{Name: apiService, Labels: map[string]string{"x": "y"}})
	if _, err := f.u.Open(ctx, logsReq("api")); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("sin las etiquetas: %v", err)
	}
	// Con las de otra app tampoco.
	f.docker.PutService(docker.Service{Name: apiService, Labels: naming.AppLabels("auditfy", "staging", "web")})
	if _, err := f.u.Open(ctx, logsReq("api")); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("con las etiquetas de otra app: %v", err)
	}
	// El id de un servicio como parámetro se busca como nombre corto
	// (auditfy_staging_api_<id>): nunca llega tal cual a Docker.
	f.putAPI()
	svc, _ := f.docker.InspectService(ctx, apiService)
	if _, err := f.u.Open(ctx, logsReq(svc.ID)); !errors.Is(err, srepo.ErrNotFound) {
		t.Fatalf("un id como servicio: %v", err)
	}
	s, err := f.u.Open(ctx, logsReq("api"))
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, c := range f.docker.Calls() {
		op, arg, _ := strings.Cut(c, " ")
		if (op == "InspectService" && !strings.HasPrefix(arg, "auditfy_staging_api_")) || (op == "ServiceLogs" && arg != svc.ID) {
			t.Fatalf("llamada a Docker con otro nombre: %q (%v)", c, f.docker.Calls())
		}
	}
	f.docker.SetUnavailable(true)
	if _, err := f.u.Open(ctx, logsReq("api")); !errors.Is(err, shared.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}

// since es inclusivo y time va truncada al milisegundo: reconectar con el
// time de la última línea la vuelve a traer.
func TestAppLogsSinceInclusive(t *testing.T) {
	f := newLogsFixture(t)
	f.putAPI()
	base := time.Date(2026, 10, 10, 14, 31, 40, 0, time.UTC)
	f.docker.AppendServiceLogs(apiService,
		docker.LogLine{Time: base, Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: "uno"},
		docker.LogLine{Time: base.Add(1500 * time.Microsecond), Stream: "stderr", TaskID: "t2bbbbbbbbbb", Text: "dos"},
	)
	s, err := f.u.Open(context.Background(), logsReq("api"))
	if err != nil {
		t.Fatal(err)
	}
	lines := collect(t, s)
	if !errors.Is(s.Err(), io.EOF) || len(lines) != 2 {
		t.Fatalf("%+v, %v", lines, s.Err())
	}
	last := lines[1]
	if last.Time == nil || !last.Time.Equal(base.Add(time.Millisecond)) || *last.Task != "api.2" || last.Stream != "stderr" || last.Line != "dos" {
		t.Fatalf("última línea: %+v", last)
	}
	if first := lines[0]; *first.Task != "api.1" || first.Stream != "stdout" {
		t.Fatalf("primera línea: %+v", first)
	}
	s, err = f.u.Open(context.Background(), logsReq("api", "since", last.Time.Format(time.RFC3339Nano)))
	if err != nil {
		t.Fatal(err)
	}
	again := collect(t, s)
	if len(again) != 1 || again[0].Line != "dos" {
		t.Fatalf("desde la última: %+v", again)
	}
}

// Un secreto partido en dos pedazos de una línea larga sale enmascarado: los
// pedazos se unen antes de enmascarar (contrato §2.5), y la línea se corta a
// 16 KiB. Un pedazo que llega solo se manda a los 200 ms.
func TestAppLogsMasksAndJoins(t *testing.T) {
	f := newLogsFixture(t)
	f.putAPI()
	const secret = "s3cr3t-0123456789abc"
	f.env.set(f.apiID, map[string]string{"K": secret}, secret)
	at := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	head := strings.Repeat("x", 16<<10-10) + secret[:10]
	f.docker.AppendServiceLogs(apiService,
		docker.LogLine{Time: at, Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: head},
		docker.LogLine{Stream: "stdout", Text: secret[10:] + " fin"},
		docker.LogLine{Time: at, Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: "corta " + secret},
		docker.LogLine{Time: at, Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: strings.Repeat("é", 10<<10)},
	)
	s, err := f.u.Open(context.Background(), logsReq("api", "follow", "1"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	joined := nextLine(t, s)
	if strings.Contains(joined.Line, secret[:10]) || strings.Contains(joined.Line, secret[10:]) || !strings.HasSuffix(joined.Line, "*** fin") {
		t.Fatalf("la línea unida: …%q", joined.Line[len(joined.Line)-40:])
	}
	if l := nextLine(t, s); l.Line != "corta ***" {
		t.Fatalf("segunda: %q", l.Line)
	}
	// La última (20 KiB de é) se corta a 16 KiB con "…", sin partir una runa;
	// llega a los 200 ms aunque no venga nada después.
	long := nextLine(t, s)
	if len(long.Line) > 16<<10 || !strings.HasSuffix(long.Line, "…") || strings.ContainsRune(long.Line, '�') {
		t.Fatalf("la larga: %d bytes, …%q", len(long.Line), long.Line[len(long.Line)-8:])
	}
}

// Con follow, una tarea nueva (un deploy durante el stream) sale con su slot:
// se vuelven a leer las tareas una vez por id desconocido.
func TestAppLogsNewTaskSlot(t *testing.T) {
	f := newLogsFixture(t)
	f.putAPI()
	s, err := f.u.Open(context.Background(), logsReq("api", "follow", "1"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	tasksCalls := func() int {
		n := 0
		for _, c := range f.docker.Calls() {
			if strings.HasPrefix(c, "ListTasks") {
				n++
			}
		}
		return n
	}
	before := tasksCalls()
	f.docker.SetTasks(apiService,
		docker.Task{ID: "t3cccccccccc", Slot: 2, State: "running", DesiredState: "running"},
		docker.Task{ID: "t1aaaaaaaaaa", Slot: 1, State: "running", DesiredState: "running"})
	at := time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)
	f.docker.AppendServiceLogs(apiService, docker.LogLine{Time: at, Stream: "stdout", TaskID: "t3cccccccccc", Text: "nueva"})
	if l := nextLine(t, s); *l.Task != "api.2" {
		t.Fatalf("tarea nueva: %q", *l.Task)
	}
	f.docker.AppendServiceLogs(apiService, docker.LogLine{Time: at, Stream: "stdout", TaskID: "t3cccccccccc", Text: "otra"})
	nextLine(t, s)
	// Un id que sigue sin aparecer sale con sus 7 primeros, sin volver a preguntar.
	f.docker.AppendServiceLogs(apiService, docker.LogLine{Time: at, Stream: "stdout", TaskID: "zzzzzzzzzzzz", Text: "huérfana"})
	if l := nextLine(t, s); *l.Task != "api.zzzzzzz" {
		t.Fatalf("tarea que no está: %q", *l.Task)
	}
	f.docker.AppendServiceLogs(apiService, docker.LogLine{Time: at, Stream: "stdout", TaskID: "zzzzzzzzzzzz", Text: "huérfana 2"})
	nextLine(t, s)
	if n := tasksCalls() - before; n != 2 {
		t.Fatalf("ListTasks después de abrir: %d, esperaba 2 (uno por id desconocido)", n)
	}
}

// Un servicio global: la tarea es <servicio>.<7 del id>.
func TestAppLogsGlobalTask(t *testing.T) {
	f := newLogsFixture(t)
	f.docker.PutStackService(naming.Stack("auditfy", "staging", "api"), docker.Service{
		Name: apiService, Labels: naming.AppLabels("auditfy", "staging", "api"),
	}, docker.Task{ID: "g1abcdefghij", State: "running", DesiredState: "running"})
	f.docker.AppendServiceLogs(apiService, docker.LogLine{Time: t0, Stream: "stdout", TaskID: "g1abcdefghij", Text: "hola"})
	s, err := f.u.Open(context.Background(), logsReq("api"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := collect(t, s); len(lines) != 1 || *lines[0].Task != "api.g1abcde" {
		t.Fatalf("%+v", lines)
	}
}

// Sin follow, las líneas y io.EOF; con follow, el log termina (io.EOF)
// cuando Docker lo cierra (el servicio se borró), y Close lo corta.
func TestAppLogsFollowEnd(t *testing.T) {
	f := newLogsFixture(t)
	f.putAPI()
	f.docker.AppendServiceLogs(apiService, docker.LogLine{Time: t0, Stream: "stdout", TaskID: "t1aaaaaaaaaa", Text: "uno"})
	s, err := f.u.Open(context.Background(), logsReq("api"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := collect(t, s); len(lines) != 1 || !errors.Is(s.Err(), io.EOF) {
		t.Fatalf("sin follow: %+v, %v", lines, s.Err())
	}

	f.cluster.end = make(chan struct{})
	s, err = f.u.Open(context.Background(), logsReq("api", "follow", "1"))
	if err != nil {
		t.Fatal(err)
	}
	if l := nextLine(t, s); l.Line != "uno" {
		t.Fatalf("con follow: %+v", l)
	}
	close(f.cluster.end)
	if lines := collect(t, s); len(lines) != 0 || !errors.Is(s.Err(), io.EOF) {
		t.Fatalf("al borrarse el servicio: %+v, %v", lines, s.Err())
	}
	f.cluster.end = nil

	// Close termina un stream con follow (el cliente se fue).
	s, err = f.u.Open(context.Background(), logsReq("api", "follow", "1"))
	if err != nil {
		t.Fatal(err)
	}
	nextLine(t, s)
	s.Close()
	collect(t, s)
	if errors.Is(s.Err(), io.EOF) {
		t.Fatal("un stream cerrado no terminó por EOF")
	}
}
