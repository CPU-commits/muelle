package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/app/dto"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// ProxyJob es el job "proxy" (contrato §1.2): cada 5 min, y al arrancar,
// lleva Docker y Caddy a lo que dice la base.
type ProxyJob struct {
	settings service.Settings
	catalog  service.Catalog
	networks *EnsureStageNetwork
	caddy    *EnsureCaddy
	orphans  *RemoveOrphanNetworks
	syncer   *Syncer
	panel    *EnsurePanelDNS
	origin   *EnsureOriginCA
	state    *ProxyState
	defImage string
	lo, hi   time.Duration
	log      *slog.Logger
}

// NewProxyJob recibe el reconciliador de Origin CA, la imagen de Caddy de
// esta versión (naming.CaddyImageFor), que decide si el proxy está
// habilitado, y el backoff con que espera a un muelle-caddy recién creado
// (Timing.RetryMin y RetryMax).
func NewProxyJob(settings service.Settings, catalog service.Catalog, networks *EnsureStageNetwork, caddy *EnsureCaddy,
	orphans *RemoveOrphanNetworks, syncer *Syncer, panel *EnsurePanelDNS, origin *EnsureOriginCA, state *ProxyState,
	defImage string, retryMin, retryMax time.Duration, log *slog.Logger) *ProxyJob {
	return &ProxyJob{settings: settings, catalog: catalog, networks: networks, caddy: caddy, orphans: orphans,
		syncer: syncer, panel: panel, origin: origin, state: state, defImage: defImage, lo: retryMin, hi: retryMax, log: log}
}

// Run, en el orden de D12 del plan de la Fase 3:
//  1. con el proxy deshabilitado (desarrollo), no hace nada;
//  2. asegura las redes de todos los stages (y conecta muelle-caddy);
//  3. asegura muelle-caddy, salvo que haya una recreación agendada o frenada
//     por un hook: esa la hace el Recreator, después de que el hook vuelve;
//  4. borra las redes de stage huérfanas;
//  5. le manda la configuración a Caddy; si recién lo creó, espera a que su
//     admin API responda (syncStarting);
//  6. con token, Cloudflare: cloudflare_zones y el registro del panel
//     (EnsurePanelDNS). Si cambió la zona del panel, y con ella su TLS, manda
//     la configuración otra vez. Va al final: un Cloudflare lento no atrasa
//     la configuración de los sitios;
//  7. despierta al reconciliador de Origin CA, sin esperarlo (contrato §1.2):
//     con la opción encendida lee el proxied de los registros, emite y
//     renueva; con la opción apagada solo revoca lo que quedó retirado.
//
// Un paso que falla no corta los que siguen: los errores se juntan
// (errors.Join), y cada uno ya quedó en el log y, si es de Caddy, en el
// estado de GET /proxy.
func (j *ProxyJob) Run(ctx context.Context) error {
	p, err := j.settings.ProxySettings(ctx)
	if err != nil {
		return fmt.Errorf("ingress: leyendo los ajustes del proxy: %w", err)
	}
	if !p.Enabled(j.defImage) {
		return nil
	}
	var errs []error
	if err := j.ensureNetworks(ctx); err != nil {
		errs = append(errs, err)
	}
	recreated := false
	if j.state.Recreating() {
		j.log.Debug("ingress: hay una recreación de muelle-caddy pendiente; el job no lo toca")
	} else if recreated, err = j.caddy.Execute(ctx); err != nil {
		errs = append(errs, err)
	}
	if err := j.orphans.Execute(ctx); err != nil {
		j.log.Warn("ingress: borrando las redes de stage huérfanas", "error", err)
		errs = append(errs, err)
	}
	if recreated {
		err = syncStarting(ctx, j.syncer, j.lo, j.hi)
	} else {
		err = j.syncer.Sync(ctx, false)
	}
	if err != nil {
		errs = append(errs, err)
	}
	if p.CloudflareToken != "" {
		zoneChanged, err := j.panel.Execute(ctx)
		if err != nil {
			j.log.Warn("ingress: el registro DNS del panel", "error", err)
			errs = append(errs, err)
		}
		if zoneChanged {
			if err := j.syncer.Sync(ctx, false); err != nil {
				errs = append(errs, err)
			}
		}
	}
	j.origin.Kick()
	return errors.Join(errs...)
}

// ensureNetworks asegura la red de cada stage. Una que falla no corta las
// demás, salvo Docker caído: ahí fallarían todas.
func (j *ProxyJob) ensureNetworks(ctx context.Context) error {
	stages, err := j.catalog.Stages(ctx)
	if err != nil {
		return fmt.Errorf("ingress: leyendo los stages: %w", err)
	}
	var errs []error
	for _, s := range stages {
		err := j.networks.Execute(ctx, dto.StageRef{Project: s.Project, Stage: s.Stage})
		if err == nil {
			continue
		}
		err = fmt.Errorf("ingress: la red del stage %s/%s: %w", s.Project, s.Stage, err)
		j.log.Warn("ingress: asegurando la red de un stage", "error", err)
		errs = append(errs, err)
		if errors.Is(err, shared.ErrDockerUnavailable) {
			break
		}
	}
	return errors.Join(errs...)
}
