package usecases

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/internal/ingress/infra/out/service/cloudflare"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

const (
	panelHost = "panel.tudominio.cl"
	oldIPv4   = "198.51.100.7"
	oldIPv6   = "2001:db8::7"
)

// newPanelDNS arma EnsurePanelDNS sobre el ucFixture (panel.tudominio.cl,
// la zona z1 tudominio.cl, el token y las dos IPs).
func newPanelDNS(f *ucFixture) *EnsurePanelDNS {
	return NewEnsurePanelDNS(f.deps.Settings, f.deps.DNS, f.ops, 30*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func (f *ucFixture) panelDNS(t *testing.T) service.PanelDNS {
	t.Helper()
	p, err := f.deps.Settings.PanelDNS(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *ucFixture) setPanelDNS(t *testing.T, p service.PanelDNS) {
	t.Helper()
	if err := f.deps.Settings.SavePanelDNS(context.Background(), p); err != nil {
		t.Fatal(err)
	}
}

// zonesSetting es cloudflare_zones tal como está en la base ("" si no está).
func (f *ucFixture) zonesSetting(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	var v string
	err := f.db.Conn(ctx).QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, shared.SettingCloudflareZones).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// one es el único registro de host con ese tipo en z1.
func (f *ucFixture) one(t *testing.T, host, typ string) service.Record {
	t.Helper()
	recs := f.recordsOf(host, typ)
	if len(recs) != 1 {
		t.Fatalf("esperaba un %s de %s: %+v", typ, host, f.cf.Records("z1"))
	}
	return recs[0]
}

func (f *ucFixture) hasCall(prefix string) bool {
	return slices.ContainsFunc(f.cf.Calls(), func(c string) bool { return strings.HasPrefix(c, prefix) })
}

// Sin registros: crea el A (sin proxy, con el comentario muelle) y el AAAA,
// guarda la zona, los ids y cloudflare_zones. La segunda vez no cambia nada.
func TestPanelDNSCreates(t *testing.T) {
	f := newUCFixture(t)
	uc := newPanelDNS(f)
	changed, err := uc.Execute(context.Background())
	if err != nil || !changed {
		t.Fatalf("Execute: %v %v", changed, err)
	}
	a, aaaa := f.one(t, panelHost, typeA), f.one(t, panelHost, typeAAAA)
	if a.Content != dnsIPv4 || a.Proxied || f.cf.Comment("z1", a.ID) != cloudflare.RecordComment {
		t.Fatalf("A: %+v %q", a, f.cf.Comment("z1", a.ID))
	}
	if aaaa.Content != dnsIPv6 || aaaa.Proxied || f.cf.Comment("z1", aaaa.ID) != cloudflare.RecordComment {
		t.Fatalf("AAAA: %+v", aaaa)
	}
	if got, want := f.panelDNS(t), (service.PanelDNS{ZoneID: "z1", RecordID: a.ID, RecordIDAAAA: aaaa.ID}); got != want {
		t.Fatalf("claves: %+v, esperaba %+v", got, want)
	}
	if got := f.zonesSetting(t); got != "1" {
		t.Fatalf("cloudflare_zones: %q", got)
	}

	writes := len(f.writes())
	changed, err = uc.Execute(context.Background())
	if err != nil || changed {
		t.Fatalf("la segunda vez: %v %v", changed, err)
	}
	if len(f.writes()) != writes {
		t.Fatalf("la segunda vez no escribe en Cloudflare: %v", f.writes())
	}
}

// Un A, un AAAA o un CNAME de ese nombre que muelle no creó no se tocan, y
// no se crea nada. La zona se guarda igual: decide el TLS del panel.
func TestPanelDNSForeign(t *testing.T) {
	for _, typ := range []string{typeA, typeAAAA, typeCNAME} {
		t.Run(typ, func(t *testing.T) {
			f := newUCFixture(t)
			content := map[string]string{typeA: oldIPv4, typeAAAA: oldIPv6, typeCNAME: "otro.cl"}[typ]
			f.cf.PutRecord("z1", service.Record{Type: typ, Name: panelHost, Content: content})
			changed, err := newPanelDNS(f).Execute(context.Background())
			if err != nil || !changed {
				t.Fatalf("Execute: %v %v", changed, err)
			}
			if w := f.writes(); len(w) != 0 {
				t.Fatalf("no debería escribir: %v", w)
			}
			if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1"}) {
				t.Fatalf("claves: %+v", got)
			}
		})
	}
}

// Un id guardado que ya no está, o que es de otro nombre, se olvida sin
// borrarlo, y se crea uno nuevo. Si el A ya no está pero su AAAA sí, el
// AAAA se borra (D29): si no, contaría como un registro ajeno.
func TestPanelDNSForgets(t *testing.T) {
	t.Run("ya no está", func(t *testing.T) {
		f := newUCFixture(t)
		f.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
		f.setPanelDNS(t, service.PanelDNS{ZoneID: "z1", RecordID: "noexiste"})
		if _, err := newPanelDNS(f).Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		a := f.one(t, panelHost, typeA)
		if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1", RecordID: a.ID}) {
			t.Fatalf("claves: %+v", got)
		}
		if f.hasCall("DELETE") {
			t.Fatalf("no borra: %v", f.cf.Calls())
		}
	})
	t.Run("el A no está y el AAAA sí", func(t *testing.T) {
		f := newUCFixture(t)
		old := f.cf.PutRecord("z1", service.Record{Type: typeAAAA, Name: panelHost, Content: oldIPv6})
		f.setPanelDNS(t, service.PanelDNS{ZoneID: "z1", RecordID: "agone", RecordIDAAAA: old})
		if _, err := newPanelDNS(f).Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		a, aaaa := f.one(t, panelHost, typeA), f.one(t, panelHost, typeAAAA)
		if aaaa.ID == old || aaaa.Content != dnsIPv6 {
			t.Fatalf("el AAAA viejo se borra y se crea uno nuevo: %+v", aaaa)
		}
		if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1", RecordID: a.ID, RecordIDAAAA: aaaa.ID}) {
			t.Fatalf("claves: %+v", got)
		}
	})
	t.Run("de otro nombre", func(t *testing.T) {
		f := newUCFixture(t)
		f.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
		old := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: "viejo.tudominio.cl", Content: dnsIPv4})
		f.setPanelDNS(t, service.PanelDNS{ZoneID: "z1", RecordID: old})
		if _, err := newPanelDNS(f).Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(f.recordsOf("viejo.tudominio.cl", typeA)) != 1 || f.hasCall("DELETE") || f.hasCall("PATCH") {
			t.Fatalf("el de otro nombre no se toca: %v", f.cf.Calls())
		}
		a := f.one(t, panelHost, typeA)
		if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1", RecordID: a.ID}) {
			t.Fatalf("claves: %+v", got)
		}
	})
}

// D27: un id que el token ya no puede leer (un token de otra cuenta) se
// olvida sin borrarlo, y el registro pasa a la zona que ve el token. Si
// Cloudflare no responde, no se olvida nada.
func TestPanelDNSForgetsUnreadable(t *testing.T) {
	old := service.PanelDNS{ZoneID: "zviejo", RecordID: "rviejo"}
	for _, status := range []int{403, 400} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			f := newUCFixture(t)
			f.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
			f.setPanelDNS(t, old)
			f.cf.FailNext("GET", "zviejo", status, 9109, "Unauthorized to access requested resource")
			changed, err := newPanelDNS(f).Execute(context.Background())
			if err != nil || !changed {
				t.Fatalf("Execute (%d): %v %v", status, changed, err)
			}
			if f.hasCall("DELETE") {
				t.Fatalf("no borra: %v", f.cf.Calls())
			}
			a := f.one(t, panelHost, typeA)
			if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1", RecordID: a.ID}) {
				t.Fatalf("claves (%d): %+v", status, got)
			}
		})
	}
	t.Run("503", func(t *testing.T) {
		f := newUCFixture(t)
		f.setPanelDNS(t, old)
		f.cf.FailNext("GET", "zviejo", 503, 10000, "unavailable")
		changed, err := newPanelDNS(f).Execute(context.Background())
		if !errors.Is(err, shared.ErrCloudflareUnavailable) || changed {
			t.Fatalf("Execute: %v %v", changed, err)
		}
		if got := f.panelDNS(t); got != old {
			t.Fatalf("las claves quedan como estaban: %+v", got)
		}
		if w := f.writes(); len(w) != 0 {
			t.Fatalf("no escribe: %v", w)
		}
	})
}

// Sin public_ip no crea ni actualiza registros (borrar public_ip no los
// toca), pero guarda la zona y cloudflare_zones.
func TestPanelDNSNoPublicIP(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP = "" })
	id := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: panelHost, Content: oldIPv4})
	f.setPanelDNS(t, service.PanelDNS{RecordID: id, ZoneID: "z1"})
	if _, err := newPanelDNS(f).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w := f.writes(); len(w) != 0 {
		t.Fatalf("no escribe: %v", w)
	}
	if a := f.one(t, panelHost, typeA); a.Content != oldIPv4 {
		t.Fatalf("A: %+v", a)
	}

	// Sin registros guardados tampoco crea nada; la zona sí se guarda.
	f = newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP = "" })
	changed, err := newPanelDNS(f).Execute(context.Background())
	if err != nil || !changed {
		t.Fatalf("Execute: %v %v", changed, err)
	}
	if w := f.writes(); len(w) != 0 {
		t.Fatalf("no escribe: %v", w)
	}
	if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1"}) || f.zonesSetting(t) != "1" {
		t.Fatalf("claves: %+v, cloudflare_zones %q", got, f.zonesSetting(t))
	}
}

// Con otra public_ip, el A propio cambia de contenido, y nunca de proxied:
// el que el usuario puso en Cloudflare se queda. El AAAA que se agrega
// después lleva el proxied del A.
func TestPanelDNSUpdatesContent(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	id := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: panelHost, Content: oldIPv4, Proxied: true})
	f.setPanelDNS(t, service.PanelDNS{ZoneID: "z1", RecordID: id})
	changed, err := newPanelDNS(f).Execute(context.Background())
	if err != nil || changed {
		t.Fatalf("Execute: %v %v", changed, err)
	}
	if a := f.one(t, panelHost, typeA); a.ID != id || a.Content != dnsIPv4 || !a.Proxied {
		t.Fatalf("A: %+v", a)
	}

	f.settings(t, nil) // con public_ipv6
	if _, err := newPanelDNS(f).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	aaaa := f.one(t, panelHost, typeAAAA)
	if aaaa.Content != dnsIPv6 || !aaaa.Proxied {
		t.Fatalf("AAAA: %+v", aaaa)
	}
	if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1", RecordID: id, RecordIDAAAA: aaaa.ID}) {
		t.Fatalf("claves: %+v", got)
	}
}

// Sin public_ipv6, el AAAA propio se borra y su clave también.
func TestPanelDNSDropsAAAA(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIPv6 = "" })
	a := f.cf.PutRecord("z1", service.Record{Type: typeA, Name: panelHost, Content: dnsIPv4})
	aaaa := f.cf.PutRecord("z1", service.Record{Type: typeAAAA, Name: panelHost, Content: oldIPv6})
	f.setPanelDNS(t, service.PanelDNS{ZoneID: "z1", RecordID: a, RecordIDAAAA: aaaa})
	if _, err := newPanelDNS(f).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if recs := f.recordsOf(panelHost, typeAAAA); len(recs) != 0 {
		t.Fatalf("el AAAA sigue: %+v", recs)
	}
	if got := f.panelDNS(t); got != (service.PanelDNS{ZoneID: "z1", RecordID: a}) {
		t.Fatalf("claves: %+v", got)
	}
}

// Un panel_domain fuera de las zonas que ve el token borra panel_cf_zone_id.
func TestPanelDNSOutOfZone(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PanelDomain = "panel.otro.cl" })
	f.setPanelDNS(t, service.PanelDNS{ZoneID: "z1"})
	changed, err := newPanelDNS(f).Execute(context.Background())
	if err != nil || !changed {
		t.Fatalf("Execute: %v %v", changed, err)
	}
	if got := f.panelDNS(t); got != (service.PanelDNS{}) {
		t.Fatalf("claves: %+v", got)
	}
	if w := f.writes(); len(w) != 0 {
		t.Fatalf("no escribe: %v", w)
	}
}

// Sin token no llama a Cloudflare.
func TestPanelDNSNoToken(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.CloudflareToken = "" })
	changed, err := newPanelDNS(f).Execute(context.Background())
	if err != nil || changed {
		t.Fatalf("Execute: %v %v", changed, err)
	}
	if calls := f.cf.Calls(); len(calls) != 0 {
		t.Fatalf("no llama a Cloudflare: %v", calls)
	}
}

// D30: usa los ajustes guardados al momento de correr.
func TestPanelDNSReadsCurrent(t *testing.T) {
	f := newUCFixture(t)
	f.settings(t, func(p *shared.ProxySettings) { p.PublicIP, p.PublicIPv6 = "203.0.113.99", "" })
	if _, err := newPanelDNS(f).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if a := f.one(t, panelHost, typeA); a.Content != "203.0.113.99" {
		t.Fatalf("A: %+v", a)
	}
}

// tokenSettings devuelve el token tok en ProxySettings y delega lo demás en
// el adaptador real: el token guardado cambió mientras corría.
type tokenSettings struct {
	service.Settings
	tok string
}

func (s tokenSettings) ProxySettings(ctx context.Context) (shared.ProxySettings, error) {
	p, err := s.Settings.ProxySettings(ctx)
	p.CloudflareToken = s.tok
	return p, err
}

// D28: si el token guardado ya no es el que listó las zonas,
// cloudflare_zones no se escribe.
func TestPanelDNSZonesCAS(t *testing.T) {
	f := newUCFixture(t)
	const t1 = "cf-token-viejo-1234567890"
	f.cf.AddToken(t1)
	uc := NewEnsurePanelDNS(tokenSettings{f.deps.Settings, t1}, f.deps.DNS, f.ops, 30*time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if _, err := uc.Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := f.zonesSetting(t); got != "" {
		t.Fatalf("cloudflare_zones: %q", got)
	}
	if !f.hasCall("GET /zones") {
		t.Fatal("debió listar las zonas")
	}
}

// Con un ctx sin plazo (el del job), Execute usa el suyo (Timing.DomainOp) y
// suelta el lock de Ops.
func TestPanelDNSDeadline(t *testing.T) {
	f := newUCFixture(t)
	f.cf.Hang(200 * time.Millisecond)
	uc := NewEnsurePanelDNS(f.deps.Settings, f.deps.DNS, f.ops, 50*time.Millisecond, slog.New(slog.NewTextHandler(io.Discard, nil)))
	start := time.Now()
	if _, err := uc.Execute(context.Background()); err == nil {
		t.Fatal("debió fallar por el plazo")
	}
	if d := time.Since(start); d < 50*time.Millisecond || d > 150*time.Millisecond {
		t.Fatalf("volvió en %s", d)
	}
	start = time.Now()
	_ = f.ops.Do(context.Background(), time.Second, func(context.Context, context.Context) error { return nil })
	if d := time.Since(start); d > 20*time.Millisecond {
		t.Fatalf("el lock seguía tomado: %s", d)
	}
}
