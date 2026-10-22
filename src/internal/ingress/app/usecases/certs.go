package usecases

import (
	"context"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	sharedrepo "github.com/CPU-commits/muelle/src/shared/domain/repository"
)

// firstQueue es cuántas verificaciones de los 30 s pueden esperar: si se
// llena, la del dominio la cubre el barrido de los pending.
const firstQueue = 64

// CertChecker verifica los certificados que sirve Caddy (§8 del spec,
// contrato §3.1, D10 del plan de la Fase 3): a los 30 s de crear, adoptar o
// reintentar un dominio (ScheduleFirst), cada minuto los pending (Run) y
// cada hora todos los que corresponden (el job "certificados", CheckAll).
// Sondea fuera del lock de Ops y aplica la regla (service.ApplyCert) dentro.
// Con el proxy sin synced (deshabilitado, arrancando o recreándose) no hace
// nada: no se sabe. A un host que se sirve con Origin CA le fija la hoja que
// guardó muelle (§8 del diseño de Origin CA).
type CertChecker struct {
	probe    service.CertProbe
	repo     repository.Domains
	settings service.Settings
	origin   OriginRepos
	state    *ProxyState
	ops      *Ops
	clock    clock.Clock
	first    chan string
	// firstDelay es el plazo de ScheduleFirst (30 s), every el del barrido de
	// los pending (1 min) y opLimit el de Ops.Do al aplicar (30 s).
	firstDelay, every, opLimit time.Duration
	log                        *slog.Logger
}

// NewCertChecker recibe los ajustes y las tablas de Origin CA (para fijar
// la hoja) y los plazos de Timing: FirstCheck, PendingEvery y DomainOp.
func NewCertChecker(probe service.CertProbe, repo repository.Domains, settings service.Settings, origin OriginRepos,
	state *ProxyState, ops *Ops, clk clock.Clock, firstDelay, every, opLimit time.Duration, log *slog.Logger) *CertChecker {
	if log == nil {
		log = slog.Default()
	}
	return &CertChecker{
		probe: probe, repo: repo, settings: settings, origin: origin, state: state, ops: ops, clock: clk,
		first:      make(chan string, firstQueue),
		firstDelay: firstDelay, every: every, opLimit: opLimit, log: log,
	}
}

// EligibleHourly es el filtro del job "certificados": pending, active y
// error + certificate. Los error con cloudflare o record_exists no se miran:
// primero hay que arreglar el DNS (contrato §3.1).
func EligibleHourly(d entity.Domain) bool {
	return d.Status != entity.StatusError || d.ErrorCode == entity.CodeCertificate
}

// isPending es el filtro del barrido de cada minuto.
func isPending(d entity.Domain) bool { return d.Status == entity.StatusPending }

// ScheduleFirst agenda la verificación de los 30 s de hostname (la corre
// Run). No bloquea: si la cola está llena, la cubre el barrido.
func (c *CertChecker) ScheduleFirst(hostname string) {
	select {
	case c.first <- hostname:
	default:
	}
}

// Check verifica el dominio hostname: uno que ya no existe, o en error por
// el DNS, no se sondea.
func (c *CertChecker) Check(ctx context.Context, hostname string) error {
	if !c.state.Synced() {
		return nil
	}
	d, err := c.repo.Get(ctx, hostname)
	if errors.Is(err, sharedrepo.ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("ingress: leyendo el dominio %s: %w", hostname, err)
	}
	if !EligibleHourly(d) {
		return nil
	}
	return c.check(ctx, d)
}

// CheckAll verifica, uno por uno, los dominios que pasan include. Un dominio
// que falla no corta los demás.
func (c *CertChecker) CheckAll(ctx context.Context, include func(entity.Domain) bool) error {
	if !c.state.Synced() {
		return nil
	}
	all, err := c.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("ingress: leyendo los dominios: %w", err)
	}
	var errs []error
	for _, d := range all {
		if ctx.Err() != nil {
			break
		}
		if include(d) {
			errs = append(errs, c.check(ctx, d))
		}
	}
	return errors.Join(errs...)
}

// pinned son los DER de las hojas de Origin CA que se esperan para d: la
// del active de su hostname si, con los datos de ahora, el host se sirve con
// Origin CA (service.OriginServes, la misma regla que la configuración de
// Caddy); si no, ninguna. El proxy está synced (habilitado).
func (c *CertChecker) pinned(ctx context.Context, d entity.Domain) ([][]byte, error) {
	if !c.origin.set() {
		return nil, nil
	}
	p, err := c.settings.ProxySettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("ingress: leyendo los ajustes del proxy: %w", err)
	}
	if !service.OriginOn(p, true) {
		return nil, nil
	}
	active, err := c.origin.Certs.ActiveFor(ctx, d.Hostname)
	if err != nil || active == nil {
		return nil, err
	}
	h, err := c.origin.Hosts.Get(ctx, d.Hostname)
	if err != nil {
		return nil, err
	}
	if !service.OriginServes(service.DomainOriginTarget(d), h, active, c.clock.Now()) {
		return nil, nil
	}
	block, _ := pem.Decode([]byte(active.CertPEM))
	if block == nil {
		return nil, fmt.Errorf("ingress: el certificado de Origin CA de %s no es un PEM", d.Hostname)
	}
	return [][]byte{block.Bytes}, nil
}

// check sondea d fuera del lock y, con el lock tomado, aplica la regla solo
// si el dominio sigue igual (su UpdatedAt no cambió: un PATCH, un retry u
// otra verificación lo habrían movido) y el proxy sigue synced.
func (c *CertChecker) check(ctx context.Context, d entity.Domain) error {
	pin, err := c.pinned(ctx, d)
	if err != nil {
		return err
	}
	o, err := c.probe.Probe(ctx, d.Hostname, pin)
	if errors.Is(err, service.ErrCaddyUnreachable) {
		c.log.Debug("ingress: no se llegó a Caddy para verificar un certificado", "hostname", d.Hostname, "error", err)
		return nil
	}
	if err != nil {
		return fmt.Errorf("ingress: verificando el certificado de %s: %w", d.Hostname, err)
	}
	return c.ops.Do(ctx, c.opLimit, func(_, dbCtx context.Context) error {
		cur, err := c.repo.Get(dbCtx, d.Hostname)
		if errors.Is(err, sharedrepo.ErrNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("ingress: leyendo el dominio %s: %w", d.Hostname, err)
		}
		if cur.ID != d.ID || !cur.UpdatedAt.Equal(d.UpdatedAt) || !c.state.Synced() {
			return nil
		}
		if !service.ApplyCert(&cur, o, c.clock.Now(), false) {
			return nil
		}
		if err := c.repo.Update(dbCtx, cur); err != nil {
			return fmt.Errorf("ingress: guardando el certificado de %s: %w", d.Hostname, err)
		}
		return nil
	})
}

// checkLocked es la verificación de retry, que ya tiene el lock de Ops:
// sondea con opCtx y aplica la regla con retry = true sobre d, y lo guarda
// con dbCtx si cambió. repush: un error + certificate volvió a pending, y el
// que llama, ya fuera del lock (D6), reenvía la configuración forzando la
// recarga (Sync(force), contrato §3.2) para que Caddy reintente la emisión.
// Sin synced, o sin llegar a Caddy, no cambia nada.
func (c *CertChecker) checkLocked(opCtx, dbCtx context.Context, d *entity.Domain) (repush bool, err error) {
	if !c.state.Synced() {
		return false, nil
	}
	pin, err := c.pinned(dbCtx, *d)
	if err != nil {
		return false, err
	}
	o, err := c.probe.Probe(opCtx, d.Hostname, pin)
	if errors.Is(err, service.ErrCaddyUnreachable) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("ingress: verificando el certificado de %s: %w", d.Hostname, err)
	}
	wasCert := d.Status == entity.StatusError && d.ErrorCode == entity.CodeCertificate
	if !service.ApplyCert(d, o, c.clock.Now(), true) {
		return false, nil
	}
	if err := c.repo.Update(dbCtx, *d); err != nil {
		return false, fmt.Errorf("ingress: guardando el certificado de %s: %w", d.Hostname, err)
	}
	return wasCert && d.Status == entity.StatusPending, nil
}

// Run corre hasta que ctx se cancela: cada ScheduleFirst espera firstDelay
// y verifica ese dominio, y cada every barre los pending. Vuelve cuando
// terminaron las verificaciones en curso.
func (c *CertChecker) Run(ctx context.Context) {
	var wg sync.WaitGroup
	defer wg.Wait()
	tick := time.NewTicker(c.every)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case host := <-c.first:
			wg.Go(func() {
				if !sleep(ctx, c.firstDelay) {
					return
				}
				if err := c.Check(ctx, host); err != nil {
					c.log.Warn("ingress: verificando un certificado", "hostname", host, "error", err)
				}
			})
		case <-tick.C:
			if err := c.CheckAll(ctx, isPending); err != nil {
				c.log.Warn("ingress: verificando los certificados pending", "error", err)
			}
		}
	}
}
