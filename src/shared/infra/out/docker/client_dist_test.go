package docker

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"testing"

	"github.com/moby/moby/api/types/swarm"
)

// ForceUpdateService es docker service update --force: lee el servicio, sube
// TaskTemplate.ForceUpdate y manda el mismo spec con la versión que leyó.
func TestForceUpdateService(t *testing.T) {
	labels := map[string]string{"com.docker.stack.namespace": "muelle"}
	current := swarm.Service{
		ID:   "svc1",
		Meta: swarm.Meta{Version: swarm.Version{Index: 7}},
		Spec: swarm.ServiceSpec{
			Annotations: swarm.Annotations{Name: "muelle_muelle", Labels: labels},
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: &swarm.ContainerSpec{Image: "ghcr.io/cpu-commits/muelle:v0.1.0"},
				ForceUpdate:   2,
			},
		},
	}
	var sent swarm.ServiceSpec
	c, f := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /services/muelle_muelle": func(w http.ResponseWriter, r *http.Request) { apiJSON(w, current) },
		"POST /services/svc1/update": func(w http.ResponseWriter, r *http.Request) {
			if err := json.NewDecoder(r.Body).Decode(&sent); err != nil {
				apiError(w, http.StatusBadRequest, err.Error())
				return
			}
			apiJSON(w, map[string]any{})
		},
	})
	if err := c.ForceUpdateService(context.Background(), "muelle_muelle"); err != nil {
		t.Fatal(err)
	}
	r := f.last(t)
	if r.Method != http.MethodPost {
		t.Fatalf("la última petición es %s %s", r.Method, r.URL.Path)
	}
	q := r.URL.Query()
	if q.Get("version") != "7" {
		t.Errorf("version=%q, esperaba 7 (la que leyó)", q.Get("version"))
	}
	// Las credenciales del registry son las que ya tiene el servicio (las de
	// --with-registry-auth de install): sin esto, Swarm no podría bajar una
	// imagen privada al recrear la tarea.
	if q.Get("registryAuthFrom") != string(swarm.RegistryAuthFromSpec) {
		t.Errorf("registryAuthFrom=%q, esperaba spec", q.Get("registryAuthFrom"))
	}
	if sent.TaskTemplate.ForceUpdate != 3 {
		t.Errorf("ForceUpdate=%d, esperaba 3", sent.TaskTemplate.ForceUpdate)
	}
	if sent.Name != "muelle_muelle" || !maps.Equal(sent.Labels, labels) ||
		sent.TaskTemplate.ContainerSpec == nil || sent.TaskTemplate.ContainerSpec.Image != "ghcr.io/cpu-commits/muelle:v0.1.0" {
		t.Errorf("el spec cambió: %+v", sent)
	}
}

// Un servicio que no existe es ErrNotFound, y no se intenta el update.
func TestForceUpdateServiceNotFound(t *testing.T) {
	c, f := newFakeAPIClient(t, map[string]func(http.ResponseWriter, *http.Request){
		"GET /services/no-existe": func(w http.ResponseWriter, r *http.Request) {
			apiError(w, http.StatusNotFound, "service no-existe not found")
		},
	})
	err := c.ForceUpdateService(context.Background(), "no-existe")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err=%v, esperaba ErrNotFound", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.requests {
		if r.Method == http.MethodPost {
			t.Errorf("hubo un %s %s", r.Method, r.URL.Path)
		}
	}
}
