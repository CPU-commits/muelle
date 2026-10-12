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
					"TaskTemplate": map[string]any{"ContainerSpec": map[string]any{
						"Image":       "ghcr.io/x/api@sha256:abc",
						"Healthcheck": map[string]any{"Test": []string{"CMD", "wget", "-q"}},
					}},
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
		!s.HasHealthcheck || s.Image != "ghcr.io/x/api@sha256:abc" || s.Update == nil || s.Update.State != UpdateRollbackCompleted {
		t.Fatalf("servicio: %+v (update %+v)", s, s.Update)
	}
}
