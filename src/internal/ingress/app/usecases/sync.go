package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// syncTimeout es el plazo de un Sync, desde que toma el mutex.
const syncTimeout = 30 * time.Second

// Los mensajes del estado del proxy (GET /proxy) cuando falla un Sync. El de
// un rechazo de Caddy es el suyo (sin el token).
const (
	msgCaddyUnreachable = "muelle no pudo conectarse con la admin API de Caddy; se reintenta solo"
	msgPlaceholder      = "el email ACME o el token de Cloudflare tienen { o }, que Caddy tomaría como placeholders; corrígelo en Ajustes"
	msgSyncInternal     = "error interno armando la configuración"
)

// Syncer arma la configuración completa de Caddy desde la base y la manda
// (§8 del spec). Las escrituras a Caddy pasan por su mutex: un POST /load a
// la vez, y la configuración se arma con el mutex tomado, así uno viejo
// nunca pisa uno nuevo (D14 del plan de la Fase 3).
type Syncer struct {
	mu         sync.Mutex
	settings   service.Settings
	domains    repository.Domains
	catalog    service.Catalog
	admin      service.CaddyAdmin
	state      *ProxyState
	clock      clock.Clock
	defImage   string
	internalCA bool
	onFail     func()
	log        *slog.Logger
}

// NewSyncer arma el Syncer. defImage es la imagen de Caddy de esta versión
// (naming.CaddyImageFor), que decide si el proxy está habilitado; internalCA
// pide la CA interna de Caddy (solo integración); onFail se llama después de
// cada Sync que falla (los reintentos con backoff).
func NewSyncer(settings service.Settings, domains repository.Domains, catalog service.Catalog, admin service.CaddyAdmin,
	state *ProxyState, clk clock.Clock, defImage string, internalCA bool, onFail func(), log *slog.Logger) *Syncer {
	if onFail == nil {
		onFail = func() {}
	}
	return &Syncer{
		settings: settings, domains: domains, catalog: catalog, admin: admin, state: state, clock: clk,
		defImage: defImage, internalCA: internalCA, onFail: onFail, log: log,
	}
}

// Sync, con el mutex tomado y un plazo de 30 s: lee los ajustes (sin el
// proxy habilitado devuelve nil sin mandar nada, contrato §3.4), la zona del
// panel, los dominios y las apps, arma la configuración (BuildProxyConfig) y
// la manda. force recarga aunque Caddy ya tenga la misma. Un error queda en
// el estado y en el log (nunca la configuración ni el token), llama a onFail
// y se devuelve.
func (s *Syncer) Sync(ctx context.Context, force bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, syncTimeout)
	defer cancel()

	p, err := s.settings.ProxySettings(ctx)
	if err != nil {
		return s.fail(fmt.Errorf("leyendo los ajustes del proxy: %w", err))
	}
	if !p.Enabled(s.defImage) {
		return nil
	}
	cfg, err := s.build(ctx, p)
	if err != nil {
		return s.fail(err)
	}
	if err := s.admin.Load(ctx, cfg, force); err != nil {
		return s.fail(err)
	}
	s.state.Loaded(s.clock.Now())
	return nil
}

// startingWindow es cuánto se reintenta mandarle la configuración a un
// muelle-caddy recién creado mientras su admin API todavía no responde.
const startingWindow = 30 * time.Second

// syncStarting es Sync para un muelle-caddy recién creado: Caddy tarda un
// momento en abrir el socket de la admin API, así que mientras Sync dé
// ErrCaddyUnreachable reintenta con backoff (lo, 2·lo… hasta hi), hasta
// startingWindow. Así el primer arranque, y la recreación (que mientras
// tanto dice recreating), quedan synced sin un out_of_sync de paso.
// Devuelve el último error.
func syncStarting(ctx context.Context, s *Syncer, lo, hi time.Duration) error {
	deadline := time.Now().Add(startingWindow)
	for d := time.Duration(0); ; {
		err := s.Sync(ctx, false)
		if err == nil || !errors.Is(err, service.ErrCaddyUnreachable) || time.Now().After(deadline) {
			return err
		}
		d = nextBackoff(d, lo, hi)
		if !sleep(ctx, d) {
			return err
		}
	}
}

// build lee lo demás de la base y arma la configuración.
func (s *Syncer) build(ctx context.Context, p shared.ProxySettings) (service.ProxyConfig, error) {
	dns, err := s.settings.PanelDNS(ctx)
	if err != nil {
		return service.ProxyConfig{}, fmt.Errorf("leyendo la zona del panel: %w", err)
	}
	domains, err := s.domains.List(ctx)
	if err != nil {
		return service.ProxyConfig{}, fmt.Errorf("leyendo los dominios: %w", err)
	}
	apps, err := s.catalog.Apps(ctx)
	if err != nil {
		return service.ProxyConfig{}, fmt.Errorf("leyendo las apps: %w", err)
	}
	return service.BuildProxyConfig(p, dns.ZoneID, domains, apps, s.internalCA, service.OriginInputs{}), nil
}

// fail deja el error en el estado y en el log, y llama a onFail.
func (s *Syncer) fail(err error) error {
	var rej *service.CaddyRejected
	var msg string
	switch {
	case errors.As(err, &rej):
		msg = rej.Message
		s.log.Warn("ingress: Caddy no aceptó la configuración", "error", msg)
	case errors.Is(err, service.ErrCaddyUnreachable):
		msg = msgCaddyUnreachable
		s.log.Warn("ingress: no se pudo hablar con la admin API de Caddy", "error", err)
	case errors.Is(err, service.ErrPlaceholderInSettings):
		msg = msgPlaceholder
		s.log.Warn("ingress: la configuración de Caddy no se mandó", "error", msg)
	default:
		msg = msgSyncInternal
		s.log.Error("ingress: armando la configuración de Caddy", "error", err)
	}
	s.state.LoadFailed(msg)
	s.onFail()
	return fmt.Errorf("ingress: sincronizando Caddy: %w", err)
}
