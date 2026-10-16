package usecases

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
)

// probeCall es un ProbePorts que pidió CheckProxyPorts.
type probeCall struct {
	image string
	ports []service.HostPort
}

// probeRuntime es un CaddyRuntime con el muelle-caddy que sirve fijo, que
// anota cada ProbePorts y responde busy para los puertos de busy.
type probeRuntime struct {
	recordingRuntime
	mu      sync.Mutex
	serving *service.CaddyServing
	busy    map[int]bool
	calls   []probeCall
}

func (r *probeRuntime) Serving(ctx context.Context) (service.CaddyServing, bool, error) {
	if r.serving == nil {
		return service.CaddyServing{}, false, nil
	}
	return *r.serving, true, nil
}

func (r *probeRuntime) ProbePorts(ctx context.Context, image string, ports []service.HostPort) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, probeCall{image, slices.Clone(ports)})
	for _, p := range ports {
		if r.busy[p.Port] {
			return service.ErrPortInUse
		}
	}
	return nil
}

// proxyPorts son ajustes con el proxy habilitado y esos puertos.
func proxyPorts(http, https int) shared.ProxySettings {
	p := shared.DefaultProxySettings()
	p.PanelDomain = "panel.auditfy.cl"
	p.HTTPPort, p.HTTPSPort = http, https
	return p
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func tcp(port int) service.HostPort { return service.HostPort{Port: port, Protocol: "tcp"} }
func udp(port int) service.HostPort { return service.HostPort{Port: port, Protocol: "udp"} }

// Antes de guardar otros puertos del proxy se prueban los que cambian, de a
// un campo, salvo los que ya tiene el muelle-caddy que sirve: esos los suelta
// al recrearse.
func TestCheckProxyPorts(t *testing.T) {
	serving := &service.CaddyServing{Image: defCaddyImage, HTTPPort: 8080, HTTPSPort: 8443}
	cases := []struct {
		name       string
		serving    *service.CaddyServing
		old, next  shared.ProxySettings
		want       []probeCall
		wantNoCall bool
	}{
		{name: "sin cambio de puertos", serving: serving, old: proxyPorts(8080, 8443), next: proxyPorts(8080, 8443), wantNoCall: true},
		{name: "solo el HTTPS", serving: serving, old: proxyPorts(8080, 8443), next: proxyPorts(8080, 443),
			want: []probeCall{{defCaddyImage, []service.HostPort{tcp(443), udp(443)}}}},
		{name: "los dos (la migración, spec §16)", serving: serving, old: proxyPorts(8080, 8443), next: proxyPorts(80, 443),
			want: []probeCall{{defCaddyImage, []service.HostPort{tcp(80)}}, {defCaddyImage, []service.HostPort{tcp(443), udp(443)}}}},
		// Los dos puertos cruzados: los TCP los tiene muelle-caddy; solo el
		// UDP del HTTPS nuevo (8080) está por tomar.
		{name: "cruzados", serving: serving, old: proxyPorts(8080, 8443), next: proxyPorts(8443, 8080),
			want: []probeCall{{defCaddyImage, []service.HostPort{udp(8080)}}}},
		// Sin un muelle-caddy que sirva (no existe, o no arrancó), se prueban
		// todos los que cambian, con la imagen de los ajustes.
		{name: "sin muelle-caddy", old: proxyPorts(8080, 8443), next: proxyPorts(8080, 9443),
			want: []probeCall{{defCaddyImage, []service.HostPort{tcp(9443), udp(9443)}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := &probeRuntime{serving: tc.serving}
			uc := NewCheckProxyPorts(rt, defCaddyImage, discardLog())
			if err := uc.Execute(context.Background(), tc.old, tc.next); err != nil {
				t.Fatal(err)
			}
			if tc.wantNoCall {
				if len(rt.calls) != 0 {
					t.Fatalf("no debió probar nada: %+v", rt.calls)
				}
				return
			}
			if len(rt.calls) != len(tc.want) {
				t.Fatalf("pruebas: %+v, esperaba %+v", rt.calls, tc.want)
			}
			for i := range tc.want {
				if rt.calls[i].image != tc.want[i].image || !slices.Equal(rt.calls[i].ports, tc.want[i].ports) {
					t.Fatalf("prueba %d: %+v, esperaba %+v", i, rt.calls[i], tc.want[i])
				}
			}
		})
	}
}

// Un puerto ocupado es un 422 en su campo, con el puerto en el mensaje.
func TestCheckProxyPortsBusy(t *testing.T) {
	serving := &service.CaddyServing{Image: defCaddyImage, HTTPPort: 8080, HTTPSPort: 8443}
	for _, tc := range []struct {
		field string
		busy  int
	}{{shared.SettingProxyHTTPPort, 80}, {shared.SettingProxyHTTPSPort, 443}} {
		t.Run(tc.field, func(t *testing.T) {
			rt := &probeRuntime{serving: serving, busy: map[int]bool{tc.busy: true}}
			err := NewCheckProxyPorts(rt, defCaddyImage, discardLog()).Execute(context.Background(), proxyPorts(8080, 8443), proxyPorts(80, 443))
			ve, ok := errors.AsType[*validate.Error](err)
			if !ok || ve.Field != tc.field || !strings.Contains(ve.Message, "el puerto "+strconv.Itoa(tc.busy)+" del host ya está en uso") {
				t.Fatalf("esperaba un 422 en %s: %v", tc.field, err)
			}
		})
	}
}

// En desarrollo (sin el proxy habilitado) no se toca Docker.
func TestCheckProxyPortsDisabled(t *testing.T) {
	rt := &probeRuntime{}
	old, next := proxyPorts(8080, 8443), proxyPorts(80, 443)
	old.PanelDomain, next.PanelDomain = "", ""
	if err := NewCheckProxyPorts(rt, defCaddyImage, discardLog()).Execute(context.Background(), old, next); err != nil || len(rt.calls) != 0 {
		t.Fatalf("deshabilitado: %v %+v", err, rt.calls)
	}
}

// Con el Docker en memoria y el runtime de verdad: un puerto que tiene otro
// contenedor es un 422; Docker caído, su error (503); y la prueba nunca toca
// el muelle-caddy que sirve.
func TestCheckProxyPortsWithDocker(t *testing.T) {
	f := newLifecycleFixture(t)
	f.enableCaddy(t, nil)
	f.docker.SetImagePresent(defCaddyImage)
	ctx := context.Background()
	if _, err := f.ensure.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	before := f.container(t)
	uc := NewCheckProxyPorts(f.runtime, defCaddyImage, f.slog)
	if err := uc.Execute(ctx, proxyPorts(8080, 8443), proxyPorts(8080, 9443)); err != nil {
		t.Fatalf("libre: %v", err)
	}
	f.docker.BusyHostPort(9443)
	err := uc.Execute(ctx, proxyPorts(8080, 8443), proxyPorts(8080, 9443))
	if ve, ok := errors.AsType[*validate.Error](err); !ok || ve.Field != shared.SettingProxyHTTPSPort {
		t.Fatalf("ocupado: %v", err)
	}
	if c := f.container(t); c.ID != before.ID || !c.Running {
		t.Fatalf("muelle-caddy cambió: %+v", c)
	}
	f.docker.SetUnavailable(true)
	if err := uc.Execute(ctx, proxyPorts(8080, 8443), proxyPorts(8080, 9443)); !errors.Is(err, shared.ErrDockerUnavailable) {
		t.Fatalf("Docker caído: %v", err)
	}
}
