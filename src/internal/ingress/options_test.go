package ingress

import (
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	"github.com/CPU-commits/muelle/src/shared/domain/naming"
)

// D3: el cero es producción.
func TestOptionsDefaults(t *testing.T) {
	o := Options{}.withDefaults()
	if o.CaddyContainer != naming.CaddyContainer || o.PanelNetwork != naming.PanelNetwork ||
		o.CaddyDataVolume != naming.CaddyDataVolume || o.CaddyConfigVolume != naming.CaddyConfigVolume ||
		o.CaddyAdminVolume != naming.CaddyAdminVolume {
		t.Errorf("nombres: %+v", o)
	}
	if o.CaddyTLSAddr != "muelle-caddy:443" || o.CloudflareURL != cloudflare.DefaultBaseURL {
		t.Errorf("CaddyTLSAddr %q, CloudflareURL %q", o.CaddyTLSAddr, o.CloudflareURL)
	}
	if o.CaddyAdminDial != nil || o.TLSRoots != nil || o.InternalCA || o.NetworkPrefix != "" {
		t.Errorf("producción: socket unix, raíces del sistema, ACME y todas las redes: %+v", o)
	}
	want := Timing{
		RetryMin: time.Second, RetryMax: 60 * time.Second, RecreateDelay: 2 * time.Second,
		FirstCheck: 30 * time.Second, PendingEvery: time.Minute, DomainOp: 30 * time.Second,
		DomainRetry: 20 * time.Second, Hook: 2 * time.Minute,
	}
	if o.Timing != want {
		t.Errorf("Timing: %+v", o.Timing)
	}

	// Un nombre propio de muelle-caddy cambia también la dirección TLS por
	// defecto; lo que viene puesto no se pisa.
	o = Options{CaddyContainer: "it3-x", CloudflareURL: "http://127.0.0.1:1/client/v4", Timing: Timing{RetryMin: time.Millisecond}}.withDefaults()
	if o.CaddyTLSAddr != "it3-x:443" || o.CloudflareURL != "http://127.0.0.1:1/client/v4" {
		t.Errorf("propios: %+v", o)
	}
	if o.Timing.RetryMin != time.Millisecond || o.Timing.RetryMax != 60*time.Second {
		t.Errorf("Timing por campo: %+v", o.Timing)
	}
	if o = (Options{CaddyContainer: "it3-x", CaddyTLSAddr: "127.0.0.1:21102"}).withDefaults(); o.CaddyTLSAddr != "127.0.0.1:21102" {
		t.Errorf("CaddyTLSAddr propio: %q", o.CaddyTLSAddr)
	}
}
