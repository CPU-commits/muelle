package usecases

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// EnsurePanelDNS lleva el registro DNS del panel y cloudflare_zones a lo que
// dicen los ajustes (contrato §3.4 y §3.6; D27 a D30 del plan de la Fase 3).
// Lo llaman el job "proxy" y ProxySettingsChanged.
type EnsurePanelDNS struct {
	settings service.Settings
	dns      service.DNS
	ops      *Ops
	limit    time.Duration
	log      *slog.Logger
}

// NewEnsurePanelDNS recibe el plazo de cada pasada (Timing.DomainOp): el ctx
// del job no tiene plazo, y sin el suyo podría tener el lock de Ops por minutos.
func NewEnsurePanelDNS(settings service.Settings, dns service.DNS, ops *Ops, limit time.Duration, log *slog.Logger) *EnsurePanelDNS {
	return &EnsurePanelDNS{settings: settings, dns: dns, ops: ops, limit: limit, log: log}
}

// Execute, con el lock de Ops (D6) y los ajustes leídos dentro (D30):
//  1. sin token, vuelve sin llamar a Cloudflare;
//  2. lista las zonas y guarda cloudflare_zones, solo si el token guardado
//     sigue siendo el que las listó (SaveZones, D28);
//  3. lee los registros guardados (owned): uno que ya no está, que no es de
//     panel_domain o que el token no puede leer se olvida sin borrarlo
//     (D27); si el A ya no está, su AAAA se borra (D29). Si Cloudflare no
//     responde, vuelve con el error sin guardar nada;
//  4. la zona de panel_domain, que se guarda aunque el registro sea ajeno
//     (decide el TLS del panel); sin zona, olvida todo;
//  5. con public_ip, crea el A (proxied false) si no hay otro A, AAAA ni
//     CNAME de ese nombre, o le cambia el contenido al propio (nunca el
//     proxied); el AAAA sigue a public_ipv6, solo si muelle maneja el A, y
//     se crea con el proxied del A. Sin public_ip no toca registros.
//
// Guarda las claves panel_cf_* que cambiaron, también si un paso de 5 falló.
// zoneChanged: cambió panel_cf_zone_id, y con eso el TLS del panel (el que
// llama manda la configuración a Caddy). Los errores nunca traen el token.
func (u *EnsurePanelDNS) Execute(ctx context.Context) (zoneChanged bool, err error) {
	err = u.ops.Do(ctx, u.limit, func(opCtx, dbCtx context.Context) error {
		p, err := u.settings.ProxySettings(dbCtx)
		if err != nil {
			return fmt.Errorf("ingress: leyendo los ajustes del proxy: %w", err)
		}
		if p.CloudflareToken == "" {
			return nil
		}
		stored, err := u.settings.PanelDNS(dbCtx)
		if err != nil {
			return fmt.Errorf("ingress: leyendo las claves del panel: %w", err)
		}
		zones, err := u.dns.Zones(opCtx, p.CloudflareToken)
		if err != nil {
			return fmt.Errorf("ingress: el registro DNS del panel: %w", err)
		}
		if _, err := u.settings.SaveZones(dbCtx, p.CloudflareToken, len(zones)); err != nil {
			return fmt.Errorf("ingress: guardando cloudflare_zones: %w", err)
		}
		own, err := u.owned(opCtx, p, stored)
		if err != nil {
			return fmt.Errorf("ingress: el registro DNS del panel: %w", err)
		}
		next, ensureErr := u.ensure(opCtx, p, zones, own)
		if next != stored {
			if err := u.settings.SavePanelDNS(dbCtx, next); err != nil {
				return errors.Join(ensureErr, fmt.Errorf("ingress: guardando las claves del panel: %w", err))
			}
			u.log.Info("ingress: el registro DNS del panel cambió", "zona", next.ZoneID, "a", next.RecordID, "aaaa", next.RecordIDAAAA)
		}
		zoneChanged = next.ZoneID != stored.ZoneID
		if ensureErr != nil {
			return fmt.Errorf("ingress: el registro DNS del panel: %w", ensureErr)
		}
		return nil
	})
	return zoneChanged, err
}

// panelOwned son los registros del panel que maneja muelle, como están en
// Cloudflare (nil: no maneja ese tipo), en zoneID.
type panelOwned struct {
	zoneID  string
	a, aaaa *service.Record
}

// readState es lo que se encontró al leer un registro guardado.
type readState int

const (
	recOK         readState = iota // existe, de ese nombre y ese tipo
	recGone                        // ya no existe (404)
	recOther                       // es de otro nombre o de otro tipo
	recUnreadable                  // el token no lo puede leer (Cloudflare rechazó la lectura)
)

// owned lee los ids guardados en su zona (D27 y D29) y devuelve los que
// siguen siendo del panel. Uno que no, se olvida sin borrarlo. Si el A ya no
// existe, su AAAA (si sigue y es del panel) se borra: si no, contaría como un
// registro ajeno. Un A olvidado por otra razón olvida también su AAAA, sin
// leerlo. Error: Cloudflare no respondió (no se olvida nada).
func (u *EnsurePanelDNS) owned(ctx context.Context, p shared.ProxySettings, s service.PanelDNS) (panelOwned, error) {
	out := panelOwned{zoneID: s.ZoneID}
	if s.ZoneID == "" || p.PanelDomain == "" {
		return out, nil
	}
	aGone := false
	if s.RecordID != "" {
		r, st, err := u.read(ctx, p, s.ZoneID, s.RecordID, typeA)
		if err != nil {
			return panelOwned{}, err
		}
		switch st {
		case recOK:
			out.a = &r
		case recGone:
			aGone = true
		}
	}
	if s.RecordIDAAAA == "" || (out.a == nil && !aGone) {
		return out, nil
	}
	r, st, err := u.read(ctx, p, s.ZoneID, s.RecordIDAAAA, typeAAAA)
	if err != nil {
		return panelOwned{}, err
	}
	switch {
	case st != recOK:
	case out.a != nil:
		out.aaaa = &r
	default:
		err := u.dns.DeleteRecord(ctx, p.CloudflareToken, s.ZoneID, r.ID)
		if err != nil && !errors.Is(err, service.ErrDNSNotFound) {
			u.log.Warn("ingress: el A del panel ya no está y su AAAA no se pudo borrar: queda en Cloudflare", "error", err)
		}
	}
	return out, nil
}

// read lee el registro guardado id y dice si sigue siendo del panel.
func (u *EnsurePanelDNS) read(ctx context.Context, p shared.ProxySettings, zoneID, id, typ string) (service.Record, readState, error) {
	r, err := u.dns.Record(ctx, p.CloudflareToken, zoneID, id)
	var st readState
	var why string
	switch {
	case err == nil && r.Type == typ && strings.EqualFold(r.Name, p.PanelDomain):
		return r, recOK, nil
	case err == nil:
		st, why = recOther, "es de otro nombre o de otro tipo"
	case errors.Is(err, service.ErrDNSNotFound):
		st, why = recGone, "ya no está"
	case isRejection(err):
		st, why = recUnreadable, "el token no lo puede leer"
	default:
		return service.Record{}, 0, err
	}
	u.log.Info("ingress: se olvida un registro guardado del panel", "tipo", typ, "id", id, "motivo", why)
	return service.Record{}, st, nil
}

// ensure aplica los pasos 4 y 5 de Execute y devuelve las claves que tienen
// que quedar, también si un paso falló (con ese error).
func (u *EnsurePanelDNS) ensure(ctx context.Context, p shared.ProxySettings, zones []service.Zone, own panelOwned) (service.PanelDNS, error) {
	host, tok := p.PanelDomain, p.CloudflareToken
	if host == "" {
		return service.PanelDNS{}, nil
	}
	z, found := service.MatchZone(host, zones)
	if !found {
		return service.PanelDNS{}, nil
	}
	if own.zoneID != z.ID {
		own = panelOwned{} // los de otra zona se olvidan, sin borrarlos
	}
	next := service.PanelDNS{ZoneID: z.ID, RecordID: recordID(own.a), RecordIDAAAA: recordID(own.aaaa)}
	if p.PublicIP == "" {
		return next, nil
	}

	var recs []service.Record
	if own.a == nil {
		var err error
		if recs, err = u.dns.Records(ctx, tok, z.ID, host); err != nil {
			return next, err
		}
		// Sin el A propio tampoco hay AAAA propio: cualquiera es ajeno.
		if typ := foreignType(recs, entity.Domain{}); typ != "" {
			u.log.Info("ingress: ya hay un registro del panel que muelle no creó; no se toca", "tipo", typ, "hostname", host)
			return next, nil
		}
		a, err := u.dns.CreateRecord(ctx, tok, z.ID, service.Record{Type: typeA, Name: host, Content: p.PublicIP})
		if err != nil {
			return next, err
		}
		own.a, next.RecordID = &a, a.ID
	} else if own.a.Content != p.PublicIP {
		ip := p.PublicIP
		if _, err := u.dns.UpdateRecord(ctx, tok, z.ID, own.a.ID, service.RecordChange{Content: &ip}); err != nil {
			return next, err
		}
	}

	switch ip6 := p.PublicIPv6; {
	case ip6 == "" && own.aaaa != nil:
		err := u.dns.DeleteRecord(ctx, tok, z.ID, own.aaaa.ID)
		if err != nil && !errors.Is(err, service.ErrDNSNotFound) {
			return next, err
		}
		next.RecordIDAAAA = ""
	case ip6 != "" && own.aaaa != nil:
		if own.aaaa.Content != ip6 {
			if _, err := u.dns.UpdateRecord(ctx, tok, z.ID, own.aaaa.ID, service.RecordChange{Content: &ip6}); err != nil {
				return next, err
			}
		}
	case ip6 != "":
		if recs == nil {
			var err error
			if recs, err = u.dns.Records(ctx, tok, z.ID, host); err != nil {
				return next, err
			}
		}
		for _, r := range recs {
			if r.Type == typeAAAA {
				u.log.Info("ingress: ya hay un AAAA del panel que muelle no creó; no se toca", "hostname", host)
				return next, nil
			}
		}
		aaaa, err := u.dns.CreateRecord(ctx, tok, z.ID,
			service.Record{Type: typeAAAA, Name: host, Content: ip6, Proxied: own.a.Proxied})
		if err != nil {
			return next, err
		}
		next.RecordIDAAAA = aaaa.ID
	}
	return next, nil
}

func recordID(r *service.Record) string {
	if r == nil {
		return ""
	}
	return r.ID
}
