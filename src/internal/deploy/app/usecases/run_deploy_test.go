package usecases

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/deploy/app/dto"
	"github.com/CPU-commits/muelle/src/internal/deploy/domain/entity"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// Un deploy de una app de formulario, de punta a punta (§7): las líneas de
// contrato §2.11 en orden (con la advertencia de Caddy mientras SyncRoutes no
// existe), la imagen resuelta, las refs guardadas, last_deployed_at y las
// etiquetas del deploy en el compose. El compose nunca queda en la base.
func TestRunDeployFormApp(t *testing.T) {
	w := newWorld(t)
	id := w.deploy(t, apiRef, nil, nil)
	d := w.runOK(t, w.apiID)
	if d.ID != id || d.Status != entity.StatusSucceeded || d.Error != nil || d.FinishedAt == nil {
		t.Fatalf("deploy: %+v\n%s", d, w.log(t, id))
	}
	if err := containsInOrder(w.texts(t, id),
		fmt.Sprintf("deploy #%d de auditfy/staging/api (api · github-actions)", id),
		"resolviendo imágenes",
		"  api: ghcr.io/x/api:1 → sha256:aaaaaaaaaaaa…",
		"render: 1 servicio",
		"red auditfy_staging lista",
		"docker stack deploy auditfy_staging_api",
		"Updating service auditfy_staging_api",
		"esperando convergencia (hasta 5 s)",
		"api: 1/1 réplicas corriendo",
		"advertencia: no se pudo actualizar Caddy: ",
		"succeeded en ",
	); err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"api": "ghcr.io/x/api:1@" + digestA}; !maps.Equal(d.Images, want) {
		t.Fatalf("images: %v", d.Images)
	}
	if want := map[string]string{"api": "ghcr.io/x/api:1"}; !maps.Equal(d.RequestedImages, want) || !maps.Equal(d.ConfigImages, want) {
		t.Fatalf("requested %v config %v", d.RequestedImages, d.ConfigImages)
	}
	app, err := w.apps.GetByID(context.Background(), w.apiID)
	if err != nil || app.LastDeployedAt == nil || !app.LastDeployedAt.Equal(*d.StartedAt) {
		t.Fatalf("last_deployed_at %v, started_at %v (%v)", app.LastDeployedAt, d.StartedAt, err)
	}
	if w.ingress.ensured[0] != "auditfy/staging" || w.ingress.synced != 1 {
		t.Fatalf("ingress: %+v", w.ingress)
	}

	deploys := w.docker.StackDeploys()
	if len(deploys) != 1 || deploys[0].Name != "auditfy_staging_api" {
		t.Fatalf("StackDeploys: %v", deploys)
	}
	svc, err := composeService(deploys[0], "api")
	if err != nil {
		t.Fatal(err)
	}
	dep := svc["deploy"].(map[string]any)
	if dep["labels"].(map[string]any)[naming.LabelDeploy] != strconv.FormatInt(id, 10) {
		t.Fatalf("deploy.labels: %v", dep["labels"])
	}
	if _, ok := svc["labels"].(map[string]any)[naming.LabelDeploy]; ok {
		t.Fatalf("muelle.deploy en las etiquetas de los contenedores: %v", svc["labels"])
	}
	noComposeInDB(t, w, `"update_config"`)
}

// noComposeInDB verifica que ninguna columna de ninguna tabla contiene marker
// (algo que solo tiene el compose final, §12: el compose nunca se guarda).
func noComposeInDB(t *testing.T, w *world, marker string) {
	t.Helper()
	ctx := context.Background()
	rows, err := w.db.Conn(ctx).QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, n)
	}
	rows.Close()
	for _, table := range tables {
		rows, err := w.db.Conn(ctx).QueryContext(ctx, `SELECT * FROM "`+table+`"`)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			for i, v := range vals {
				var s string
				switch v := v.(type) {
				case string:
					s = v
				case []byte:
					s = string(v)
				}
				if strings.Contains(s, marker) {
					t.Fatalf("la columna %s.%s tiene el compose: %.200s", table, cols[i], s)
				}
			}
		}
		rows.Close()
	}
}

// Los secretos nunca llegan al log ni a deploys.error: tampoco en la salida
// del CLI ni en su error. Un secreto de varias líneas cortas se reconoce
// entero, porque la salida se enmascara antes de partirla en líneas.
func TestRunDeployMasks(t *testing.T) {
	const secret = "s3cr3t-valor-largo"
	w := newWorld(t)
	w.env.set(w.apiID, map[string]string{"TOKEN": secret}, secret)
	w.sim.set(simOpts{Fail: fmt.Errorf("no se pudo usar %s", secret)})
	w.deploy(t, apiRef, nil, nil)
	d, err := w.claimAndRun(t, w.apiID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "docker stack deploy falló")
	if l := w.log(t, d.ID); strings.Contains(l, secret) || !strings.Contains(l, "no se pudo usar ***") {
		t.Fatalf("log:\n%s", l)
	}
	if strings.Contains(*d.Error, secret) || !strings.Contains(*d.Error, "***") {
		t.Fatalf("error: %s", *d.Error)
	}

	// Un secreto de 12 bytes con líneas de menos de 8: solo se enmascara entero.
	const multi = "abc\ndefg\nhij"
	w.env.set(w.apiID, nil, multi)
	w.sim.set(simOpts{Fail: errors.New("valor:\n" + multi + "\nfin")})
	w.deploy(t, apiRef, nil, nil)
	d, err = w.claimAndRun(t, w.apiID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "docker stack deploy falló")
	if l := w.log(t, d.ID); strings.Contains(l, "defg") || !strings.Contains(l, "***") {
		t.Fatalf("log:\n%s", l)
	}
	if strings.Contains(*d.Error, "defg") {
		t.Fatalf("error: %s", *d.Error)
	}
}

// El margen de un servicio que no cambió se cuenta desde que docker stack
// deploy volvió, no desde antes de llamarlo (decisión 6): con un CLI lento y
// un servicio cambiado que Swarm todavía no empezó a actualizar, el deploy no
// se da por convergido antes de tiempo.
func TestRunDeploySlowCLI(t *testing.T) {
	w := newWorld(t)
	w.deploy(t, apiRef, nil, nil)
	w.runOK(t, w.apiID)
	w.sim.age(time.Minute)

	w.docker.SetDigest("ghcr.io/x/api:2", digestB)
	w.sim.set(simOpts{Delay: 2 * testTiming.Settle, NoUpdate: true})
	w.deploy(t, apiRef, new("ghcr.io/x/api:2"), nil)
	d := w.runOK(t, w.apiID)
	if d.Status != entity.StatusSucceeded {
		t.Fatalf("%s\n%s", d.Status, w.log(t, d.ID))
	}
	returns := w.sim.returns()
	t1 := returns[len(returns)-1].Truncate(time.Millisecond)
	if d.FinishedAt.Before(t1.Add(testTiming.Settle)) {
		t.Fatalf("cerró %v, antes de %v + el margen (%v)", d.FinishedAt, t1, testTiming.Settle)
	}
}

// Los errores al resolver una imagen (§7 paso 2): no existe, sin acceso y
// Docker caído, con una ref con tag y con una ref con digest, con los mismos
// mensajes. Swarm no se toca y images siguen siendo los overrides.
func TestRunDeployImageErrors(t *testing.T) {
	pinned := "ghcr.io/x/api@" + digestB
	for _, ref := range []string{"ghcr.io/x/api:9", pinned} {
		cases := []struct {
			name  string
			setup func(w *world)
			want  string
		}{
			{"no existe", func(w *world) {}, "la imagen " + ref + " no existe en el registry"},
			{"sin acceso", func(w *world) { w.docker.DenyImage(ref) },
				"sin acceso a " + ref + ": revisa la credencial de ghcr.io en Ajustes → Registries"},
			{"Docker caído", func(w *world) {
				w.docker.FailNext("DistributionDigest", fmt.Errorf("caído: %w", docker.ErrUnavailable))
			}, "Docker no responde"},
		}
		for _, c := range cases {
			t.Run(c.name+" "+ref, func(t *testing.T) {
				w := newWorld(t)
				c.setup(w)
				w.deploy(t, apiRef, new(ref), nil)
				d, err := w.claimAndRun(t, w.apiID)
				if err != nil {
					t.Fatal(err)
				}
				expectFailed(t, w, d, c.want)
				if *d.Error != c.want {
					t.Fatalf("error %q, esperaba %q", *d.Error, c.want)
				}
				if n := len(w.docker.StackDeploys()); n != 0 {
					t.Fatalf("docker stack deploy se llamó %d veces", n)
				}
				if want := map[string]string{"api": ref}; !maps.Equal(d.Images, want) {
					t.Fatalf("images: %v", d.Images)
				}
			})
		}
	}
}

// Una ref con digest también se consulta al registry (decisión 1): con la
// credencial de su host, y se conserva; si el registry ya no la tiene, o
// devuelve otro digest, el deploy falla sin tocar Swarm.
func TestRunDeployPinnedDigestVerified(t *testing.T) {
	ref := "ghcr.io/x/api:1@" + digestA
	w := newWorld(t)
	if _, err := NewCreateRegistry(w.db, w.regs, w.clock, w.audit).Execute(sessionCtx(),
		dto.RegistryInput{Host: "ghcr.io", Username: "bot", Password: "s3cr3t-pass"}); err != nil {
		t.Fatal(err)
	}
	w.docker.SetDigest(ref, digestA)
	w.deploy(t, apiRef, new(ref), nil)
	d := w.runOK(t, w.apiID)
	if d.Status != entity.StatusSucceeded || d.Images["api"] != ref {
		t.Fatalf("%s %v\n%s", d.Status, d.Images, w.log(t, d.ID))
	}
	calls := w.docker.DistributionCalls()
	if len(calls) != 1 || calls[0].Ref != ref || calls[0].Auth == nil || calls[0].Auth.Host != "ghcr.io" || calls[0].Auth.Password != "s3cr3t-pass" {
		t.Fatalf("DistributionCalls: %+v", calls)
	}

	gone := "ghcr.io/x/api:1@" + digestC
	w.deploy(t, apiRef, new(gone), nil)
	d, _ = w.claimAndRun(t, w.apiID)
	expectFailed(t, w, d, "la imagen "+gone+" no existe en el registry")

	other := "ghcr.io/x/api:1@" + digestB
	w.docker.SetDigest(other, digestC)
	w.deploy(t, apiRef, new(other), nil)
	d, _ = w.claimAndRun(t, w.apiID)
	expectFailed(t, w, d, "el registry devolvió otro digest para "+other)
	if n := len(w.docker.StackDeploys()); n != 1 {
		t.Fatalf("docker stack deploy se llamó %d veces", n)
	}
}

// Si la app se empezó a borrar mientras corrían los pasos 2 a 4, el deploy
// cierra antes de tocar Swarm (decisión 8).
func TestRunDeployStopsIfRemoving(t *testing.T) {
	w := newWorld(t)
	w.ingress.onEnsure = func() { w.queue.setRemoving(w.apiID) }
	w.deploy(t, apiRef, nil, nil)
	d, err := w.claimAndRun(t, w.apiID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "la app se borró durante el deploy")
	if n := len(w.docker.StackDeploys()); n != 0 {
		t.Fatalf("docker stack deploy se llamó %d veces", n)
	}
}

// Cada deploy guarda manager_node_id si falta (contrato §1.2); en una app sin
// volúmenes, no poder leerlo es solo una advertencia.
func TestRunDeployStoresManagerNode(t *testing.T) {
	w := newWorld(t)
	w.deploy(t, apiRef, nil, nil)
	d := w.runOK(t, w.apiID)
	if d.Status != entity.StatusSucceeded || settingValue(t, w, shared.SettingManagerNodeID) != "node-1" {
		t.Fatalf("%s, manager_node_id %q", d.Status, settingValue(t, w, shared.SettingManagerNodeID))
	}

	w2 := newWorld(t)
	w2.docker.FailNext("Info", fmt.Errorf("caído: %w", docker.ErrUnavailable))
	w2.deploy(t, apiRef, nil, nil)
	d = w2.runOK(t, w2.apiID)
	if d.Status != entity.StatusSucceeded || settingValue(t, w2, shared.SettingManagerNodeID) != "" {
		t.Fatalf("%s\n%s", d.Status, w2.log(t, d.ID))
	}
	if !slices.Contains(w2.texts(t, d.ID), "advertencia: no se pudo guardar el nodo del manager: Docker no responde") {
		t.Fatalf("falta la advertencia:\n%s", w2.log(t, d.ID))
	}
}

// Las credenciales van por host exacto (contrato §2.10): la de ghcr.io para
// ghcr.io/x, anónimo para nginx (docker.io), y la de registry.example.com:5000
// no sirve para registry.example.com/x. docker stack deploy recibe solo las de
// los hosts del compose.
func TestRunDeployCredentialsByExactHost(t *testing.T) {
	w := newWorld(t)
	create := NewCreateRegistry(w.db, w.regs, w.clock, w.audit)
	for i, host := range []string{"ghcr.io", "registry.example.com:5000", "quay.io"} {
		if _, err := create.Execute(sessionCtx(), dto.RegistryInput{Host: host, Username: fmt.Sprintf("bot%d", i), Password: "s3cr3t-" + host}); err != nil {
			t.Fatal(err)
		}
	}
	exec(t, w.db, `INSERT INTO apps (stage_id, slug, kind, compose_yaml, deploy_timeout_s, config_changed_at, created_at)
		SELECT id, 'mix', 'compose', ?, 5, 0, 0 FROM stages`, `services:
  web:
    image: ghcr.io/x/web:1
  side:
    image: nginx
  reg:
    image: registry.example.com/x:1
`)
	w.docker.SetDigest("registry.example.com/x:1", digestB)
	ref := dto.AppRef{Project: "auditfy", Stage: "staging", App: "mix"}
	w.deploy(t, ref, nil, nil)
	app, _ := w.apps.Get(context.Background(), "auditfy", "staging", "mix")
	d := w.runOK(t, app.ID)
	if d.Status != entity.StatusSucceeded {
		t.Fatalf("%s\n%s", d.Status, w.log(t, d.ID))
	}
	auth := map[string]string{}
	for _, c := range w.docker.DistributionCalls() {
		auth[c.Ref] = "anónimo"
		if c.Auth != nil {
			auth[c.Ref] = c.Auth.Host
		}
	}
	want := map[string]string{"ghcr.io/x/web:1": "ghcr.io", "nginx": "anónimo", "registry.example.com/x:1": "anónimo"}
	if !maps.Equal(auth, want) {
		t.Fatalf("credenciales al resolver: %v", auth)
	}
	sd := w.docker.StackDeploys()
	if len(sd) != 1 || len(sd[0].Registries) != 1 || sd[0].Registries[0].Host != "ghcr.io" || sd[0].Registries[0].Password != "s3cr3t-ghcr.io" {
		t.Fatalf("credenciales de docker stack deploy: %+v", sd)
	}
}

// Un error de render (una variable sin definir) o de la red falla el deploy
// sin tocar Swarm.
func TestRunDeployRenderAndNetworkErrors(t *testing.T) {
	w := newWorld(t)
	w.env.set(w.infraID, nil) // sin TAG
	w.deploy(t, infraRef, nil, nil)
	d, err := w.claimAndRun(t, w.infraID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "render: compose_yaml: ")
	if !strings.Contains(*d.Error, "TAG") || len(w.docker.DistributionCalls()) != 0 {
		t.Fatalf("error %q, %d consultas al registry", *d.Error, len(w.docker.DistributionCalls()))
	}

	w.ingress.ensureErr = errors.New("la red no se pudo crear")
	w.deploy(t, apiRef, nil, nil)
	d, _ = w.claimAndRun(t, w.apiID)
	expectFailed(t, w, d, "la red no se pudo crear")
	if n := len(w.docker.StackDeploys()); n != 0 {
		t.Fatalf("docker stack deploy se llamó %d veces", n)
	}
}

// Si docker stack deploy falla, el deploy falla con su salida en el log.
func TestRunDeployStackError(t *testing.T) {
	w := newWorld(t)
	w.docker.FailNext("DeployStack", errors.New("network auditfy_staging not found"))
	w.deploy(t, apiRef, nil, nil)
	d, err := w.claimAndRun(t, w.apiID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "docker stack deploy falló")
	if !slices.Contains(w.texts(t, d.ID), "failed to deploy stack: network auditfy_staging not found") {
		t.Fatalf("falta la salida del CLI:\n%s", w.log(t, d.ID))
	}
}

// Un rollback de Swarm (§7 paso 6): failed con el Err de la última tarea
// fallida y exactamente sus últimas 50 líneas de log.
func TestRunDeployRollbackDetected(t *testing.T) {
	w := newWorld(t)
	w.deploy(t, apiRef, nil, nil)
	w.runOK(t, w.apiID)
	w.sim.age(time.Minute)

	w.docker.SetDigest("ghcr.io/x/api:2", digestB)
	w.sim.set(simOpts{State: docker.UpdateRollbackCompleted, Message: "rollback completed", Running: new(uint64(0)),
		FailedErr: "task: non-zero exit (1)", FailedLogs: 60})
	w.deploy(t, apiRef, new("ghcr.io/x/api:2"), nil)
	d, err := w.claimAndRun(t, w.apiID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "api: Swarm hizo rollback (rollback_completed): rollback completed")
	texts := w.texts(t, d.ID)
	if !slices.Contains(texts, "api: última tarea fallida: task: non-zero exit (1)") {
		t.Fatalf("falta el Err de la tarea:\n%s", strings.Join(texts, "\n"))
	}
	var logs []string
	for _, l := range texts {
		if strings.HasPrefix(l, "    | ") {
			logs = append(logs, strings.TrimPrefix(l, "    | "))
		}
	}
	if len(logs) != 50 || logs[0] != "línea 10" || logs[49] != "línea 59" {
		t.Fatalf("las últimas 50 líneas: %d %v", len(logs), logs)
	}
}

// La tarea que hizo fallar la actualización es la que nombra Swarm en el
// Message ("…early termination of task <id>"): una que no pasó el
// healthcheck termina complete y sin Err. Una tarea rechazada de antes de
// este deploy no cuenta.
func TestRunDeployFailedTaskOfThisDeploy(t *testing.T) {
	w := newWorld(t)
	w.deploy(t, apiRef, nil, nil)
	w.runOK(t, w.apiID)
	w.sim.age(time.Minute)
	ctx := context.Background()
	tasks, _ := w.docker.ListTasks(ctx, docker.TaskFilter{Service: "auditfy_staging_api_api"})
	old := docker.Task{ID: "vieja", Slot: 1, State: docker.TaskRejected, DesiredState: docker.TaskShutdown,
		Err: "invalid pool request", CreatedAt: time.Now().Add(-30 * time.Second)}
	w.docker.SetTasks("auditfy_staging_api_api", append(tasks, old)...)

	w.docker.SetDigest("ghcr.io/x/api:2", digestB)
	w.sim.set(simOpts{State: docker.UpdateRollbackStarted, Message: "update rolled back due to failure or early termination of task",
		TaskInMessage: true, FailedState: docker.TaskComplete, FailedLogs: 3})
	w.deploy(t, apiRef, new("ghcr.io/x/api:2"), nil)
	d, err := w.claimAndRun(t, w.apiID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "api: Swarm hizo rollback (rollback_started): update rolled back due to failure or early termination of task")
	texts := w.texts(t, d.ID)
	if err := containsInOrder(texts, "api: última tarea fallida: terminó en complete, sin un motivo de Swarm",
		"    | línea 0", "    | línea 2", "failed: "); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.log(t, d.ID), "invalid pool request") {
		t.Fatalf("tomó una tarea de antes del deploy:\n%s", w.log(t, d.ID))
	}
}

// Si no converge en deploy_timeout_s: failed con el estado de cada servicio
// que no llegó y el Err de su última tarea fallida.
func TestRunDeployTimeout(t *testing.T) {
	w := newWorld(t)
	exec(t, w.db, `UPDATE apps SET deploy_timeout_s = 1 WHERE id = ?`, w.apiID)
	w.sim.set(simOpts{Running: new(uint64(0)), FailedErr: "no hay memoria"})
	w.deploy(t, apiRef, nil, nil)
	start := time.Now()
	d, err := w.claimAndRun(t, w.apiID)
	if err != nil {
		t.Fatal(err)
	}
	expectFailed(t, w, d, "timeout esperando convergencia")
	if time.Since(start) < time.Second {
		t.Fatalf("cerró antes del timeout: %v", time.Since(start))
	}
	if err := containsInOrder(w.texts(t, d.ID), "esperando convergencia (hasta 1 s)", "api: 0/1 réplicas corriendo",
		"api: 0/1 réplicas corriendo", "api: última tarea fallida: no hay memoria", "failed: timeout esperando convergencia"); err != nil {
		t.Fatal(err)
	}
}

// Un redeploy sin cambios no reinicia nada (decisión 6): Swarm no deja un
// UpdateStatus nuevo, las tareas son las mismas, y el deploy converge pasado
// el margen.
func TestRunDeployUnchangedRedeploy(t *testing.T) {
	w := newWorld(t)
	w.deploy(t, apiRef, nil, nil)
	w.runOK(t, w.apiID)
	w.sim.age(time.Minute)
	ctx := context.Background()
	before, _ := w.docker.ListTasks(ctx, docker.TaskFilter{Service: "auditfy_staging_api_api"})

	w.deploy(t, apiRef, nil, nil)
	d := w.runOK(t, w.apiID)
	if d.Status != entity.StatusSucceeded {
		t.Fatalf("%s\n%s", d.Status, w.log(t, d.ID))
	}
	if took := d.FinishedAt.Sub(*d.StartedAt); took < testTiming.Settle {
		t.Fatalf("convergió en %v, antes del margen", took)
	}
	svc, _ := w.docker.InspectService(ctx, "auditfy_staging_api_api")
	after, _ := w.docker.ListTasks(ctx, docker.TaskFilter{Service: "auditfy_staging_api_api"})
	if svc.Update != nil || len(after) != len(before) || after[0].ID != before[0].ID {
		t.Fatalf("Update %+v, tareas %v → %v", svc.Update, before, after)
	}
	if svc.Labels[naming.LabelDeploy] != strconv.FormatInt(d.ID, 10) {
		t.Fatalf("etiquetas: %v", svc.Labels)
	}
}

// Una app de formulario con volúmenes queda en el nodo del manager (§6.2):
// storage_node_id y la restricción node.id en el compose.
func TestRunDeployStorageNode(t *testing.T) {
	w := newWorld(t)
	exec(t, w.db, `INSERT INTO settings (key, value, encrypted) VALUES ('manager_node_id', CAST('mgr1' AS BLOB), 0)`)
	exec(t, w.db, `UPDATE apps SET spec = '{"image":"ghcr.io/x/api:1","port":3000,"volumes":[{"name":"data","mount":"/data"}]}' WHERE id = ?`, w.apiID)
	w.deploy(t, apiRef, nil, nil)
	d := w.runOK(t, w.apiID)
	if d.Status != entity.StatusSucceeded {
		t.Fatalf("%s\n%s", d.Status, w.log(t, d.ID))
	}
	app, _ := w.apps.GetByID(context.Background(), w.apiID)
	if app.StorageNodeID != "mgr1" {
		t.Fatalf("storage_node_id %q", app.StorageNodeID)
	}
	svc, err := composeService(w.docker.StackDeploys()[0], "api")
	if err != nil {
		t.Fatal(err)
	}
	cons := svc["deploy"].(map[string]any)["placement"].(map[string]any)["constraints"].([]any)
	if len(cons) != 1 || cons[0] != "node.id == mgr1" {
		t.Fatalf("restricciones: %v", cons)
	}

	// Sin manager_node_id guardado, el del nodo local.
	w2 := newWorld(t)
	exec(t, w2.db, `UPDATE apps SET spec = '{"image":"ghcr.io/x/api:1","port":3000,"volumes":[{"name":"data","mount":"/data"}]}' WHERE id = ?`, w2.apiID)
	w2.deploy(t, apiRef, nil, nil)
	w2.runOK(t, w2.apiID)
	app, _ = w2.apps.GetByID(context.Background(), w2.apiID)
	if app.StorageNodeID != "node-1" || settingValue(t, w2, shared.SettingManagerNodeID) != "node-1" {
		t.Fatalf("storage_node_id %q", app.StorageNodeID)
	}
}

// El rollback de punta a punta (contrato §2.2 y §2.7): vuelve al digest del
// deploy anterior, que el paso 2 verifica en el registry (si ya no está,
// falla sin tocar Swarm), y un deploy sin cuerpo después no lo deshace,
// aunque el tag ahora resuelva a otro digest.
func TestRollbackFlow(t *testing.T) {
	w := newWorld(t)
	exec(t, w.db, `UPDATE apps SET spec = '{"image":"ghcr.io/x/api:staging","port":3000}' WHERE id = ?`, w.apiID)
	w.docker.SetDigest("ghcr.io/x/api:staging", digestA)
	w.deploy(t, apiRef, nil, nil)
	d1 := w.runOK(t, w.apiID)
	w.sim.age(time.Minute)
	w.docker.SetDigest("ghcr.io/x/api:staging", digestB)
	w.deploy(t, apiRef, nil, nil)
	d2 := w.runOK(t, w.apiID)
	if d1.Status != entity.StatusSucceeded || d2.Status != entity.StatusSucceeded || d2.Images["api"] != "ghcr.io/x/api:staging@"+digestB {
		t.Fatalf("d1 %s %v, d2 %s %v", d1.Status, d1.Images, d2.Status, d2.Images)
	}
	pinned := "ghcr.io/x/api:staging@" + digestA

	// El registry ya no tiene A.
	rollback := NewRollback(w.deps)
	if _, err := rollback.Execute(tokenCtx(), apiRef); err != nil {
		t.Fatal(err)
	}
	r1, _ := w.claimAndRun(t, w.apiID)
	expectFailed(t, w, r1, "la imagen "+pinned+" no existe en el registry")
	if n := len(w.docker.StackDeploys()); n != 2 {
		t.Fatalf("docker stack deploy se llamó %d veces", n)
	}

	// Vuelve a estar: el destino sigue siendo d1 (r1 no cuenta).
	w.docker.SetDigest(pinned, digestA)
	w.sim.age(time.Minute)
	if _, err := rollback.Execute(tokenCtx(), apiRef); err != nil {
		t.Fatal(err)
	}
	r2 := w.runOK(t, w.apiID)
	if r2.Status != entity.StatusSucceeded || r2.Images["api"] != pinned || r2.Trigger != entity.TriggerRollback {
		t.Fatalf("r2: %s %v\n%s", r2.Status, r2.Images, w.log(t, r2.ID))
	}
	if !slices.ContainsFunc(w.docker.DistributionCalls(), func(c dockertest.DistributionCall) bool { return c.Ref == pinned }) {
		t.Fatalf("no consultó %s: %+v", pinned, w.docker.DistributionCalls())
	}

	// Un deploy sin cuerpo repite el digest del rollback, aunque el tag se movió.
	w.docker.SetDigest("ghcr.io/x/api:staging", digestC)
	w.sim.age(time.Minute)
	w.deploy(t, apiRef, nil, nil)
	d3 := w.runOK(t, w.apiID)
	if d3.Status != entity.StatusSucceeded || d3.Images["api"] != pinned || d3.RequestedImages["api"] != pinned {
		t.Fatalf("d3: %s %v %v", d3.Status, d3.Images, d3.RequestedImages)
	}
}
