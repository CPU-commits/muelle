package sqlite

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/service"
	"github.com/CPU-commits/muelle/src/shared/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/sqlite/sqlitetest"
)

var t0 = time.Date(2026, 10, 10, 14, 0, 0, 0, time.UTC)

func exec(t *testing.T, db *sqlite.DB, query string, args ...any) int64 {
	t.Helper()
	ctx := context.Background()
	res, err := db.Conn(ctx).ExecContext(ctx, query, args...)
	if err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// seedApp siembra por SQL (las tablas son de catalog) el proyecto p, el stage
// s y una app de formulario con ese slug, y devuelve su id.
func seedApp(t *testing.T, db *sqlite.DB, slug string) int64 {
	t.Helper()
	exec(t, db, `INSERT OR IGNORE INTO projects (slug, name, created_at) VALUES ('p', 'P', 0)`)
	exec(t, db, `INSERT OR IGNORE INTO stages (project_id, slug, created_at) SELECT id, 's', 0 FROM projects WHERE slug = 'p'`)
	return exec(t, db, `INSERT INTO apps (stage_id, slug, kind, spec, config_changed_at, created_at)
		SELECT id, ?, 'service', '{"image":"x:1","port":80}', 0, 0 FROM stages WHERE slug = 's'`, slug)
}

func ptr[T any](v T) *T { return &v }

// queued crea un deploy queued de la app, con un panel de "ana".
func queued(t *testing.T, r *DeployRepository, appID int64, at time.Time, images map[string]string) entity.Deploy {
	t.Helper()
	d := entity.Deploy{AppID: appID, Trigger: entity.TriggerPanel, Images: images,
		ActorType: entity.ActorUser, ActorID: ptr(int64(1)), ActorName: "ana", CreatedAt: at}
	if err := r.Create(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	return d
}

// closeAs lleva un deploy queued a status: running, o un final (pasando por
// running, salvo failed, que se cierra desde queued).
func closeAs(t *testing.T, r *DeployRepository, d entity.Deploy, status entity.Status) {
	t.Helper()
	ctx := context.Background()
	if status != entity.StatusFailed {
		if ok, err := r.Claim(ctx, d.ID, t0); err != nil || !ok {
			t.Fatalf("Claim(%d) = %v, %v", d.ID, ok, err)
		}
	}
	if status == entity.StatusRunning {
		return
	}
	var msg *string
	if status == entity.StatusFailed {
		msg = ptr("falló")
	}
	if err := r.Finish(ctx, d.ID, status, msg, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
}

func mustGet(t *testing.T, r *DeployRepository, id int64) entity.Deploy {
	t.Helper()
	d, err := r.GetByID(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestDeployCreateAndSupersede(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app, other := seedApp(t, db, "api"), seedApp(t, db, "web")

	d1 := queued(t, r, app, t0, map[string]string{"api": "ghcr.io/x/api:sha-1"})
	if d1.ID == 0 || d1.Status != entity.StatusQueued {
		t.Fatalf("Create: %+v", d1)
	}
	got := mustGet(t, r, d1.ID)
	want := entity.Deploy{ID: d1.ID, AppID: app, Status: entity.StatusQueued, Trigger: entity.TriggerPanel,
		Images: map[string]string{"api": "ghcr.io/x/api:sha-1"}, RequestedImages: map[string]string{}, ConfigImages: map[string]string{},
		ActorType: entity.ActorUser, ActorID: ptr(int64(1)), ActorName: "ana", CreatedAt: t0}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetByID:\n got %+v\nwant %+v", got, want)
	}
	// Sin overrides, images es {} (nunca null).
	dOther := queued(t, r, other, t0, nil)
	if g := mustGet(t, r, dOther.ID); g.Images == nil || len(g.Images) != 0 {
		t.Fatalf("images de un deploy sin overrides: %#v", g.Images)
	}

	d2 := queued(t, r, app, t0.Add(time.Second), nil)
	at := t0.Add(2 * time.Second)
	ids, err := r.SupersedeQueued(ctx, app, d2.ID, at)
	if err != nil || !slices.Equal(ids, []int64{d1.ID}) {
		t.Fatalf("SupersedeQueued = %v, %v", ids, err)
	}
	old := mustGet(t, r, d1.ID)
	if old.Status != entity.StatusSuperseded || old.SupersededBy == nil || *old.SupersededBy != d2.ID ||
		old.FinishedAt == nil || !old.FinishedAt.Equal(at) || old.StartedAt != nil {
		t.Fatalf("el viejo: %+v", old)
	}
	// El de otra app sigue queued.
	if g := mustGet(t, r, dOther.ID); g.Status != entity.StatusQueued {
		t.Fatalf("otra app: %+v", g)
	}

	// Un running no se toca; sin queued que reemplazar, [] (no nil).
	if ok, err := r.Claim(ctx, d2.ID, at); err != nil || !ok {
		t.Fatalf("Claim = %v, %v", ok, err)
	}
	d3 := queued(t, r, app, at, nil)
	ids, err = r.SupersedeQueued(ctx, app, d3.ID, at)
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("sin queued: %#v, %v", ids, err)
	}
	if g := mustGet(t, r, d2.ID); g.Status != entity.StatusRunning {
		t.Fatalf("el running: %+v", g)
	}
}

// Un DELETE de la app entre su lectura y el INSERT es un 404, no el error de
// la clave foránea.
func TestDeployCreateAppGone(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	d := entity.Deploy{AppID: 999, Trigger: entity.TriggerAPI, ActorType: entity.ActorToken, ActorName: "ci", CreatedAt: t0}
	if err := r.Create(context.Background(), &d); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Create de una app que no existe: %v", err)
	}
}

// SetRoutesWarning marca que el paso 7 no pudo actualizar Caddy (contrato
// §2.1): un deploy nace sin la marca, y la lee cualquier consulta.
func TestDeploySetRoutesWarning(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app := seedApp(t, db, "api")
	d := queued(t, r, app, t0, nil)
	other := queued(t, r, seedApp(t, db, "web"), t0, nil)
	if mustGet(t, r, d.ID).RoutesWarning {
		t.Fatal("un deploy nuevo ya trae routes_warning")
	}
	closeAs(t, r, d, entity.StatusRunning)
	if err := r.SetRoutesWarning(ctx, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(ctx, d.ID, entity.StatusSucceeded, nil, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if got := mustGet(t, r, d.ID); !got.RoutesWarning || got.Status != entity.StatusSucceeded {
		t.Fatalf("GetByID: %+v", got)
	}
	if got, err := r.Get(ctx, app, d.ID); err != nil || !got.RoutesWarning {
		t.Fatalf("Get: %+v %v", got, err)
	}
	if list, err := r.List(ctx, app); err != nil || len(list) != 1 || !list[0].RoutesWarning {
		t.Fatalf("List: %+v %v", list, err)
	}
	if cur, err := r.Current(ctx, app); err != nil || cur == nil || !cur.RoutesWarning {
		t.Fatalf("Current: %+v %v", cur, err)
	}
	if mustGet(t, r, other.ID).RoutesWarning {
		t.Fatal("la marca pasó a otro deploy")
	}
	if err := r.SetRoutesWarning(ctx, 9999); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("un deploy que no existe: %v", err)
	}
}

func TestDeployGetListAndLifecycle(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app, other := seedApp(t, db, "api"), seedApp(t, db, "web")

	if list, err := r.List(ctx, app); err != nil || list == nil || len(list) != 0 {
		t.Fatalf("List vacía: %#v, %v", list, err)
	}
	for _, f := range []func() (*entity.Deploy, error){
		func() (*entity.Deploy, error) { return r.Current(ctx, app) },
		func() (*entity.Deploy, error) { return r.NextQueued(ctx, app) },
		func() (*entity.Deploy, error) { return r.Running(ctx, app) },
		func() (*entity.Deploy, error) { return r.PreviousSucceeded(ctx, app, 100) },
	} {
		if d, err := f(); err != nil || d != nil {
			t.Fatalf("sin deploys: %+v, %v", d, err)
		}
	}

	d1 := queued(t, r, app, t0, nil)
	d2 := queued(t, r, app, t0.Add(time.Second), nil)
	dx := queued(t, r, other, t0, nil)
	list, err := r.List(ctx, app)
	if err != nil || len(list) != 2 || list[0].ID != d2.ID || list[1].ID != d1.ID {
		t.Fatalf("List: %+v, %v", list, err)
	}
	// Get es por app: un id de otra app es ErrNotFound.
	if _, err := r.Get(ctx, app, dx.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get de otra app: %v", err)
	}
	if _, err := r.GetByID(ctx, 12345); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetByID inexistente: %v", err)
	}
	if g, err := r.Get(ctx, app, d1.ID); err != nil || g.ID != d1.ID {
		t.Fatalf("Get: %+v, %v", g, err)
	}

	// NextQueued es el más viejo; Running, el que se reclamó.
	if n, err := r.NextQueued(ctx, app); err != nil || n == nil || n.ID != d1.ID {
		t.Fatalf("NextQueued: %+v, %v", n, err)
	}
	if ok, err := r.Claim(ctx, d1.ID, t0.Add(3*time.Second)); err != nil || !ok {
		t.Fatalf("Claim: %v, %v", ok, err)
	}
	if run, err := r.Running(ctx, app); err != nil || run == nil || run.ID != d1.ID || run.StartedAt == nil || !run.StartedAt.Equal(t0.Add(3*time.Second)) {
		t.Fatalf("Running: %+v, %v", run, err)
	}
	if n, err := r.NextQueued(ctx, app); err != nil || n == nil || n.ID != d2.ID {
		t.Fatalf("NextQueued después del Claim: %+v, %v", n, err)
	}

	// SetImages guarda las tres.
	req := map[string]string{"api": "x:1"}
	cfg := map[string]string{"api": "x:1"}
	res := map[string]string{"api": "x:1@sha256:" + strings.Repeat("a", 64)}
	if err := r.SetImages(ctx, d1.ID, req, cfg, res); err != nil {
		t.Fatal(err)
	}
	g := mustGet(t, r, d1.ID)
	if !reflect.DeepEqual(g.RequestedImages, req) || !reflect.DeepEqual(g.ConfigImages, cfg) || !reflect.DeepEqual(g.Images, res) {
		t.Fatalf("SetImages: %+v", g)
	}
	if err := r.SetImages(ctx, 12345, req, cfg, res); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("SetImages inexistente: %v", err)
	}

	// Finish cierra con su error y su hora; un deploy ya cerrado no se vuelve a cerrar.
	if err := r.Finish(ctx, d1.ID, entity.StatusFailed, ptr("render: x"), t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	g = mustGet(t, r, d1.ID)
	if g.Status != entity.StatusFailed || g.Error == nil || *g.Error != "render: x" || g.FinishedAt == nil || !g.FinishedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("Finish: %+v", g)
	}
	if err := r.Finish(ctx, d1.ID, entity.StatusSucceeded, nil, t0); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Finish de uno cerrado: %v", err)
	}
	if err := r.Finish(ctx, d2.ID, entity.StatusRunning, nil, t0); err == nil {
		t.Fatal("Finish con un estado que no es final")
	}
	// Un queued se puede cerrar como failed (lo hace FailQueued, y RunDeploy si
	// la app se borró antes de reclamarlo).
	if err := r.Finish(ctx, d2.ID, entity.StatusFailed, ptr("la app se borró"), t0); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentAndPrevious(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app := seedApp(t, db, "api")

	s1 := queued(t, r, app, t0, nil)
	closeAs(t, r, s1, entity.StatusSucceeded)
	f1 := queued(t, r, app, t0, nil)
	closeAs(t, r, f1, entity.StatusFailed)
	s2 := queued(t, r, app, t0, nil)
	closeAs(t, r, s2, entity.StatusSucceeded)
	sup := queued(t, r, app, t0, nil)
	s3 := queued(t, r, app, t0, nil)
	if _, err := r.SupersedeQueued(ctx, app, s3.ID, t0); err != nil {
		t.Fatal(err)
	}
	closeAs(t, r, s3, entity.StatusSucceeded)
	f2 := queued(t, r, app, t0, nil)
	closeAs(t, r, f2, entity.StatusFailed)
	_ = sup

	cur, err := r.Current(ctx, app)
	if err != nil || cur == nil || cur.ID != s3.ID {
		t.Fatalf("Current: %+v, %v", cur, err)
	}
	prev, err := r.PreviousSucceeded(ctx, app, cur.ID)
	if err != nil || prev == nil || prev.ID != s2.ID {
		t.Fatalf("PreviousSucceeded(s3): %+v, %v", prev, err)
	}
	if prev, err := r.PreviousSucceeded(ctx, app, s1.ID); err != nil || prev != nil {
		t.Fatalf("antes del primero: %+v, %v", prev, err)
	}
}

func TestClaim(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app := seedApp(t, db, "api")

	d := queued(t, r, app, t0, nil)
	if ok, err := r.Claim(ctx, d.ID, t0); err != nil || !ok {
		t.Fatalf("Claim = %v, %v", ok, err)
	}
	if ok, err := r.Claim(ctx, d.ID, t0); err != nil || ok {
		t.Fatalf("segundo Claim = %v, %v", ok, err)
	}
	if ok, err := r.Claim(ctx, 12345, t0); err != nil || ok {
		t.Fatalf("Claim inexistente = %v, %v", ok, err)
	}

	// Dos Claim a la vez: gana uno solo.
	for range 5 {
		d := queued(t, r, app, t0, nil)
		var wins sync.WaitGroup
		results := make(chan bool, 2)
		for range 2 {
			wins.Go(func() {
				ok, err := r.Claim(ctx, d.ID, t0)
				if err != nil {
					t.Error(err)
				}
				results <- ok
			})
		}
		wins.Wait()
		close(results)
		n := 0
		for ok := range results {
			if ok {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("ganaron %d", n)
		}
		closeAs(t, r, d, entity.StatusFailed)
	}
}

// line es una línea de log como la arma FormatLine.
func line(text string) string { return service.FormatLine(service.Masker{}, t0, text) }

func readAll(t *testing.T, r *DeployRepository, appID, id int64) string {
	t.Helper()
	c, err := r.ReadLog(context.Background(), appID, id, 0, service.LogCap+1)
	if err != nil {
		t.Fatal(err)
	}
	return string(c.Chunk)
}

// El tope de 256 KB (contrato §2.5): las normales hasta LogCap−LogReserve, la
// marca una sola vez, las de cierre en la reserva, y la marca queda en la base.
func TestAppendLogCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "muelle.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	r := NewDeployRepository(db)
	app := seedApp(t, db, "api")
	d := queued(t, r, app, t0, nil)
	marker := line(service.TruncatedMarker)

	if err := r.AppendLog(ctx, 12345, []string{line("x")}, false, marker); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("AppendLog inexistente: %v", err)
	}

	// Líneas de 1 KiB hasta pasar 252 KiB, de a 10 por escritura. Con ñ (dos
	// bytes): el tope es en bytes, no en caracteres.
	normal := line(strings.Repeat("ñ", 500))
	for i := 0; i < 300; i += 10 {
		if err := r.AppendLog(ctx, d.ID, slices.Repeat([]string{normal}, 10), false, marker); err != nil {
			t.Fatal(err)
		}
	}
	log := readAll(t, r, app, d.ID)
	if strings.Count(log, marker) != 1 || !strings.HasSuffix(log, marker) {
		t.Fatalf("la marca: %d veces; termina en %q", strings.Count(log, marker), log[len(log)-80:])
	}
	if len(log)-len(marker) > service.LogCap-service.LogReserve {
		t.Fatalf("las normales pasaron el límite: %d", len(log))
	}
	// Las normales que siguen se descartan; las de cierre entran en la reserva.
	if err := r.AppendLog(ctx, d.ID, []string{line("normal descartada")}, false, marker); err != nil {
		t.Fatal(err)
	}
	if err := r.AppendLog(ctx, d.ID, []string{line("failed: algo")}, true, marker); err != nil {
		t.Fatal(err)
	}
	log = readAll(t, r, app, d.ID)
	if strings.Contains(log, "normal descartada") || !strings.HasSuffix(log, line("failed: algo")) || strings.Count(log, marker) != 1 {
		t.Fatalf("después de truncar: …%q", log[len(log)-200:])
	}

	// Reabrir la base mantiene log_truncated: una normal sigue sin entrar.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	r = NewDeployRepository(db)
	before := len(readAll(t, r, app, d.ID))
	if err := r.AppendLog(ctx, d.ID, []string{line("corta")}, false, marker); err != nil {
		t.Fatal(err)
	}
	if after := readAll(t, r, app, d.ID); len(after) != before {
		t.Fatalf("una normal entró después de reabrir: %d → %d", before, len(after))
	}
	// Una de cierre que pasa LogCap no entra, y el log nunca pasa el tope.
	if err := r.AppendLog(ctx, d.ID, []string{line(strings.Repeat("z", 7000))}, true, marker); err != nil {
		t.Fatal(err)
	}
	if err := r.AppendLog(ctx, d.ID, []string{line(strings.Repeat("z", 7000))}, true, marker); err != nil {
		t.Fatal(err)
	}
	if n := len(readAll(t, r, app, d.ID)); n > service.LogCap {
		t.Fatalf("el log pasó el tope: %d", n)
	}
}

// AppendLog usa la transacción del ctx: un rollback se lleva las líneas.
func TestAppendLogInTx(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app := seedApp(t, db, "api")
	d := queued(t, r, app, t0, nil)
	boom := errors.New("boom")
	err := db.InTx(ctx, func(ctx context.Context) error {
		if err := r.AppendLog(ctx, d.ID, []string{line("dentro")}, true, ""); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if log := readAll(t, r, app, d.ID); log != "" {
		t.Fatalf("quedó %q", log)
	}
}

func TestReadLog(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app, other := seedApp(t, db, "api"), seedApp(t, db, "web")
	d := queued(t, r, app, t0, nil)

	// Al principio el log está vacío: offset 0 da "" y 1 pasa el largo.
	c, err := r.ReadLog(ctx, app, d.ID, 0, 64<<10)
	if err != nil || len(c.Chunk) != 0 || c.Length != 0 || c.Status != entity.StatusQueued || c.SupersededBy != nil {
		t.Fatalf("vacío: %+v, %v", c, err)
	}
	if _, err := r.ReadLog(ctx, app, d.ID, 1, 64<<10); !errors.Is(err, service.ErrOffsetBeyond) {
		t.Fatalf("offset más allá: %v", err)
	}

	l1, l2, l3 := line("resolviendo imágenes"), line("año ñandú … listo"), line("succeeded en 3 s")
	if err := r.AppendLog(ctx, d.ID, []string{l1, l2, l3}, false, ""); err != nil {
		t.Fatal(err)
	}
	total := len(l1) + len(l2) + len(l3)

	// Offsets en bytes: después de l1 empieza l2 (con ñ y …, multibyte).
	c, err = r.ReadLog(ctx, app, d.ID, len(l1), 64<<10)
	if err != nil || string(c.Chunk) != l2+l3 || c.Length != total {
		t.Fatalf("desde l1: %q (%d), %v", c.Chunk, c.Length, err)
	}
	// Solo líneas completas: un máximo que corta l2 a la mitad devuelve hasta l1.
	c, err = r.ReadLog(ctx, app, d.ID, 0, len(l1)+5)
	if err != nil || string(c.Chunk) != l1 {
		t.Fatalf("corte a mitad de línea: %q, %v", c.Chunk, err)
	}
	// Un máximo menor que la primera línea da "".
	c, err = r.ReadLog(ctx, app, d.ID, 0, 3)
	if err != nil || len(c.Chunk) != 0 {
		t.Fatalf("máximo menor que una línea: %q, %v", c.Chunk, err)
	}
	// Al final, "" (no un error).
	c, err = r.ReadLog(ctx, app, d.ID, total, 64<<10)
	if err != nil || len(c.Chunk) != 0 || c.Length != total {
		t.Fatalf("al final: %+v, %v", c, err)
	}
	if _, err := r.ReadLog(ctx, app, d.ID, total+1, 64<<10); !errors.Is(err, service.ErrOffsetBeyond) {
		t.Fatalf("más allá del final: %v", err)
	}
	// De otra app: no existe.
	if _, err := r.ReadLog(ctx, other, d.ID, 0, 64<<10); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("otra app: %v", err)
	}

	// Hasta 64 KiB por lectura, en líneas completas.
	big := line(strings.Repeat("b", 8000))
	if err := r.AppendLog(ctx, d.ID, slices.Repeat([]string{big}, 20), false, ""); err != nil {
		t.Fatal(err)
	}
	c, err = r.ReadLog(ctx, app, d.ID, 0, 64<<10)
	if err != nil || len(c.Chunk) > 64<<10 || !bytes.HasSuffix(c.Chunk, []byte("\n")) || len(c.Chunk) < 64<<10-len(big) {
		t.Fatalf("64 KiB: %d bytes, %v", len(c.Chunk), err)
	}

	// Estado y superseded_by vienen con el pedazo.
	d2 := queued(t, r, app, t0, nil)
	if _, err := r.SupersedeQueued(ctx, app, d2.ID, t0); err != nil {
		t.Fatal(err)
	}
	c, err = r.ReadLog(ctx, app, d.ID, 0, 10)
	if err != nil || c.Status != entity.StatusSuperseded || c.SupersededBy == nil || *c.SupersededBy != d2.ID {
		t.Fatalf("superseded: %+v, %v", c, err)
	}
}

// La retención (contrato §2.3): quedan los 20 más nuevos, el actual, el
// succeeded anterior y los queued y running; el resto se borra.
func TestRetention(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app, other := seedApp(t, db, "api"), seedApp(t, db, "web")

	var ds []entity.Deploy
	for range 25 {
		ds = append(ds, queued(t, r, app, t0, nil))
	}
	otherD := queued(t, r, other, t0, nil)
	closeAs(t, r, otherD, entity.StatusFailed)
	// d0: el succeeded anterior; d1: failed; d2: superseded por d3; d3: el
	// actual; d4: running (viejo, a propósito); d5…d24: los 20 más nuevos,
	// todos failed salvo el último, queued (20 fallidos seguidos no borran
	// la versión que corre).
	closeAs(t, r, ds[0], entity.StatusSucceeded)
	closeAs(t, r, ds[1], entity.StatusFailed)
	exec(t, db, `UPDATE deploys SET status = 'superseded', superseded_by = ?, finished_at = 0 WHERE id = ?`, ds[3].ID, ds[2].ID)
	closeAs(t, r, ds[3], entity.StatusSucceeded)
	closeAs(t, r, ds[4], entity.StatusRunning)
	for _, d := range ds[5:24] {
		closeAs(t, r, d, entity.StatusFailed)
	}
	// Uno de los que quedan apunta (a mano) a uno que se borra: su
	// superseded_by queda en NULL (ON DELETE SET NULL).
	exec(t, db, `UPDATE deploys SET status = 'superseded', superseded_by = ? WHERE id = ?`, ds[1].ID, ds[5].ID)

	n, err := r.ApplyRetention(ctx, app)
	if err != nil || n != 2 {
		t.Fatalf("ApplyRetention = %d, %v", n, err)
	}
	list, err := r.List(ctx, app)
	if err != nil {
		t.Fatal(err)
	}
	var kept []int64
	for _, d := range list {
		kept = append(kept, d.ID)
	}
	var want []int64
	for i := 24; i >= 0; i-- {
		if i != 1 && i != 2 {
			want = append(want, ds[i].ID)
		}
	}
	if !slices.Equal(kept, want) {
		t.Fatalf("quedaron %v, esperaba %v", kept, want)
	}
	if g := mustGet(t, r, ds[5].ID); g.SupersededBy != nil {
		t.Fatalf("superseded_by de un borrado: %v", *g.SupersededBy)
	}
	// La otra app no se toca, y otra pasada no borra nada.
	if _, err := r.GetByID(ctx, otherD.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := r.ApplyRetention(ctx, app); err != nil || n != 0 {
		t.Fatalf("segunda pasada = %d, %v", n, err)
	}
}

func TestFailQueued(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	app, other := seedApp(t, db, "api"), seedApp(t, db, "web")

	run := queued(t, r, app, t0, nil)
	closeAs(t, r, run, entity.StatusRunning)
	q1 := queued(t, r, app, t0, nil)
	q2 := queued(t, r, app, t0, nil)
	qx := queued(t, r, other, t0, nil)
	at := t0.Add(time.Hour)

	ids, err := r.FailQueued(ctx, app, "la app se borró", at)
	if err != nil || !slices.Equal(ids, []int64{q1.ID, q2.ID}) {
		t.Fatalf("FailQueued = %v, %v", ids, err)
	}
	for _, id := range ids {
		g := mustGet(t, r, id)
		if g.Status != entity.StatusFailed || g.Error == nil || *g.Error != "la app se borró" || g.FinishedAt == nil || !g.FinishedAt.Equal(at) {
			t.Fatalf("cerrado: %+v", g)
		}
	}
	if g := mustGet(t, r, run.ID); g.Status != entity.StatusRunning {
		t.Fatalf("el running: %+v", g)
	}
	if g := mustGet(t, r, qx.ID); g.Status != entity.StatusQueued {
		t.Fatalf("otra app: %+v", g)
	}
	ids, err = r.FailQueued(ctx, app, "otra vez", at)
	if err != nil || ids == nil || len(ids) != 0 {
		t.Fatalf("sin queued: %#v, %v", ids, err)
	}
}

func TestActiveApps(t *testing.T) {
	db := sqlitetest.New(t)
	r := NewDeployRepository(db)
	ctx := context.Background()
	if apps, err := r.ActiveApps(ctx); err != nil || apps == nil || len(apps) != 0 {
		t.Fatalf("sin deploys: %#v, %v", apps, err)
	}
	a, b, c := seedApp(t, db, "a"), seedApp(t, db, "b"), seedApp(t, db, "c")
	queued(t, r, b, t0, nil)
	queued(t, r, b, t0, nil)
	closeAs(t, r, queued(t, r, a, t0, nil), entity.StatusRunning)
	closeAs(t, r, queued(t, r, c, t0, nil), entity.StatusSucceeded)
	apps, err := r.ActiveApps(ctx)
	if err != nil || !slices.Equal(apps, []int64{a, b}) {
		t.Fatalf("ActiveApps = %v, %v (%s)", apps, err, fmt.Sprint(a, b, c))
	}
}
