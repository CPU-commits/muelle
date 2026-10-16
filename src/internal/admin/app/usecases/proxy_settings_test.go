package usecases

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/CPU-commits/muelle/src/internal/admin/admintest"
	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	adminrepo "github.com/CPU-commits/muelle/src/internal/admin/infra/out/repository/sqlite"
	adminsvc "github.com/CPU-commits/muelle/src/internal/admin/infra/out/service"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
	"github.com/CPU-commits/muelle/src/shared/domain/validate"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
)

// fakeHook hace de ingress: responde las zonas del token y registra los cambios.
type fakeHook struct {
	mu       sync.Mutex
	zones    *int
	checkErr error
	checked  []string
	changes  [][2]entity.ProxySettings
}

func (h *fakeHook) CheckCloudflareToken(ctx context.Context, token string) (*int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checked = append(h.checked, token)
	return h.zones, h.checkErr
}

func (h *fakeHook) ProxySettingsChanged(ctx context.Context, old, next entity.ProxySettings) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.changes = append(h.changes, [2]entity.ProxySettings{old, next})
}

type proxyEnv struct {
	*env
	repo   *adminrepo.SettingsRepository
	cipher *crypto.Cipher
	hook   *fakeHook
	get    *GetSettings
	origin *GetPanelOrigin
	patch  *PatchSettings
	user   int64
}

func newProxyEnv(t *testing.T) *proxyEnv {
	t.Helper()
	e := newEnv(t)
	c, err := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	repo := adminrepo.NewSettingsRepository(e.db)
	two := 2
	hook := &fakeHook{zones: &two}
	origin := NewGetPanelOrigin(repo, e.clk)
	return &proxyEnv{
		env: e, repo: repo, cipher: c, hook: hook, origin: origin,
		get:   NewGetSettings(repo, c),
		patch: NewPatchSettings(repo, adminsvc.NewTOTPStatus(e.db), c, hook, origin, e.rec, e.db),
		user:  admintest.InsertUser(t, e.db, "admin", true),
	}
}

func (e *proxyEnv) do(t *testing.T, in dto.PatchSettingsInput) (dto.SettingsOutput, error) {
	t.Helper()
	if in.ClientIP == "" {
		in.ClientIP = "203.0.113.9"
	}
	return e.patch.Execute(asUser(e.user), in)
}

func (e *proxyEnv) raw(t *testing.T, key string) (string, bool) {
	t.Helper()
	v, enc, err := e.repo.Get(context.Background(), key)
	if err != nil {
		return "", false
	}
	if enc {
		pt, err := e.cipher.Decrypt(v, "settings:"+key)
		if err != nil {
			t.Fatalf("%s no descifra con su AAD: %v", key, err)
		}
		return string(pt), true
	}
	return string(v), true
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	ve, ok := errors.AsType[*validate.Error](err)
	if !ok || ve.Field != field {
		t.Fatalf("esperaba un error de validación en %q, llegó %v", field, err)
	}
}

// GET /settings trae siempre todas las claves de la Fase 3, con sus valores
// por defecto: el panel usa su presencia para saber que la fase está.
func TestGetSettingsDefaults(t *testing.T) {
	e := newProxyEnv(t)
	out, err := e.get.Execute(context.Background(), dto.GetSettingsInput{ClientIP: "203.0.113.9"})
	if err != nil {
		t.Fatal(err)
	}
	want := dto.SettingsOutput{PanelAllowedCIDRs: []string{}, ProxyHTTPPort: 80, ProxyHTTPSPort: 443, ClientIP: "203.0.113.9"}
	if fmt.Sprintf("%+v", out) != fmt.Sprintf("%+v", want) || out.PanelAllowedCIDRs == nil {
		t.Fatalf("por defecto:\n%+v\nesperaba\n%+v", out, want)
	}
}

func TestPatchProxySettings(t *testing.T) {
	e := newProxyEnv(t)
	out, err := e.do(t, dto.PatchSettingsInput{
		PublicIP: ptr("203.0.113.7"), PublicIPv6: ptr("2001:db8::7"), ACMEEmail: ptr("tu@correo.cl"),
		ProxyHTTPPort: ptr(8080), ProxyHTTPSPort: ptr(8443),
		PanelAllowedCIDRs: &[]string{"203.0.113.9/24", "2001:db8::1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Los rangos se guardan normalizados (enmascarados, una IP sola como /32 o /128).
	if out.PublicIP != "203.0.113.7" || out.PublicIPv6 != "2001:db8::7" || out.ACMEEmail != "tu@correo.cl" ||
		out.ProxyHTTPPort != 8080 || out.ProxyHTTPSPort != 8443 ||
		!slices.Equal(out.PanelAllowedCIDRs, []string{"203.0.113.0/24", "2001:db8::1/128"}) {
		t.Fatalf("out=%+v", out)
	}
	// Se guardan con el formato que lee ingress (entity.ProxySettings.Encode).
	for key, want := range map[string]string{
		entity.SettingPublicIP: "203.0.113.7", entity.SettingProxyHTTPPort: "8080",
		entity.SettingPanelAllowedCIDRs: `["203.0.113.0/24","2001:db8::1/128"]`,
	} {
		if got, _ := e.raw(t, key); got != want {
			t.Errorf("%s guardado como %q, esperaba %q", key, got, want)
		}
	}
	// ingress recibe los ajustes viejos y los nuevos, una vez.
	if len(e.hook.changes) != 1 {
		t.Fatalf("hook: %d llamadas", len(e.hook.changes))
	}
	old, next := e.hook.changes[0][0], e.hook.changes[0][1]
	want := []string{entity.SettingACMEEmail, entity.SettingPanelAllowedCIDRs, entity.SettingProxyHTTPPort, entity.SettingProxyHTTPSPort, entity.SettingPublicIP, entity.SettingPublicIPv6}
	if got := next.ChangedKeys(old); !slices.Equal(got, want) {
		t.Fatalf("ChangedKeys=%v", got)
	}

	// Los mismos valores otra vez: no se llama a ingress ni se audita.
	before := len(admintestEvents(t, e))
	if _, err := e.do(t, dto.PatchSettingsInput{PublicIP: ptr("203.0.113.7"), ProxyHTTPPort: ptr(8080)}); err != nil {
		t.Fatal(err)
	}
	if len(e.hook.changes) != 1 || len(admintestEvents(t, e)) != before {
		t.Fatal("sin cambios no se llama a ingress ni se audita")
	}

	// "" borra la IPv6 (sin registros AAAA) y la IP pública.
	out, err = e.do(t, dto.PatchSettingsInput{PublicIPv6: ptr(""), PublicIP: ptr("")})
	if err != nil || out.PublicIPv6 != "" || out.PublicIP != "" {
		t.Fatalf("borrar las IP: %+v %v", out, err)
	}
}

func admintestEvents(t *testing.T, e *proxyEnv) []dto.AuditEventOutput {
	t.Helper()
	events, err := NewListAudit(adminrepo.NewAuditRepository(e.db)).Execute(context.Background(), dto.ListAuditInput{})
	if err != nil {
		t.Fatal(err)
	}
	return events
}

// La auditoría dice qué cambió, con los valores que no son secretos; el token
// solo como "set" o "removed".
func TestPatchProxySettingsAudit(t *testing.T) {
	e := newProxyEnv(t)
	if _, err := e.do(t, dto.PatchSettingsInput{PublicIP: ptr("203.0.113.7"), CloudflareToken: ptr("cf-token-1234567890abcdef")}); err != nil {
		t.Fatal(err)
	}
	events := admintestEvents(t, e)
	if len(events) != 1 || events[0].Action != "settings.update" ||
		string(events[0].Details) != `{"cloudflare_token":"set","public_ip":"203.0.113.7"}` {
		t.Fatalf("events=%+v (%s)", events, events[0].Details)
	}
	if bytes.Contains(events[0].Details, []byte("cf-token")) {
		t.Fatal("el token llegó a la auditoría")
	}
}

func TestPatchProxySettingsValidation(t *testing.T) {
	e := newProxyEnv(t)
	cases := []struct {
		in    dto.PatchSettingsInput
		field string
	}{
		{dto.PatchSettingsInput{PublicIP: ptr("203.0.113")}, "public_ip"},
		{dto.PatchSettingsInput{PublicIP: ptr("2001:db8::7")}, "public_ip"},
		{dto.PatchSettingsInput{PublicIPv6: ptr("203.0.113.7")}, "public_ipv6"},
		{dto.PatchSettingsInput{ACMEEmail: ptr("no-es-email")}, "acme_email"},
		{dto.PatchSettingsInput{ProxyHTTPPort: ptr(0)}, "proxy_http_port"},
		{dto.PatchSettingsInput{ProxyHTTPSPort: ptr(70000)}, "proxy_https_port"},
		// Puertos iguales: el error va en proxy_https_port, también si solo cambia uno.
		{dto.PatchSettingsInput{ProxyHTTPPort: ptr(8443), ProxyHTTPSPort: ptr(8443)}, "proxy_https_port"},
		{dto.PatchSettingsInput{ProxyHTTPPort: ptr(443)}, "proxy_https_port"},
		{dto.PatchSettingsInput{PanelAllowedCIDRs: &[]string{"10.0.0.0/8", "no"}}, "panel_allowed_cidrs"},
		{dto.PatchSettingsInput{CloudflareToken: ptr("con espacios adentro y largo")}, "cloudflare_token"},
		{dto.PatchSettingsInput{CloudflareToken: ptr("corto")}, "cloudflare_token"},
	}
	for _, c := range cases {
		_, err := e.do(t, c.in)
		wantField(t, err, c.field)
	}
	if len(e.hook.changes) != 0 || len(e.hook.checked) != 0 {
		t.Fatal("un PATCH inválido no llega a ingress")
	}
	if _, ok := e.raw(t, entity.SettingPublicIP); ok {
		t.Fatal("un PATCH inválido no guarda nada")
	}
}

// El token se verifica con ingress antes de guardarlo (cuántas zonas ve), va
// cifrado con settings:cloudflare_token y nunca vuelve en el GET.
func TestPatchCloudflareToken(t *testing.T) {
	e := newProxyEnv(t)
	const token = "cf-token-1234567890abcdef"
	out, err := e.do(t, dto.PatchSettingsInput{CloudflareToken: ptr(token)})
	if err != nil {
		t.Fatal(err)
	}
	if !out.CloudflareTokenSet || out.CloudflareZones == nil || *out.CloudflareZones != 2 {
		t.Fatalf("out=%+v", out)
	}
	if got, ok := e.raw(t, entity.SettingCloudflareToken); !ok || got != token {
		t.Fatalf("token guardado: %q %v", got, ok)
	}
	if !slices.Equal(e.hook.checked, []string{token}) || len(e.hook.changes) != 1 || e.hook.changes[0][1].CloudflareToken != token {
		t.Fatalf("hook: %+v", e.hook)
	}

	// Un token que Cloudflare rechaza: 422 en cloudflare_token y no se guarda.
	e.hook.checkErr = validate.Errorf("cloudflare_token", "Cloudflare rechazó el token")
	_, err = e.do(t, dto.PatchSettingsInput{CloudflareToken: ptr("otro-token-1234567890abc"), PublicIP: ptr("203.0.113.7")})
	wantField(t, err, "cloudflare_token")
	if got, _ := e.raw(t, entity.SettingCloudflareToken); got != token {
		t.Fatal("un token rechazado no reemplaza al anterior")
	}
	if _, ok := e.raw(t, entity.SettingPublicIP); ok {
		t.Fatal("si el token falla no se guarda nada del PATCH")
	}
	// Cloudflare caído: el error pasa (502).
	e.hook.checkErr = fmt.Errorf("zonas: %w", entity.ErrCloudflareUnavailable)
	_, err = e.do(t, dto.PatchSettingsInput{CloudflareToken: ptr("otro-token-1234567890abc")})
	if !errors.Is(err, entity.ErrCloudflareUnavailable) {
		t.Fatalf("Cloudflare caído: %v", err)
	}

	// Sin verificar (zonas desconocidas): se guarda y cloudflare_zones es null.
	e.hook.checkErr, e.hook.zones = nil, nil
	out, err = e.do(t, dto.PatchSettingsInput{CloudflareToken: ptr("otro-token-1234567890abc")})
	if err != nil || !out.CloudflareTokenSet || out.CloudflareZones != nil {
		t.Fatalf("sin verificar: %+v %v", out, err)
	}

	// "" borra el token (y sus zonas).
	out, err = e.do(t, dto.PatchSettingsInput{CloudflareToken: ptr("")})
	if err != nil || out.CloudflareTokenSet || out.CloudflareZones != nil {
		t.Fatalf("borrar el token: %+v %v", out, err)
	}
	if _, ok := e.raw(t, entity.SettingCloudflareToken); ok {
		t.Fatal("el token sigue guardado")
	}
	if last := e.hook.changes[len(e.hook.changes)-1][1]; last.CloudflareToken != "" {
		t.Fatal("ingress no se enteró de que se borró el token")
	}
}

// Una lista de IPs permitidas que deja fuera la IP de quien la guarda lo
// encerraría (Caddy lo bloquea, §8): 409 salvo que lo confirme.
func TestPatchPanelAllowedCIDRsLockout(t *testing.T) {
	e := newProxyEnv(t)
	other := &[]string{"198.51.100.0/24"}
	_, err := e.do(t, dto.PatchSettingsInput{PanelAllowedCIDRs: other, ClientIP: "203.0.113.9"})
	if !errors.Is(err, service.ErrWouldLockOut) {
		t.Fatalf("err=%v", err)
	}
	if _, ok := e.raw(t, entity.SettingPanelAllowedCIDRs); ok {
		t.Fatal("no debió guardarse")
	}
	out, err := e.do(t, dto.PatchSettingsInput{PanelAllowedCIDRs: other, ClientIP: "203.0.113.9", ConfirmLockout: true})
	if err != nil || !slices.Equal(out.PanelAllowedCIDRs, []string{"198.51.100.0/24"}) {
		t.Fatalf("confirmado: %+v %v", out, err)
	}
	// Si la incluye, o la lista queda vacía, no hace falta confirmar.
	for _, list := range [][]string{{"203.0.113.0/24"}, {"2001:db8::/32", "203.0.113.9"}, {}} {
		if _, err := e.do(t, dto.PatchSettingsInput{PanelAllowedCIDRs: &list, ClientIP: "203.0.113.9"}); err != nil {
			t.Fatalf("%v: %v", list, err)
		}
	}
	if _, err := e.do(t, dto.PatchSettingsInput{PanelAllowedCIDRs: &[]string{"2001:db8::/32"}, ClientIP: "2001:db8::5"}); err != nil {
		t.Fatalf("IPv6 incluida: %v", err)
	}
}

// Sin ingress conectado (Hook nil), los ajustes solo se guardan.
func TestPatchProxySettingsWithoutHook(t *testing.T) {
	e := newProxyEnv(t)
	patch := NewPatchSettings(e.repo, adminsvc.NewTOTPStatus(e.db), e.cipher, nil, nil, e.rec, e.db)
	out, err := patch.Execute(asUser(e.user), dto.PatchSettingsInput{PublicIP: ptr("203.0.113.7"), CloudflareToken: ptr("cf-token-1234567890abcdef")})
	if err != nil || out.PublicIP != "203.0.113.7" || !out.CloudflareTokenSet || out.CloudflareZones != nil {
		t.Fatalf("sin hook: %+v %v", out, err)
	}
}

// Al arrancar: panel_domain sale siempre de MUELLE_PANEL_ORIGIN y la
// configuración inicial de install (MUELLE_INIT_*) se guarda una sola vez:
// después mandan los cambios del panel.
func TestApplyStartupSettings(t *testing.T) {
	e := newProxyEnv(t)
	apply := NewApplyStartupSettings(e.repo, e.cipher, e.rec, e.db)
	ctx := context.Background()
	init := dto.InitialSettings{
		PublicIP: "203.0.113.7", PublicIPv6: "2001:db8::7", ACMEEmail: "tu@correo.cl",
		ProxyHTTPPort: 8080, ProxyHTTPSPort: 8443, CloudflareToken: "cf-token-1234567890abcdef",
		CaddyImage: "ghcr.io/cpu-commits/muelle-caddy:v0.1.0",
	}
	if err := apply.Execute(ctx, dto.StartupSettings{PanelDomain: "panel.tudominio.cl", Init: init}); err != nil {
		t.Fatal(err)
	}
	out, _ := e.get.Execute(ctx, dto.GetSettingsInput{})
	if out.PanelDomain != "panel.tudominio.cl" || out.PublicIP != "203.0.113.7" || out.PublicIPv6 != "2001:db8::7" ||
		out.ACMEEmail != "tu@correo.cl" || out.ProxyHTTPPort != 8080 || out.ProxyHTTPSPort != 8443 || !out.CloudflareTokenSet {
		t.Fatalf("out=%+v", out)
	}
	if got, _ := e.raw(t, entity.SettingCaddyImage); got != init.CaddyImage {
		t.Fatalf("caddy_image=%q", got)
	}
	if got, _ := e.raw(t, entity.SettingCloudflareToken); got != init.CloudflareToken {
		t.Fatal("el token va cifrado con su AAD")
	}
	events := admintestEvents(t, e)
	if len(events) != 1 || events[0].Action != "settings.init" || events[0].ActorType != "system" || bytes.Contains(events[0].Details, []byte("cf-token")) {
		t.Fatalf("events=%+v", events)
	}

	// El panel cambia la IP; en el próximo arranque, MUELLE_INIT_* no la pisa
	// (ni audita de nuevo), pero panel_domain sí sigue al origen.
	if _, err := e.do(t, dto.PatchSettingsInput{PublicIP: ptr("198.51.100.1")}); err != nil {
		t.Fatal(err)
	}
	if err := apply.Execute(ctx, dto.StartupSettings{PanelDomain: "otro.tudominio.cl", Init: init}); err != nil {
		t.Fatal(err)
	}
	out, _ = e.get.Execute(ctx, dto.GetSettingsInput{})
	if out.PublicIP != "198.51.100.1" || out.PanelDomain != "otro.tudominio.cl" {
		t.Fatalf("segundo arranque: %+v", out)
	}
	if n := len(admintestEvents(t, e)); n != 2 {
		t.Fatalf("eventos: %d (init una sola vez + el PATCH)", n)
	}

	// Un origen que no es un dominio (desarrollo) deja panel_domain vacío.
	if err := apply.Execute(ctx, dto.StartupSettings{}); err != nil {
		t.Fatal(err)
	}
	if out, _ = e.get.Execute(ctx, dto.GetSettingsInput{}); out.PanelDomain != "" {
		t.Fatalf("panel_domain=%q", out.PanelDomain)
	}
}

// Lo que el panel borra no vuelve con el próximo arranque: install suele
// dejar montado el secreto del token (y las variables MUELLE_INIT_*), y un
// reinicio cualquiera (docker service update, el nodo que se reinicia) no
// puede devolver un token que el admin quitó, quizás porque se filtró.
func TestApplyStartupSettingsOnlyOnFirstBoot(t *testing.T) {
	e := newProxyEnv(t)
	apply := NewApplyStartupSettings(e.repo, e.cipher, e.rec, e.db)
	ctx := context.Background()
	init := dto.InitialSettings{PublicIP: "203.0.113.7", ACMEEmail: "tu@correo.cl", CloudflareToken: "cf-token-1234567890abcdef"}
	boot := func() dto.SettingsOutput {
		t.Helper()
		if err := apply.Execute(ctx, dto.StartupSettings{PanelDomain: "panel.tudominio.cl", Init: init}); err != nil {
			t.Fatal(err)
		}
		out, err := e.get.Execute(ctx, dto.GetSettingsInput{})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	if out := boot(); !out.CloudflareTokenSet || out.PublicIP != "203.0.113.7" {
		t.Fatalf("primer arranque: %+v", out)
	}

	// El admin quita el token y la IP, y muelle se reinicia dos veces.
	if _, err := e.do(t, dto.PatchSettingsInput{CloudflareToken: ptr(""), PublicIP: ptr("")}); err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		if out := boot(); out.CloudflareTokenSet || out.PublicIP != "" || out.ACMEEmail != "tu@correo.cl" {
			t.Fatalf("arranque %d después de quitar el token: %+v", i+2, out)
		}
	}
	if _, ok := e.raw(t, entity.SettingCloudflareToken); ok {
		t.Fatal("el token volvió a la base")
	}
	if n := len(admintestEvents(t, e)); n != 2 {
		t.Fatalf("eventos: %d (settings.init una sola vez + el PATCH)", n)
	}
}

// Una base que arrancó sin MUELLE_INIT_* ya tuvo su primer arranque: las
// variables que se agreguen después no hacen nada (los ajustes se cambian en
// el panel).
func TestApplyStartupSettingsLateInitIsIgnored(t *testing.T) {
	e := newProxyEnv(t)
	apply := NewApplyStartupSettings(e.repo, e.cipher, e.rec, e.db)
	ctx := context.Background()
	if err := apply.Execute(ctx, dto.StartupSettings{PanelDomain: "panel.tudominio.cl"}); err != nil {
		t.Fatal(err)
	}
	late := dto.InitialSettings{PublicIP: "203.0.113.7", CloudflareToken: "cf-token-1234567890abcdef"}
	if err := apply.Execute(ctx, dto.StartupSettings{PanelDomain: "panel.tudominio.cl", Init: late}); err != nil {
		t.Fatal(err)
	}
	out, _ := e.get.Execute(ctx, dto.GetSettingsInput{})
	if out.CloudflareTokenSet || out.PublicIP != "" || out.PanelDomain != "panel.tudominio.cl" {
		t.Fatalf("MUELLE_INIT_* después del primer arranque: %+v", out)
	}
	if n := len(admintestEvents(t, e)); n != 0 {
		t.Fatalf("eventos: %d", n)
	}
}
