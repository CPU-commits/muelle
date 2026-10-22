package usecases

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const hookPanel = "panel.auditfy.cl"

// hookFixture arma ProxySettingsChanged como el módulo, sobre el
// lifecycleFixture: muelle-caddy corriendo con 8080/8443 y la configuración
// en Caddy (synced), el Recreator corriendo con delay, y Cloudflare falso
// con el token syncToken y la zona z1 auditfy.cl (el token no está en los
// ajustes: cada test lo pone si lo necesita).
type hookFixture struct {
	*lifecycleFixture
	recreator *Recreator
	uc        *ProxySettingsChanged
}

func newHookFixture(t *testing.T, delay time.Duration) *hookFixture {
	t.Helper()
	f, r := newRecreatorFixture(t, delay)
	f.cf.AddToken(syncToken)
	f.cf.AddZone("z1", "auditfy.cl")
	records := NewUpdateManagedRecords(DomainDeps{
		Repo: f.repo, Settings: f.settings, DNS: cloudflare.New(f.cf.URL()), Ops: f.ops, Clock: f.clock, Log: f.slog,
		DomainOp: 30 * time.Second, DomainRetry: 20 * time.Second,
	})
	return &hookFixture{lifecycleFixture: f, recreator: r,
		uc: NewProxySettingsChanged(r, records, f.panel, f.syncer, nil, f.state, 2*time.Minute, f.slog)}
}

// change guarda los ajustes con mod, como admin antes de llamar al hook, y
// devuelve los de antes y los nuevos.
func (f *hookFixture) change(t *testing.T, mod func(*shared.ProxySettings)) (old, next shared.ProxySettings) {
	t.Helper()
	old, err := f.settings.ProxySettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	next = old
	next.PanelAllowedCIDRs = append([]string{}, old.PanelAllowedCIDRs...)
	mod(&next)
	f.enableCaddy(t, func(p *shared.ProxySettings) { *p = next })
	return old, next
}

// withToken deja el token y la IP pública en los ajustes, sin pasar por el
// hook, y Caddy synced con eso.
func (f *hookFixture) withToken(t *testing.T) {
	t.Helper()
	f.enableCaddy(t, func(p *shared.ProxySettings) { p.CloudflareToken, p.PublicIP = syncToken, oldIPv4 })
	if err := f.syncer.Sync(context.Background(), false); err != nil {
		t.Fatal(err)
	}
}

func (f *hookFixture) port(t *testing.T) int {
	t.Helper()
	return f.container(t).Ports[0].HostPort
}

// Cambian los puertos: GET /proxy dice recreating y no hay un Load antes de
// volver. Después, Run recrea muelle-caddy con los puertos nuevos y le
// manda la configuración.
func TestSettingsChangedPorts(t *testing.T) {
	f := newHookFixture(t, 20*time.Millisecond)
	loads := len(f.caddy.Loads())
	old, next := f.change(t, func(p *shared.ProxySettings) { p.HTTPPort = 9080 })
	f.uc.Execute(context.Background(), old, next)
	if status, _, _ := f.state.Snapshot(true); status != ProxyRecreating {
		t.Fatalf("al volver: %s", status)
	}
	if len(f.caddy.Loads()) != loads || f.creates() != 1 {
		t.Fatalf("no manda ni recrea antes de volver: loads %d → %d, CreateContainer %d", loads, len(f.caddy.Loads()), f.creates())
	}
	eventually(t, 2*time.Second, "recrea", func() bool { return !f.state.Recreating() })
	if f.creates() != 2 || f.port(t) != 9080 || len(f.caddy.Loads()) != loads+1 || !f.state.Synced() {
		t.Fatalf("CreateContainer %d, puerto %d, loads %d", f.creates(), f.port(t), len(f.caddy.Loads()))
	}
}

// D15: la recreación espera al hook. Mientras el hook trabaja con
// Cloudflare (más que RecreateDelay), GET /proxy dice recreating y
// muelle-caddy no se toca; se recrea recién RecreateDelay después de que el
// hook vuelve.
func TestSettingsChangedWaitsForHook(t *testing.T) {
	const delay = 20 * time.Millisecond
	f := newHookFixture(t, delay)
	f.withToken(t)
	id := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: hookPanel, Content: oldIPv4})
	if err := f.settings.SavePanelDNS(context.Background(), service.PanelDNS{ZoneID: "z1", RecordID: id}); err != nil {
		t.Fatal(err)
	}
	f.cf.Hang(50 * time.Millisecond)
	old, next := f.change(t, func(p *shared.ProxySettings) { p.HTTPPort, p.PublicIP = 9080, newIPv4 })
	var returned time.Time // lo escribe la goroutine antes de cerrar done
	done := make(chan struct{})
	go func() {
		f.uc.Execute(context.Background(), old, next)
		returned = time.Now()
		close(done)
	}()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	timeout := time.After(5 * time.Second)
	checks := 0
wait:
	for {
		select {
		case <-done:
			break wait
		case <-tick.C:
			if status, _, _ := f.state.Snapshot(true); status != ProxyRecreating || f.creates() != 1 {
				t.Fatalf("mientras el hook corre: %s, CreateContainer %d", status, f.creates())
			}
			checks++
		case <-timeout:
			t.Fatal("el hook no volvió")
		}
	}
	if checks < 5 {
		t.Fatalf("el hook duró poco para probar algo: %d verificaciones", checks)
	}
	eventually(t, 2*time.Second, "recrea", func() bool { return f.creates() == 2 })
	// El release corre justo antes de que vuelva Execute: un margen para esa
	// diferencia, y para que returned se tomó un poco después.
	if d := time.Since(returned); d < delay/2 {
		t.Fatalf("recreó %s después de que volvió el hook, antes de RecreateDelay", d)
	}
	if a := f.cf.Records("z1")[0]; a.Content != newIPv4 {
		t.Fatalf("el registro del panel: %+v", a)
	}
	eventually(t, 2*time.Second, "termina", func() bool { return !f.state.Recreating() })
	if f.port(t) != 9080 {
		t.Fatalf("puerto %d", f.port(t))
	}
}

// Cambia el email ACME: un Load enseguida, con el email nuevo.
func TestSettingsChangedEmail(t *testing.T) {
	f := newHookFixture(t, time.Hour)
	loads := len(f.caddy.Loads())
	old, next := f.change(t, func(p *shared.ProxySettings) { p.ACMEEmail = "nuevo@correo.cl" })
	f.uc.Execute(context.Background(), old, next)
	if len(f.caddy.Loads()) != loads+1 || !strings.Contains(f.lastLoad(t), `"email":"nuevo@correo.cl"`) {
		t.Fatalf("loads %d → %d: %s", loads, len(f.caddy.Loads()), f.lastLoad(t))
	}
	if f.state.Recreating() {
		t.Fatal("no recrea")
	}
}

// Cambian las IPs permitidas del panel: un Load con el 403.
func TestSettingsChangedCIDRs(t *testing.T) {
	f := newHookFixture(t, time.Hour)
	if strings.Contains(f.lastLoad(t), "403") {
		t.Fatal("sin IPs permitidas no hay 403")
	}
	old, next := f.change(t, func(p *shared.ProxySettings) { p.PanelAllowedCIDRs = []string{"190.5.6.0/24"} })
	f.uc.Execute(context.Background(), old, next)
	if load := f.lastLoad(t); !strings.Contains(load, `"status_code":403`) || !strings.Contains(load, "190.5.6.0/24") {
		t.Fatalf("Load: %s", load)
	}
}

// Sin token, el panel (que tenía zona) deja de ir por Cloudflare.
func TestSettingsChangedTokenRemoved(t *testing.T) {
	f := newHookFixture(t, time.Hour)
	if err := f.settings.SavePanelDNS(context.Background(), service.PanelDNS{ZoneID: "z1"}); err != nil {
		t.Fatal(err)
	}
	f.withToken(t)
	if !strings.Contains(f.lastLoad(t), `"dns"`) {
		t.Fatalf("con token y zona, el panel va por Cloudflare: %s", f.lastLoad(t))
	}
	old, next := f.change(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	f.uc.Execute(context.Background(), old, next)
	if strings.Contains(f.lastLoad(t), `"dns"`) || strings.Contains(f.lastLoad(t), syncToken) {
		t.Fatalf("sin token no hay política cloudflare: %s", f.lastLoad(t))
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("sin token no llama a Cloudflare: %v", calls)
	}
}

// Un token nuevo: el registro del panel se crea, y el Load manda el panel
// por Cloudflare.
func TestSettingsChangedNewToken(t *testing.T) {
	f := newHookFixture(t, time.Hour)
	f.enableCaddy(t, func(p *shared.ProxySettings) { p.PublicIP = oldIPv4 })
	old, next := f.change(t, func(p *shared.ProxySettings) { p.CloudflareToken = syncToken })
	f.uc.Execute(context.Background(), old, next)
	recs := f.cf.Records("z1")
	if len(recs) != 1 || recs[0].Name != hookPanel || recs[0].Content != oldIPv4 {
		t.Fatalf("el registro del panel: %+v", recs)
	}
	if !strings.Contains(f.lastLoad(t), `"dns"`) {
		t.Fatalf("el panel va por Cloudflare: %s", f.lastLoad(t))
	}
}

// Cambia public_ip: los registros de los dominios y el del panel siguen a
// la IP, y un Load.
func TestSettingsChangedPublicIP(t *testing.T) {
	f := newHookFixture(t, time.Hour)
	f.withToken(t)
	panelID := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: hookPanel, Content: oldIPv4})
	if err := f.settings.SavePanelDNS(context.Background(), service.PanelDNS{ZoneID: "z1", RecordID: panelID}); err != nil {
		t.Fatal(err)
	}
	domA := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: "api.auditfy.cl", Content: oldIPv4})
	d, err := entity.NewDomain(f.apiID, "api.auditfy.cl", "api", 3000, false, f.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
	d.CFZoneID, d.TLSMode, d.CFRecordID = "z1", entity.TLSCloudflare, domA
	if err := f.repo.Create(context.Background(), &d); err != nil {
		t.Fatal(err)
	}
	loads := len(f.caddy.Loads())
	old, next := f.change(t, func(p *shared.ProxySettings) { p.PublicIP = newIPv4 })
	f.uc.Execute(context.Background(), old, next)
	for _, r := range f.cf.Records("z1") {
		if r.Content != newIPv4 {
			t.Fatalf("registro sin la IP nueva: %+v", r)
		}
	}
	if len(f.caddy.Loads()) != loads+1 {
		t.Fatalf("loads %d → %d", loads, len(f.caddy.Loads()))
	}
}

// El hook recibe el ctx de la petición: si el cliente ya se fue, termina igual.
func TestSettingsChangedCanceledCtx(t *testing.T) {
	f := newHookFixture(t, time.Hour)
	f.withToken(t)
	id := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: hookPanel, Content: oldIPv4})
	if err := f.settings.SavePanelDNS(context.Background(), service.PanelDNS{ZoneID: "z1", RecordID: id}); err != nil {
		t.Fatal(err)
	}
	loads := len(f.caddy.Loads())
	old, next := f.change(t, func(p *shared.ProxySettings) { p.PublicIP, p.ACMEEmail = newIPv4, "nuevo@correo.cl" })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	f.uc.Execute(ctx, old, next)
	if len(f.caddy.Loads()) != loads+1 || !strings.Contains(f.lastLoad(t), "nuevo@correo.cl") {
		t.Fatalf("loads %d → %d", loads, len(f.caddy.Loads()))
	}
	if a := f.cf.Records("z1")[0]; a.Content != newIPv4 {
		t.Fatalf("el registro del panel: %+v", a)
	}
}

// Con los puertos y el email en el mismo cambio, no hay un Load antes de la
// recreación: la configuración nueva la manda la recreación (contrato §3.5).
func TestSettingsChangedPortsAndEmail(t *testing.T) {
	f := newHookFixture(t, 20*time.Millisecond)
	loads := len(f.caddy.Loads())
	old, next := f.change(t, func(p *shared.ProxySettings) { p.HTTPPort, p.ACMEEmail = 9080, "nuevo@correo.cl" })
	f.uc.Execute(context.Background(), old, next)
	if len(f.caddy.Loads()) != loads {
		t.Fatal("hubo un Load antes de la recreación")
	}
	eventually(t, 2*time.Second, "recrea", func() bool { return !f.state.Recreating() })
	if len(f.caddy.Loads()) != loads+1 || !strings.Contains(f.lastLoad(t), "nuevo@correo.cl") {
		t.Fatalf("la recreación manda el email nuevo: loads %d → %d", loads, len(f.caddy.Loads()))
	}
}

// Un PATCH que vuelve a cambiar los puertos olvida la vuelta atrás anterior
// (GET /proxy: ports_rollback null): es un intento nuevo. Otro ajuste no la
// olvida.
func TestSettingsChangedPortsClearsRollback(t *testing.T) {
	f := newHookFixture(t, 20*time.Millisecond)
	rb := PortsRollback{Attempted: service.ProxyPorts{HTTPPort: 80, HTTPSPort: 443}, Restored: servingPorts, Error: "ocupados"}
	f.state.PortsRolledBack(rb)
	old, next := f.change(t, func(p *shared.ProxySettings) { p.ACMEEmail = "nuevo@correo.cl" })
	f.uc.Execute(context.Background(), old, next)
	if f.state.LastPortsRollback() == nil {
		t.Fatal("otro ajuste no la olvida")
	}
	old, next = f.change(t, func(p *shared.ProxySettings) { p.HTTPPort = 9080 })
	f.uc.Execute(context.Background(), old, next)
	if got := f.state.LastPortsRollback(); got != nil {
		t.Fatalf("un cambio de puertos la olvida: %+v", got)
	}
}
