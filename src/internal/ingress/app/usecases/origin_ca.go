package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// Los reintentos de una revocación que falla (§6 del diseño de Origin CA):
// de 5 min, doblando, hasta 24 h si Cloudflare no responde; 24 h si la
// rechaza.
const (
	revokeBackoffMin = 5 * time.Minute
	revokeBackoffMax = 24 * time.Hour
)

// OriginDeps es lo que necesita EnsureOriginCA.
type OriginDeps struct {
	Settings service.Settings
	Domains  repository.Domains
	Repos    OriginRepos
	DNS      service.DNS
	CA       service.OriginCA
	Keys     service.OriginKeys
	Syncer   *Syncer
	Clock    clock.Clock
	// DefImage es la imagen de Caddy de esta versión (naming.CaddyImageFor),
	// que decide si el proxy está habilitado.
	DefImage string
	// Pass es el plazo de una pasada (Timing.OriginPass, 5 min), y Grace el
	// margen entre el retiro y la revocación (Timing.OriginRevokeGrace, 1 min).
	Pass, Grace time.Duration
	Log         *slog.Logger
}

// EnsureOriginCA es el reconciliador de los certificados de Cloudflare
// Origin CA (contrato §3.7, §5.5 y §6 del diseño de Origin CA): lee el
// proxied de los registros, emite, renueva, retira y revoca. Una pasada a la
// vez (su mutex); escribe solo en sus tablas, sin el lock de Ops.
type EnsureOriginCA struct {
	d    OriginDeps
	mu   sync.Mutex
	kick chan struct{}
}

func NewEnsureOriginCA(d OriginDeps) *EnsureOriginCA {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &EnsureOriginCA{d: d, kick: make(chan struct{}, 1)}
}

// Kick despierta al reconciliador (Run) sin esperarlo; si ya hay uno
// pendiente, no agrega otro.
func (u *EnsureOriginCA) Kick() {
	select {
	case u.kick <- struct{}{}:
	default:
	}
}

// Run corre una pasada por cada Kick, cada una con el plazo de Pass, hasta
// que ctx se cancela.
func (u *EnsureOriginCA) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-u.kick:
			pctx, cancel := context.WithTimeout(ctx, u.d.Pass)
			if err := u.Execute(pctx); err != nil && ctx.Err() == nil {
				u.d.Log.Warn("ingress: la pasada de los certificados de Cloudflare", "error", err)
			}
			cancel()
		}
	}
}

// ResetBackoff pone el reintento de la emisión en ahora, en el host (o en
// todos con ""), y deja la falla: el aviso se sigue viendo hasta que la
// pasada emite o vuelve a fallar (§9.2 del diseño de Origin CA). No
// despierta al reconciliador: eso es Kick.
func (u *EnsureOriginCA) ResetBackoff(ctx context.Context, hostname string) error {
	if !u.d.Repos.set() {
		return nil
	}
	return u.d.Repos.Hosts.ResetBackoff(ctx, hostname, u.d.Clock.Now())
}

// ForgetObserved olvida lo que se vio en Cloudflare de hostname (un PATCH
// que cambia su proxied): la API dice issuing hasta la próxima lectura. La
// falla queda.
func (u *EnsureOriginCA) ForgetObserved(ctx context.Context, hostname string) error {
	if !u.d.Repos.set() {
		return nil
	}
	return u.d.Repos.Hosts.ForgetObserved(ctx, hostname, u.d.Clock.Now())
}

// Forget borra lo observado y las fallas de todos los hosts (al apagar la
// opción), sin el mutex de la pasada: no espera una pasada en curso. Los
// certificados retirados no se tocan: se revocan igual.
func (u *EnsureOriginCA) Forget(ctx context.Context) error {
	if !u.d.Repos.set() {
		return nil
	}
	return u.d.Repos.Hosts.DeleteAll(ctx)
}

// pass es el estado de una pasada.
type pass struct {
	now     time.Time
	token   string
	changed bool
	// stopped: un 429 (o el plazo) cortó la pasada: no más llamadas a
	// Cloudflare.
	stopped bool
}

// Execute es una pasada (§5.5 del diseño de Origin CA), con el mutex:
//  1. lee los ajustes y la zona del panel. Con el proxy deshabilitado no hace
//     nada; con la opción apagada o sin token, no hay candidatos;
//  2. lee los candidatos (los dominios y el panel), los active y lo
//     observado;
//  3. por cada candidato, por hostname: lee sus registros en Cloudflare y
//     guarda lo observado (si Cloudflare no responde, queda lo de antes; si
//     rechaza la lectura, unreadable). Con proxied, si le hace falta un
//     certificado (NeedsOriginCert) y su espera ya pasó, emite: la clave y
//     el CSR, POST /certificates (90 días), Check y guardar el active (que
//     retira el anterior). Una emisión que sale bien borra la falla; una que
//     falla la registra con su espera (OriginBackoff);
//  4. retira los active que no se van a servir: sin candidatos, el host que
//     ya no es candidato, lo observado en dns_only, missing o unreadable, o
//     uno vencido (nunca porque Cloudflare no respondió);
//  5. si algo cambió, manda la configuración (Syncer.Sync). Si falla, la
//     pasada termina sin revocar;
//  6. revoca los retirados que ya se pueden revocar (RetiredDue, con
//     LastGoodBuild y Grace): bien, 404 o 1014 borran la fila; un vencido se
//     borra sin llamar; sin token, quedan. Una que falla espera de 5 min a
//     24 h;
//  7. borra lo observado de los hostnames que no son candidatos y no tienen
//     certificados.
//
// Un 429 corta las llamadas a Cloudflare de la pasada (pasos 3 y 6): bloquea
// el token 5 min, también el DNS. Las escrituras en la base van sin la
// cancelación de ctx: una emisión que salió bien se guarda aunque venza el
// plazo.
func (u *EnsureOriginCA) Execute(ctx context.Context) error {
	if !u.d.Repos.set() {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	db := context.WithoutCancel(ctx)

	p, err := u.d.Settings.ProxySettings(db)
	if err != nil {
		return fmt.Errorf("ingress: leyendo los ajustes del proxy: %w", err)
	}
	if !p.Enabled(u.d.DefImage) {
		return nil
	}
	panelDNS, err := u.d.Settings.PanelDNS(db)
	if err != nil {
		return fmt.Errorf("ingress: leyendo la zona del panel: %w", err)
	}
	targets, err := u.targets(db, p, panelDNS.ZoneID)
	if err != nil {
		return err
	}
	active, err := u.d.Repos.Certs.Active(db)
	if err != nil {
		return fmt.Errorf("ingress: leyendo los certificados de Origin CA: %w", err)
	}
	hosts, err := u.d.Repos.Hosts.All(db)
	if err != nil {
		return fmt.Errorf("ingress: leyendo lo observado en Cloudflare: %w", err)
	}
	ps := &pass{now: u.d.Clock.Now(), token: p.CloudflareToken}

	// 3. Observar y emitir.
	if service.OriginOn(p, true) {
		for _, t := range targets {
			if ps.stopped || ctx.Err() != nil {
				ps.stopped = true
				break
			}
			if !t.Candidate() {
				continue
			}
			h := hosts[t.Hostname]
			h.Hostname = t.Hostname
			if err := u.observe(ctx, db, ps, t, &h); err != nil {
				return err
			}
			hosts[t.Hostname] = h
			var cur *entity.OriginCert
			if c, ok := active[t.Hostname]; ok {
				cur = &c
			}
			due := h.NextAttemptAt == nil || !h.NextAttemptAt.After(ps.now)
			if ps.stopped || h.Observed != entity.ObservedProxied || !service.NeedsOriginCert(t, cur, ps.now) || !due {
				continue
			}
			c, err := u.issue(ctx, db, ps, t, h)
			if err != nil {
				return err
			}
			if c != nil {
				active[t.Hostname] = *c
			}
		}
	}

	// 4. Retirar lo que no se va a servir.
	on := service.OriginOn(p, true)
	byHost := map[string]service.OriginTarget{}
	for _, t := range targets {
		byHost[t.Hostname] = t
	}
	for _, c := range sortedCerts(active) {
		t, ok := byHost[c.Hostname]
		obs := hosts[c.Hostname].Observed
		keep := on && ok && t.Candidate() && c.ZoneID == t.ZoneID && c.ExpiresAt.After(ps.now) &&
			obs != entity.ObservedDNSOnly && obs != entity.ObservedMissing && obs != entity.ObservedUnreadable
		if keep {
			continue
		}
		if err := u.d.Repos.Certs.Retire(db, c.ID, ps.now); err != nil {
			return fmt.Errorf("ingress: retirando el certificado de Origin CA de %s: %w", c.Hostname, err)
		}
		u.d.Log.Info("ingress: certificado de Cloudflare retirado", "hostname", c.Hostname, "cf_cert_id", c.CFCertID)
		ps.changed = true
	}

	// 5. Mandar la configuración.
	if ps.changed {
		if err := u.d.Syncer.Sync(db, false); err != nil {
			return fmt.Errorf("ingress: la pasada de Origin CA no revoca nada: %w", err)
		}
	}

	// 6. Revocar.
	if err := u.revoke(ctx, db, ps); err != nil {
		return err
	}

	// 7. Olvidar lo observado de lo que ya no es candidato ni tiene certificados.
	return u.cleanup(db, targets, on)
}

// targets son los hosts que pueden usar Origin CA, por hostname: los
// dominios y el panel (un dominio con el hostname del panel no cuenta: la
// ruta del panel gana).
func (u *EnsureOriginCA) targets(ctx context.Context, p shared.ProxySettings, panelZone string) ([]service.OriginTarget, error) {
	domains, err := u.d.Domains.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("ingress: leyendo los dominios: %w", err)
	}
	var out []service.OriginTarget
	for _, d := range domains {
		if p.PanelDomain != "" && strings.EqualFold(d.Hostname, p.PanelDomain) {
			continue
		}
		out = append(out, service.DomainOriginTarget(d))
	}
	if p.PanelDomain != "" {
		out = append(out, service.PanelOriginTarget(p.PanelDomain, panelZone))
	}
	slices.SortFunc(out, func(a, b service.OriginTarget) int { return strings.Compare(a.Hostname, b.Hostname) })
	return out, nil
}

// observe lee los registros de t en Cloudflare y guarda lo observado en h y
// en la base. Si Cloudflare no responde, h queda como estaba (con un 429,
// además, se corta la pasada); si rechaza la lectura, unreadable.
func (u *EnsureOriginCA) observe(ctx, db context.Context, ps *pass, t service.OriginTarget, h *entity.OriginHost) error {
	records, err := u.d.DNS.Records(ctx, ps.token, t.ZoneID, t.Hostname)
	var obs entity.Observation
	switch {
	case err == nil:
		obs = service.ObserveRecords(records, t.RecordID)
	case isRejection(err):
		obs = entity.ObservedUnreadable
	default:
		if errors.Is(err, service.ErrRateLimited) || ctx.Err() != nil {
			ps.stopped = true
		}
		u.d.Log.Debug("ingress: Cloudflare no respondió al leer los registros; vale lo último que se vio", "hostname", t.Hostname, "error", err)
		return nil
	}
	if obs != h.Observed {
		ps.changed = true
	}
	if err := u.d.Repos.Hosts.SetObserved(db, t.Hostname, obs, ps.now); err != nil {
		return fmt.Errorf("ingress: guardando lo observado de %s: %w", t.Hostname, err)
	}
	h.Observed = obs
	return nil
}

// issue emite (o renueva) el certificado de t. Devuelve el active nuevo, o
// nil si la emisión falló (la falla queda en origin_ca_hosts). El error es
// solo de la base.
func (u *EnsureOriginCA) issue(ctx, db context.Context, ps *pass, t service.OriginTarget, h entity.OriginHost) (*entity.OriginCert, error) {
	keyPEM, csrPEM, err := u.d.Keys.Generate(t.Hostname)
	if err != nil {
		return nil, fmt.Errorf("ingress: la clave de %s: %w", t.Hostname, err)
	}
	issued, err := u.d.CA.CreateOriginCert(ctx, ps.token, t.Hostname, csrPEM, service.OriginCAValidityDays)
	var leafPEM string
	var notAfter time.Time
	if err == nil {
		leafPEM, notAfter, err = u.d.Keys.Check(issued.CertPEM, keyPEM, t.Hostname, ps.now)
	}
	if err != nil {
		if errors.Is(err, service.ErrRateLimited) {
			ps.stopped = true
		} else if ctx.Err() != nil {
			// El plazo de la pasada: no es una falla del host.
			ps.stopped = true
			return nil, nil
		}
		code := service.ClassifyOriginError(err)
		failures := h.Failures + 1
		next := ps.now.Add(service.OriginBackoff(failures, code))
		if serr := u.d.Repos.Hosts.RecordFailure(db, t.Hostname, failures, code, service.OriginFailureCause(err, t), next, ps.now); serr != nil {
			return nil, fmt.Errorf("ingress: guardando la falla de %s: %w", t.Hostname, serr)
		}
		u.d.Log.Warn("ingress: no se pudo emitir el certificado de Cloudflare", "hostname", t.Hostname, "code", string(code),
			"failures", failures, "retry_at", next)
		return nil, nil
	}
	c := entity.OriginCert{Hostname: t.Hostname, ZoneID: t.ZoneID, CFCertID: issued.ID, CertPEM: leafPEM, KeyPEM: keyPEM,
		ExpiresAt: notAfter, CreatedAt: ps.now}
	if err := u.d.Repos.Certs.Insert(db, &c); err != nil {
		return nil, fmt.Errorf("ingress: guardando el certificado de Origin CA de %s: %w", t.Hostname, err)
	}
	if h.Failed() {
		if err := u.d.Repos.Hosts.ClearFailure(db, t.Hostname, ps.now); err != nil {
			return nil, fmt.Errorf("ingress: borrando la falla de %s: %w", t.Hostname, err)
		}
	}
	u.d.Log.Info("ingress: certificado de Cloudflare emitido", "hostname", t.Hostname, "cf_cert_id", issued.ID, "expires_at", notAfter)
	ps.changed = true
	c.KeyPEM = ""
	return &c, nil
}

// revoke revoca los retirados que ya se pueden revocar (§6 del diseño).
func (u *EnsureOriginCA) revoke(ctx, db context.Context, ps *pass) error {
	due, err := u.d.Repos.Certs.RetiredDue(db, ps.now, u.d.Syncer.LastGoodBuild(), u.d.Grace)
	if err != nil {
		return fmt.Errorf("ingress: leyendo los certificados de Origin CA retirados: %w", err)
	}
	for _, c := range due {
		if c.ExpiresAt.Before(ps.now) {
			if err := u.d.Repos.Certs.Delete(db, c.ID); err != nil {
				return fmt.Errorf("ingress: borrando un certificado de Origin CA vencido: %w", err)
			}
			u.d.Log.Info("ingress: certificado de Cloudflare vencido, borrado sin revocar", "hostname", c.Hostname, "cf_cert_id", c.CFCertID)
			continue
		}
		if ps.token == "" || ps.stopped || ctx.Err() != nil {
			continue
		}
		err := u.d.CA.RevokeOriginCert(ctx, ps.token, c.CFCertID)
		de, isDNS := errors.AsType[*service.DNSError](err)
		if err == nil || errors.Is(err, service.ErrDNSNotFound) || (isDNS && de.Code == 1014) {
			if err := u.d.Repos.Certs.Delete(db, c.ID); err != nil {
				return fmt.Errorf("ingress: borrando un certificado de Origin CA revocado: %w", err)
			}
			u.d.Log.Info("ingress: certificado de Cloudflare revocado", "hostname", c.Hostname, "cf_cert_id", c.CFCertID)
			continue
		}
		if errors.Is(err, service.ErrRateLimited) || ctx.Err() != nil {
			ps.stopped = true
		}
		failures := c.RevokeFailures + 1
		wait := revokeBackoffMax
		if !isRejection(err) {
			wait = revokeBackoffMin
			for i := 1; i < failures && wait < revokeBackoffMax; i++ {
				wait *= 2
			}
			wait = min(wait, revokeBackoffMax)
		}
		if err := u.d.Repos.Certs.RevokeFailed(db, c.ID, failures, ps.now.Add(wait)); err != nil {
			return fmt.Errorf("ingress: guardando la revocación que falló: %w", err)
		}
		u.d.Log.Warn("ingress: no se pudo revocar un certificado de Cloudflare; se reintenta", "hostname", c.Hostname,
			"cf_cert_id", c.CFCertID, "failures", failures, "error", err)
	}
	return nil
}

// cleanup borra lo observado de los hostnames que no son candidatos (con la
// opción en uso) y no tienen certificados, active ni retirados.
func (u *EnsureOriginCA) cleanup(db context.Context, targets []service.OriginTarget, on bool) error {
	hosts, err := u.d.Repos.Hosts.All(db)
	if err != nil {
		return fmt.Errorf("ingress: leyendo lo observado en Cloudflare: %w", err)
	}
	if len(hosts) == 0 {
		return nil
	}
	keep := map[string]bool{}
	if on {
		for _, t := range targets {
			if t.Candidate() {
				keep[t.Hostname] = true
			}
		}
	}
	active, err := u.d.Repos.Certs.Active(db)
	if err != nil {
		return err
	}
	retired, err := u.d.Repos.Certs.Retired(db)
	if err != nil {
		return err
	}
	for h := range active {
		keep[h] = true
	}
	for _, c := range retired {
		keep[c.Hostname] = true
	}
	for h := range hosts {
		if !keep[h] {
			if err := u.d.Repos.Hosts.Delete(db, h); err != nil {
				return fmt.Errorf("ingress: borrando lo observado de %s: %w", h, err)
			}
		}
	}
	return nil
}

// sortedCerts son los certificados del mapa, por hostname.
func sortedCerts(m map[string]entity.OriginCert) []entity.OriginCert {
	out := make([]entity.OriginCert, 0, len(m))
	for _, c := range m {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b entity.OriginCert) int { return strings.Compare(a.Hostname, b.Hostname) })
	return out
}
