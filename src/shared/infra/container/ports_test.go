package container

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/cmd/http/httptestutil"
	ingressout "github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// Contratos de la etapa de contratos de las fases 2 y 3: catalog → ingress
// (la red del stage), catalog → deploy (docker stack rm), admin → ingress
// (los ajustes del proxy) y el arranque.

func fakeDocker(t *testing.T, c *Container) *dockertest.Fake {
	t.Helper()
	f, ok := c.Docker.(*dockertest.Fake)
	if !ok {
		t.Fatalf("el contenedor de prueba no usa dockertest: %T", c.Docker)
	}
	return f
}

// Crear un stage crea su red overlay attachable (ingress); borrar una app
// ejecuta docker stack rm (deploy); borrar el stage borra la red.
func TestContractStageNetworkAndAppStack(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	f := fakeDocker(t, e.c)
	ctx := context.Background()
	s := e.setup()
	const P = "/api/v1/projects/p"
	e.create(s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	e.create(s, P+"/stages", map[string]string{"slug": "tmp"})
	n, err := f.InspectNetwork(ctx, naming.Network("p", "tmp"))
	if err != nil || n.Driver != "overlay" || !n.Attachable || n.Labels[naming.LabelStage] != "tmp" {
		t.Fatalf("red del stage: %+v %v", n, err)
	}

	e.create(s, P+"/stages/tmp/apps", map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}})
	stack := naming.Stack("p", "tmp", "api")
	f.PutStackService(stack, docker.Service{Name: naming.Service("p", "tmp", "api", "api")})
	httptestutil.Expect(t, e.do("DELETE", P+"/stages/tmp/apps/api", nil, s), http.StatusNoContent)
	if !slices.Equal(f.RemovedStacks(), []string{stack}) {
		t.Fatalf("stack rm: %v", f.RemovedStacks())
	}

	httptestutil.Expect(t, e.do("DELETE", P+"/stages/tmp", nil, s), http.StatusNoContent)
	if _, err := f.InspectNetwork(ctx, naming.Network("p", "tmp")); !errors.Is(err, docker.ErrNotFound) {
		t.Fatalf("la red del stage borrado sigue: %v", err)
	}
}

// §13: con Docker caído, lo que lo necesita responde 503 docker_unavailable y
// no deja nada a medias.
func TestContractDockerDown(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	f := fakeDocker(t, e.c)
	s := e.setup()
	const P = "/api/v1/projects/p"
	e.create(s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	e.create(s, P+"/stages", map[string]string{"slug": "tmp"})
	e.create(s, P+"/stages/tmp/apps", map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}})

	f.SetUnavailable(true)
	expect(t, "crear un stage", e.do("POST", P+"/stages", map[string]string{"slug": "otro"}, s), http.StatusServiceUnavailable, "docker_unavailable")
	expect(t, "borrar una app", e.do("DELETE", P+"/stages/tmp/apps/api", nil, s), http.StatusServiceUnavailable, "docker_unavailable")
	if n := e.queryInt(`SELECT COUNT(*) FROM stages`); n != 1 {
		t.Fatalf("stages: %d", n)
	}
	if n := e.queryInt(`SELECT COUNT(*) FROM apps`); n != 1 {
		t.Fatalf("la app no debió borrarse: %d", n)
	}
}

// Una app con dominios no se borra: primero sus dominios (§9).
func TestContractAppWithDomains(t *testing.T) {
	e := api{t: t, c: newTestContainer(t)}
	s := e.setup()
	const P = "/api/v1/projects/p"
	e.create(s, "/api/v1/projects", map[string]string{"slug": "p", "name": "P"})
	e.create(s, P+"/stages", map[string]string{"slug": "tmp"})
	e.create(s, P+"/stages/tmp/apps", map[string]any{"slug": "api", "kind": "service", "spec": map[string]any{"image": "x:1", "port": 80}})
	e.exec(`INSERT INTO domains (app_id, hostname, target_service, target_port, tls_mode, created_at, updated_at)
		VALUES ((SELECT id FROM apps WHERE slug = 'api'), 'api.p.cl', 'api', 80, 'http', 0, 0)`)
	expect(t, "borrar una app con dominios", e.do("DELETE", P+"/stages/tmp/apps/api", nil, s), http.StatusConflict, "not_empty")
	if got := fakeDocker(t, e.c).RemovedStacks(); len(got) != 0 {
		t.Fatalf("no debió tocar el stack: %v", got)
	}
}

// Lo que admin guarda por PATCH /settings (y al arrancar, panel_domain) es lo
// que lee ingress: entity.ProxySettings, con el token cifrado.
func TestContractProxySettings(t *testing.T) {
	st := testSettings(t)
	st.Init.ACMEEmail = "inicial@correo.cl"
	c, err := New(context.Background(), st, testLogger(t), WithDocker(dockertest.New()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	e := api{t: t, c: c}
	s := e.setup()
	w := e.do("PATCH", "/api/v1/settings", map[string]any{
		"public_ip": "203.0.113.7", "proxy_http_port": 8080, "proxy_https_port": 8443,
		"cloudflare_token": "cf-token-1234567890abcdef", "panel_allowed_cidrs": []string{},
	}, s)
	httptestutil.Expect(t, w, http.StatusOK)

	key, err := crypto.LoadKeyFile(st.MasterKeyFile)
	if err != nil {
		t.Fatal(err)
	}
	ciph, err := crypto.New(key)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ingressout.NewProxySettingsReader(c.DB, ciph).ProxySettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := shared.ProxySettings{
		CloudflareToken: "cf-token-1234567890abcdef", PublicIP: "203.0.113.7", ACMEEmail: "inicial@correo.cl",
		PanelDomain: "panel.test", PanelAllowedCIDRs: []string{}, HTTPPort: 8080, HTTPSPort: 8443,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ingress lee:\n%+v\nesperaba\n%+v", got, want)
	}
}

// Run corre los workers de deploy e ingress y vuelve al cancelar el ctx.
func TestRun(t *testing.T) {
	c := newTestContainer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { c.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run no volvió al cancelar el ctx")
	}
}
