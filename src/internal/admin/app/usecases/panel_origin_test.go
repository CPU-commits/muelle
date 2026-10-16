package usecases

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/app/dto"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// countingRepo cuenta las lecturas, para ver cuándo GetPanelOrigin va a la
// base. Con reached, cada Get avisa ahí después de leer y espera release
// antes de volver.
type countingRepo struct {
	repository.SettingsRepository
	gets    atomic.Int64
	reached chan string
	release chan struct{}
}

func (r *countingRepo) Get(ctx context.Context, key string) ([]byte, bool, error) {
	r.gets.Add(1)
	v, enc, err := r.SettingsRepository.Get(ctx, key)
	if r.reached != nil {
		r.reached <- key
		<-r.release
	}
	return v, enc, err
}

func (e *proxyEnv) originOK(t *testing.T, get *GetPanelOrigin) string {
	t.Helper()
	o, err := get.Execute(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// El origen en que muelle-caddy sirve el panel sigue a los ajustes: install
// con --https-port 8443 (spec §16 paso 2) y después 443 en el panel (paso 5).
// El PATCH lo invalida, así que el origen nuevo vale enseguida.
func TestGetPanelOrigin(t *testing.T) {
	e := newProxyEnv(t)
	ctx := context.Background()
	if o := e.originOK(t, e.origin); o != "" {
		t.Fatalf("sin dominio del panel: %q", o)
	}
	apply := NewApplyStartupSettings(e.repo, e.cipher, e.rec, e.db)
	init := dto.InitialSettings{ProxyHTTPPort: 8080, ProxyHTTPSPort: 8443}
	if err := apply.Execute(ctx, dto.StartupSettings{PanelDomain: "panel.tudominio.cl", Init: init}); err != nil {
		t.Fatal(err)
	}
	e.origin.Invalidate() // lo hace admin.Module.ApplyStartupSettings
	if o := e.originOK(t, e.origin); o != "https://panel.tudominio.cl:8443" {
		t.Fatalf("después de install: %q", o)
	}
	if _, err := e.do(t, dto.PatchSettingsInput{ProxyHTTPPort: ptr(80), ProxyHTTPSPort: ptr(443)}); err != nil {
		t.Fatal(err)
	}
	if o := e.originOK(t, e.origin); o != "https://panel.tudominio.cl" {
		t.Fatalf("después de cambiar los puertos: %q", o)
	}
	// Un valor guardado que no se entiende es un error, no un origen inventado.
	if err := e.repo.Set(ctx, entity.SettingProxyHTTPSPort, []byte("x"), false); err != nil {
		t.Fatal(err)
	}
	e.origin.Invalidate()
	if o, err := e.origin.Execute(ctx); err == nil || o != "" {
		t.Fatalf("puerto inválido: %q %v", o, err)
	}
}

// Cada petición que modifica algo pregunta el origen: no va a la base cada
// vez. Lo que cambia fuera de muelle (el UPDATE a mano del contrato §4.1) se
// ve a más tardar en PanelOriginTTL; lo que cambia por el PATCH, enseguida, y
// un PATCH que no toca el proxy no invalida nada.
func TestGetPanelOriginCache(t *testing.T) {
	e := newProxyEnv(t)
	ctx := context.Background()
	if err := e.repo.Set(ctx, entity.SettingPanelDomain, []byte("panel.test"), false); err != nil {
		t.Fatal(err)
	}
	repo := &countingRepo{SettingsRepository: e.repo}
	get := NewGetPanelOrigin(repo, e.clk)
	if o := e.originOK(t, get); o != "https://panel.test" {
		t.Fatalf("origen: %q", o)
	}
	n := repo.gets.Load()
	for range 5 {
		if o := e.originOK(t, get); o != "https://panel.test" {
			t.Fatalf("origen: %q", o)
		}
	}
	if got := repo.gets.Load(); got != n {
		t.Fatalf("con el valor en caché, leyó la base %d veces", got-n)
	}

	if err := e.repo.Set(ctx, entity.SettingProxyHTTPSPort, []byte("9443"), false); err != nil {
		t.Fatal(err)
	}
	e.clk.Advance(PanelOriginTTL - time.Millisecond)
	if o := e.originOK(t, get); o != "https://panel.test" {
		t.Fatalf("antes del TTL: %q", o)
	}
	e.clk.Advance(time.Millisecond)
	if o := e.originOK(t, get); o != "https://panel.test:9443" {
		t.Fatalf("después del TTL: %q", o)
	}

	n = repo.gets.Load()
	e.origin = get
	e.patch = NewPatchSettings(e.repo, e.patch.totp, e.cipher, e.hook, get, e.rec, e.db)
	if _, err := e.do(t, dto.PatchSettingsInput{Require2FA: ptr(false)}); err != nil {
		t.Fatal(err)
	}
	e.originOK(t, get)
	if got := repo.gets.Load(); got != n {
		t.Fatalf("un PATCH sin cambios del proxy invalidó el origen (%d lecturas)", got-n)
	}
	if _, err := e.do(t, dto.PatchSettingsInput{ProxyHTTPSPort: ptr(10443)}); err != nil {
		t.Fatal(err)
	}
	if o := e.originOK(t, get); o != "https://panel.test:10443" {
		t.Fatalf("después del PATCH: %q", o)
	}
}

// Un error no queda en caché: la próxima petición vuelve a leer.
func TestGetPanelOriginErrorNotCached(t *testing.T) {
	e := newProxyEnv(t)
	ctx := context.Background()
	for k, v := range map[string]string{entity.SettingPanelDomain: "panel.test", entity.SettingProxyHTTPSPort: "x"} {
		if err := e.repo.Set(ctx, k, []byte(v), false); err != nil {
			t.Fatal(err)
		}
	}
	get := NewGetPanelOrigin(e.repo, e.clk)
	if _, err := get.Execute(ctx); err == nil {
		t.Fatal("puerto inválido sin error")
	}
	if err := e.repo.Set(ctx, entity.SettingProxyHTTPSPort, []byte("8443"), false); err != nil {
		t.Fatal(err)
	}
	if o := e.originOK(t, get); o != "https://panel.test:8443" {
		t.Fatalf("después de arreglarlo: %q", o)
	}
}

// Una lectura que empezó antes de un Invalidate puede responder lo que leyó,
// pero no lo deja en caché: si no, el origen viejo seguiría valiendo hasta el
// TTL después de un PATCH que se cruzó con ella.
func TestGetPanelOriginInvalidateDuringLoad(t *testing.T) {
	e := newProxyEnv(t)
	ctx := context.Background()
	if err := e.repo.Set(ctx, entity.SettingPanelDomain, []byte("panel.test"), false); err != nil {
		t.Fatal(err)
	}
	repo := &countingRepo{SettingsRepository: e.repo, reached: make(chan string), release: make(chan struct{})}
	get := NewGetPanelOrigin(repo, e.clk)
	done := make(chan string)
	go func() {
		o, _ := get.Execute(ctx)
		done <- o
	}()
	<-repo.reached
	repo.release <- struct{}{}
	<-repo.reached // ya leyó las dos claves, con el puerto de antes
	if err := e.repo.Set(ctx, entity.SettingProxyHTTPSPort, []byte("9443"), false); err != nil {
		t.Fatal(err)
	}
	get.Invalidate()
	repo.release <- struct{}{}
	if o := <-done; o != "https://panel.test" {
		t.Fatalf("la lectura cruzada: %q", o)
	}
	repo.reached = nil
	if o := e.originOK(t, get); o != "https://panel.test:9443" {
		t.Fatalf("quedó en caché lo leído antes del Invalidate: %q", o)
	}
}
