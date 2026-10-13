package docker

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/moby/moby/api/pkg/authconfig"

	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// fakeAPI es un daemon de Docker de mentira por HTTP: responde lo que diga
// routes ("MÉTODO /ruta" sin la versión) y guarda las peticiones.
type fakeAPI struct {
	mu       sync.Mutex
	requests []*http.Request
	routes   map[string]func(w http.ResponseWriter, r *http.Request)
}

var versionPrefix = regexp.MustCompile(`^/v[0-9.]+`)

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := versionPrefix.ReplaceAllString(r.URL.Path, "")
	if path == "/_ping" {
		w.Header().Set("Api-Version", "1.55")
		w.Write([]byte("OK"))
		return
	}
	f.mu.Lock()
	f.requests = append(f.requests, r.Clone(context.Background()))
	f.mu.Unlock()
	if h, ok := f.routes[r.Method+" "+path]; ok {
		h(w, r)
		return
	}
	apiError(w, http.StatusNotImplemented, "ruta no simulada: "+r.Method+" "+path)
}

func (f *fakeAPI) last(t *testing.T) *http.Request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.requests) == 0 {
		t.Fatal("no llegó ninguna petición")
	}
	return f.requests[len(f.requests)-1]
}

func apiError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

func apiJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func newFakeAPIClient(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) (*Client, *fakeAPI) {
	t.Helper()
	f := &fakeAPI{routes: routes}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(Options{Host: "tcp://" + strings.TrimPrefix(srv.URL, "http://")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c, f
}

func TestDistributionDigestSendsRegistryAuth(t *testing.T) {
	const digest = "sha256:" + "ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12ab12"
	c, f := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /distribution/ghcr.io/x/api:sha-1/json": func(w http.ResponseWriter, r *http.Request) {
			apiJSON(w, map[string]any{"Descriptor": map[string]any{
				"mediaType": "application/vnd.oci.image.index.v1+json", "digest": digest, "size": 1,
			}})
		},
	})
	got, err := c.DistributionDigest(context.Background(), "ghcr.io/x/api:sha-1",
		&RegistryAuth{Host: "ghcr.io", Username: "bot", Password: "s3cr3t"})
	if err != nil {
		t.Fatal(err)
	}
	if got != digest {
		t.Fatalf("digest = %q", got)
	}
	ac, err := authconfig.Decode(f.last(t).Header.Get("X-Registry-Auth"))
	if err != nil || ac.Username != "bot" || ac.Password != "s3cr3t" || ac.ServerAddress != "ghcr.io" {
		t.Fatalf("X-Registry-Auth = %+v (%v)", ac, err)
	}

	// Anónimo: sin credenciales en el header.
	if _, err := c.DistributionDigest(context.Background(), "ghcr.io/x/api:sha-1", nil); err != nil {
		t.Fatal(err)
	}
	if h := f.last(t).Header.Get("X-Registry-Auth"); h != "" {
		if ac, _ := authconfig.Decode(h); ac != nil && (ac.Username != "" || ac.Password != "") {
			t.Fatalf("anónimo mandó credenciales: %+v", ac)
		}
	}
}

func TestErrorMapping(t *testing.T) {
	c, _ := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /distribution/ghcr.io/x/no:1/json": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusNotFound, "manifest unknown")
		},
		"GET /distribution/ghcr.io/x/privada:1/json": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusUnauthorized, "unauthorized")
		},
		"POST /networks/create": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusConflict, "network with name auditfy_staging already exists")
		},
		"DELETE /networks/auditfy_staging": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusForbidden, "error while removing network: network auditfy_staging has active endpoints")
		},
		// Una red overlay attachable con un contenedor conectado tiene una tarea de Swarm: el daemon responde 400.
		"DELETE /networks/auditfy_production": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusBadRequest, "rpc error: code = FailedPrecondition desc = network naidvm70mi5cd0a271hnzv0rw is in use by task derwfmwgprla2fdbq980fqug9")
		},
		// Justo después de borrarla, el nodo todavía la conoce por nombre y el cluster ya no.
		"DELETE /networks/recien_borrada": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusInternalServerError, "network rrko0zuejk03zm9yn937txnfm not found")
		},
		// En una overlay, desconectar dos veces es un 500 con este texto.
		"POST /networks/auditfy_production/disconnect": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusInternalServerError, "container aa22cfaa5766 is not connected to network auditfy_production")
		},
		"DELETE /networks/mala": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusBadRequest, "invalid network name")
		},
		"POST /networks/auditfy_staging/connect": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusForbidden, "endpoint with name muelle-caddy already exists in network auditfy_staging")
		},
		"POST /networks/auditfy_staging/disconnect": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusForbidden, "container muelle-caddy is not connected to network auditfy_staging")
		},
		"GET /containers/muelle-caddy/json": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusNotFound, "No such container: muelle-caddy")
		},
		"GET /info": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusInternalServerError, "algo raro")
		},
	})
	ctx := context.Background()
	_, err := c.DistributionDigest(ctx, "ghcr.io/x/no:1", nil)
	check(t, "imagen inexistente", err, ErrNotFound)
	_, err = c.DistributionDigest(ctx, "ghcr.io/x/privada:1", nil)
	check(t, "imagen sin permiso", err, ErrDenied)
	_, err = c.CreateNetwork(ctx, NetworkSpec{Name: "auditfy_staging", Driver: "overlay"})
	check(t, "red repetida", err, ErrConflict)
	check(t, "red en uso", c.RemoveNetwork(ctx, "auditfy_staging"), ErrConflict)
	check(t, "red de swarm en uso por una tarea", c.RemoveNetwork(ctx, "auditfy_production"), ErrConflict)
	check(t, "red recién borrada", c.RemoveNetwork(ctx, "recien_borrada"), ErrNotFound)
	check(t, "overlay: no conectado", c.DisconnectNetwork(ctx, "auditfy_production", "muelle-caddy"), ErrNotFound)
	if err := c.RemoveNetwork(ctx, "mala"); err == nil || errors.Is(err, ErrConflict) {
		t.Errorf("un 400 que no es \"en uso\" no es ErrConflict: %v", err)
	}
	check(t, "ya conectado", c.ConnectNetwork(ctx, "auditfy_staging", "muelle-caddy", nil), ErrConflict)
	check(t, "no conectado", c.DisconnectNetwork(ctx, "auditfy_staging", "muelle-caddy"), ErrNotFound)
	_, err = c.InspectContainer(ctx, "muelle-caddy")
	check(t, "contenedor inexistente", err, ErrNotFound)

	// Un error que no es de ninguno de los tipos no se marca como tal.
	_, err = c.Info(ctx)
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrConflict) || errors.Is(err, ErrUnavailable) {
		t.Fatalf("error interno del daemon: %v", err)
	}
}

func check(t *testing.T, what string, err, want error) {
	t.Helper()
	if !errors.Is(err, want) {
		t.Errorf("%s: %v, esperaba %v", what, err, want)
	}
}

// Sin daemon, todo da ErrUnavailable, que es el error compartido que cmd/http
// responde con 503.
func TestUnavailable(t *testing.T) {
	c, err := New(Options{Host: "unix://" + t.TempDir() + "/no-hay-docker.sock"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	err = c.Ping(context.Background())
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, entity.ErrDockerUnavailable) {
		t.Fatalf("Ping sin daemon: %v", err)
	}
	if _, err := c.ListServices(context.Background(), nil); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("ListServices sin daemon: %v", err)
	}
}

func TestListServicesFiltersByLabel(t *testing.T) {
	c, f := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /services": func(w http.ResponseWriter, r *http.Request) {
			apiJSON(w, []map[string]any{{
				"ID": "s1",
				"Spec": map[string]any{
					"Name":   "auditfy_staging_api_api",
					"Labels": map[string]string{"muelle.app": "api"},
					"Mode":   map[string]any{"Replicated": map[string]any{"Replicas": 2}},
					"TaskTemplate": map[string]any{
						"ContainerSpec": map[string]any{
							"Image":       "ghcr.io/x/api@sha256:abc",
							"Healthcheck": map[string]any{"Test": []string{"CMD", "wget", "-q"}},
						},
						"Resources": map[string]any{"Limits": map[string]any{"MemoryBytes": 512 << 20, "NanoCPUs": 1500000000}},
					},
				},
				"ServiceStatus": map[string]any{"RunningTasks": 1, "DesiredTasks": 2},
				"UpdateStatus":  map[string]any{"State": "rollback_completed", "Message": "rollback completed"},
			}})
		},
	})
	got, err := c.ListServices(context.Background(), map[string]string{"muelle.app": "api", "muelle.stage": "staging"})
	if err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(f.last(t).URL.RawQuery)
	var filters map[string]map[string]bool
	if err := json.Unmarshal([]byte(q.Get("filters")), &filters); err != nil {
		t.Fatalf("filters=%q: %v", q.Get("filters"), err)
	}
	if !filters["label"]["muelle.app=api"] || !filters["label"]["muelle.stage=staging"] || q.Get("status") != "true" {
		t.Fatalf("query: %v", q)
	}
	if len(got) != 1 {
		t.Fatalf("servicios: %+v", got)
	}
	s := got[0]
	if s.Name != "auditfy_staging_api_api" || s.Replicas == nil || *s.Replicas != 2 || s.RunningTasks != 1 || s.DesiredTasks != 2 ||
		!s.HasHealthcheck || s.Image != "ghcr.io/x/api@sha256:abc" || s.Update == nil || s.Update.State != UpdateRollbackCompleted ||
		s.MemoryLimitBytes != 512<<20 || s.NanoCPUs != 1500000000 {
		t.Fatalf("servicio: %+v (update %+v)", s, s.Update)
	}
}

// Docker filtra las tareas por las etiquetas de la tarea, no las del servicio:
// ListTasks con Labels busca primero los servicios con esas etiquetas.
func TestListTasksFiltersByServiceLabels(t *testing.T) {
	c, f := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /services": func(w http.ResponseWriter, r *http.Request) {
			apiJSON(w, []map[string]any{
				{"ID": "s1", "Spec": map[string]any{"Name": "auditfy_staging_api_api", "Labels": map[string]string{"muelle.app": "api"}}},
				{"ID": "s2", "Spec": map[string]any{"Name": "auditfy_staging_api_worker", "Labels": map[string]string{"muelle.app": "api"}}},
			})
		},
		"GET /tasks": func(w http.ResponseWriter, r *http.Request) {
			apiJSON(w, []map[string]any{
				{"ID": "t1", "ServiceID": "s1", "Slot": 1, "DesiredState": "running", "CreatedAt": "2026-10-12T10:00:00Z",
					"Status": map[string]any{"State": "running", "ContainerStatus": map[string]any{"ContainerID": "c1"}}},
				{"ID": "t2", "ServiceID": "s2", "Slot": 1, "DesiredState": "running", "CreatedAt": "2026-10-12T11:00:00Z",
					"Status": map[string]any{"State": "failed", "Err": "task: non-zero exit (1)"}},
				// Una tarea de otro servicio que el daemon no debió devolver no pasa.
				{"ID": "t3", "ServiceID": "otro", "DesiredState": "running", "Status": map[string]any{"State": "running"}},
			})
		},
	})
	got, err := c.ListTasks(context.Background(), TaskFilter{Labels: map[string]string{"muelle.app": "api"}, DesiredState: TaskRunning})
	if err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(f.last(t).URL.RawQuery)
	var filters map[string]map[string]bool
	if err := json.Unmarshal([]byte(q.Get("filters")), &filters); err != nil {
		t.Fatalf("filters=%q: %v", q.Get("filters"), err)
	}
	if !filters["service"]["s1"] || !filters["service"]["s2"] || !filters["desired-state"]["running"] || len(filters["label"]) != 0 {
		t.Fatalf("filtros de /tasks: %v", filters)
	}
	if len(got) != 2 || got[0].ID != "t2" || got[0].Err != "task: non-zero exit (1)" || got[1].ContainerID != "c1" {
		t.Fatalf("tareas (de la más nueva a la más vieja): %+v", got)
	}

	// Sin servicios con esas etiquetas no hay tareas, y no se lista todo.
	c2, f2 := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /services": func(w http.ResponseWriter, r *http.Request) { apiJSON(w, []any{}) },
	})
	got, err = c2.ListTasks(context.Background(), TaskFilter{Labels: map[string]string{"muelle.app": "nada"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("sin servicios: %+v %v", got, err)
	}
	if r := f2.last(t); !strings.HasSuffix(r.URL.Path, "/services") {
		t.Fatalf("no debió pedir /tasks: %s", r.URL.Path)
	}
}

// Un nodo que no es manager (o sin Swarm) responde 503: para muelle es lo
// mismo que Docker caído (§13).
func TestSwarmNotActiveIsUnavailable(t *testing.T) {
	c, _ := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /services": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusServiceUnavailable, "This node is not a swarm manager. Use \"docker swarm init\" or \"docker swarm join\" to connect this node to swarm and try again.")
		},
	})
	_, err := c.ListServices(context.Background(), nil)
	check(t, "sin swarm", err, ErrUnavailable)
}

// InspectImage mira el nodo local, no el registry: ingress lo usa para saber
// si tiene que bajar la imagen de muelle-caddy antes de crearlo.
func TestInspectImage(t *testing.T) {
	c, _ := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /images/ghcr.io/cpu-commits/muelle-caddy:v1/json": func(w http.ResponseWriter, r *http.Request) {
			apiJSON(w, map[string]any{
				"Id":          "sha256:111",
				"RepoTags":    []string{"ghcr.io/cpu-commits/muelle-caddy:v1"},
				"RepoDigests": []string{"ghcr.io/cpu-commits/muelle-caddy@sha256:222"},
			})
		},
		"GET /images/ghcr.io/cpu-commits/muelle-caddy:v2/json": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusNotFound, "No such image: ghcr.io/cpu-commits/muelle-caddy:v2")
		},
	})
	img, err := c.InspectImage(context.Background(), "ghcr.io/cpu-commits/muelle-caddy:v1")
	if err != nil {
		t.Fatal(err)
	}
	if img.ID != "sha256:111" || len(img.RepoTags) != 1 || img.RepoDigests[0] != "ghcr.io/cpu-commits/muelle-caddy@sha256:222" {
		t.Fatalf("imagen: %+v", img)
	}
	_, err = c.InspectImage(context.Background(), "ghcr.io/cpu-commits/muelle-caddy:v2")
	check(t, "imagen que no está en el nodo", err, ErrNotFound)
}
