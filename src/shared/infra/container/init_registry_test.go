package container

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/CPU-commits/muelle/src/cmd/cron"
	"github.com/CPU-commits/muelle/src/internal/ingress"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/caddy/caddytest"
	"github.com/CPU-commits/muelle/src/settings"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker"
	"github.com/CPU-commits/muelle/src/shared/infra/out/docker/dockertest"
)

// pullAuths es el Docker en memoria que, además, guarda la credencial con
// que se pidió cada PullImage (dockertest solo guarda la ref).
type pullAuths struct {
	*dockertest.Fake
	mu    sync.Mutex
	auths map[string]*docker.RegistryAuth
}

func (p *pullAuths) PullImage(ctx context.Context, ref string, auth *docker.RegistryAuth) error {
	p.mu.Lock()
	if p.auths == nil {
		p.auths = map[string]*docker.RegistryAuth{}
	}
	if auth != nil {
		a := *auth
		auth = &a
	}
	p.auths[ref] = auth
	p.mu.Unlock()
	return p.Fake.PullImage(ctx, ref, auth)
}

func (p *pullAuths) auth(ref string) (*docker.RegistryAuth, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	a, ok := p.auths[ref]
	return a, ok
}

// Fase 5 → 2 → 3: el registry que install le pasa a muelle
// (MUELLE_INIT_REGISTRY_FILE) lo guarda deploy, cifrado, en el primer
// arranque, y con esa credencial ingress baja la imagen de muelle-caddy de
// esa versión, que es privada (contrato §2.10 y §3.4).
func TestContractInitRegistryPullsCaddyImage(t *testing.T) {
	const password = "ghp_s3cr3t-de-install"
	s := testSettings(t)
	s.Version = "v0.1.0"
	s.Init.Registry = &settings.InitRegistry{Host: "ghcr.io", Username: "bot", Password: password}
	f := &pullAuths{Fake: dockertest.New()}
	admin := caddytest.New(t)
	ctx := context.Background()
	c, err := New(ctx, s, testLogger(t), WithDocker(f), WithIngress(ingress.Options{CaddyAdminDial: admin.Dial}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })

	i := slices.IndexFunc(c.Jobs, func(j cron.Job) bool { return j.Name == "proxy" })
	if i < 0 {
		t.Fatal(`falta el job "proxy" en c.Jobs`)
	}
	if err := c.Jobs[i].Run(ctx); err != nil {
		t.Fatalf("job proxy: %v", err)
	}
	image := naming.CaddyImageFor(s.Version)
	auth, pulled := f.auth(image)
	if !pulled {
		t.Fatalf("no bajó %s: %v", image, f.Pulls())
	}
	if auth == nil || auth.Host != "ghcr.io" || auth.Username != "bot" || auth.Password != password {
		t.Fatalf("bajó %s con %+v, esperaba la credencial de install", image, auth)
	}
	if got, err := f.InspectContainer(ctx, naming.CaddyContainer); err != nil || got.Image != image || !got.Running {
		t.Fatalf("muelle-caddy: %+v %v", got, err)
	}
}
