package usecases

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
	"github.com/CPU-commits/muelle/src/internal/ingress/domain/service"
	"github.com/CPU-commits/muelle/src/package/clock"
	shared "github.com/CPU-commits/muelle/src/shared/domain/entity"
)

// dnsOps son las reglas del DNS de un dominio en Cloudflare (contrato §3.1 y
// §3.2, §9 del spec). Cambian d en memoria y nunca guardan: el que llama
// guarda d (con el dbCtx de Ops.Do). Un error de Cloudflare no se devuelve:
// queda en el estado del dominio (error + cloudflare), salvo en remove.
type dnsOps struct {
	dns   service.DNS
	clock clock.Clock
}

// Los tipos de registro que cuentan como "un registro de ese nombre" (D7).
const (
	typeA     = "A"
	typeAAAA  = "AAAA"
	typeCNAME = "CNAME"
)

// ensure aplica a d las reglas de crear (contrato §3.1) con los ajustes p.
// Con registros propios, los actualiza: el A a public_ip con el proxied del
// dominio, y el AAAA a public_ipv6 (lo crea si falta, lo borra si ya no hay
// IPv6). Si el A ya no está (D29), sigue como sin registros. Sin registros
// propios, busca la zona; al encontrarla fija CFZoneID y TLSCloudflare (D5)
// antes de revisar los registros ajenos y crear los suyos. Sin token, o sin
// zona, queda http, pending y sin proxied. Si todo sale bien queda pending.
func (o dnsOps) ensure(ctx context.Context, d *entity.Domain, p shared.ProxySettings) {
	now := o.clock.Now()
	if d.Managed() {
		if p.CloudflareToken == "" {
			d.Fail(entity.CodeCloudflare, msgNoToken, now)
			return
		}
		if !o.updateManaged(ctx, d, p, now) {
			return
		}
		// El A ya no estaba: sus ids se olvidaron (D29), y se crea de nuevo.
	}
	if p.CloudflareToken == "" {
		withoutZone(d, now)
		return
	}
	zones, err := o.dns.Zones(ctx, p.CloudflareToken)
	if err != nil {
		// Sin las zonas: http provisorio, y el proxied pedido queda para el
		// reintento (D26).
		d.TLSMode, d.CFZoneID = entity.TLSHTTP, ""
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return
	}
	z, found := service.MatchZone(d.Hostname, zones)
	if !found {
		withoutZone(d, now)
		return
	}
	o.createIn(ctx, d, p, z.ID, now)
}

// findZone es para retry de un dominio sin zona y sin registros propios
// (D23): busca la zona y, si la encuentra, crea los registros como al crear
// (createIn). found false: no tocó d (sin token, sin zona, o Cloudflare que
// no respondió: el dominio sigue funcionando por http).
func (o dnsOps) findZone(ctx context.Context, d *entity.Domain, p shared.ProxySettings) (found bool) {
	if p.CloudflareToken == "" {
		return false
	}
	zones, err := o.dns.Zones(ctx, p.CloudflareToken)
	if err != nil {
		return false
	}
	z, ok := service.MatchZone(d.Hostname, zones)
	if !ok {
		return false
	}
	o.createIn(ctx, d, p, z.ID, o.clock.Now())
	return true
}

// withoutZone: sin token o fuera de toda zona, el registro se crea a mano y
// el certificado sale por HTTP-01 (contrato §3.1).
func withoutZone(d *entity.Domain, now time.Time) {
	d.TLSMode, d.CFZoneID, d.Proxied = entity.TLSHTTP, "", false
	d.MarkPending(now)
}

// createIn fija la zona (D5) y, si no hay un registro ajeno de ese nombre,
// crea los registros de d.
func (o dnsOps) createIn(ctx context.Context, d *entity.Domain, p shared.ProxySettings, zoneID string, now time.Time) {
	d.CFZoneID, d.TLSMode = zoneID, entity.TLSCloudflare
	recs, err := o.dns.Records(ctx, p.CloudflareToken, zoneID, d.Hostname)
	if err != nil {
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return
	}
	if typ := foreignType(recs, *d); typ != "" {
		d.Fail(entity.CodeRecordExists, msgRecordExists(typ, d.Hostname), now)
		return
	}
	if p.PublicIP == "" {
		d.Fail(entity.CodeCloudflare, msgNoPublicIP, now)
		return
	}
	o.createRecords(ctx, d, p, d.Proxied, now)
}

// createRecords crea el A (y el AAAA si hay public_ipv6) con proxied, y deja
// d pending. Si el AAAA falla, el A queda manejado y d en error.
func (o dnsOps) createRecords(ctx context.Context, d *entity.Domain, p shared.ProxySettings, proxied bool, now time.Time) {
	a, err := o.dns.CreateRecord(ctx, p.CloudflareToken, d.CFZoneID,
		service.Record{Type: typeA, Name: d.Hostname, Content: p.PublicIP, Proxied: proxied})
	if err != nil {
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return
	}
	d.CFRecordID, d.Proxied = a.ID, proxied
	if p.PublicIPv6 != "" {
		aaaa, err := o.dns.CreateRecord(ctx, p.CloudflareToken, d.CFZoneID,
			service.Record{Type: typeAAAA, Name: d.Hostname, Content: p.PublicIPv6, Proxied: proxied})
		if err != nil {
			d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
			return
		}
		d.CFRecordIDAAAA = aaaa.ID
	}
	d.MarkPending(now)
}

// foreignType es el tipo de un registro A, CNAME o AAAA de ese nombre que
// muelle no maneja (D7), en ese orden de preferencia; "" si no hay.
func foreignType(recs []service.Record, d entity.Domain) string {
	for _, typ := range []string{typeA, typeCNAME, typeAAAA} {
		if slices.ContainsFunc(recs, func(r service.Record) bool {
			return r.Type == typ && r.ID != d.CFRecordID && r.ID != d.CFRecordIDAAAA
		}) {
			return typ
		}
	}
	return ""
}

// updateManaged lleva los registros propios a p: el A a public_ip y al
// proxied del dominio, y el AAAA según public_ipv6 (syncAAAA). Si todo sale
// bien, d queda pending. aGone: el A ya no estaba, y sus ids se olvidaron
// (D29); el que llama sigue como sin registros.
func (o dnsOps) updateManaged(ctx context.Context, d *entity.Domain, p shared.ProxySettings, now time.Time) (aGone bool) {
	if p.PublicIP == "" {
		d.Fail(entity.CodeCloudflare, msgNoPublicIP, now)
		return false
	}
	ip, proxied := p.PublicIP, d.Proxied
	_, err := o.dns.UpdateRecord(ctx, p.CloudflareToken, d.CFZoneID, d.CFRecordID,
		service.RecordChange{Content: &ip, Proxied: &proxied})
	if errors.Is(err, service.ErrDNSNotFound) {
		o.dropManaged(ctx, d, p)
		return true
	}
	if err != nil {
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return false
	}
	if o.syncAAAA(ctx, d, p, now) {
		d.MarkPending(now)
	}
	return false
}

// syncAAAA lleva el AAAA propio de un dominio con A propio a public_ipv6: lo
// actualiza (contenido y proxied), lo crea si falta (y si no hay un AAAA
// ajeno, contrato §3.5), o lo borra si no hay IPv6. false: falló, y d quedó
// en error + cloudflare.
func (o dnsOps) syncAAAA(ctx context.Context, d *entity.Domain, p shared.ProxySettings, now time.Time) bool {
	tok, zone := p.CloudflareToken, d.CFZoneID
	if p.PublicIPv6 == "" {
		if d.CFRecordIDAAAA == "" {
			return true
		}
		if err := o.dns.DeleteRecord(ctx, tok, zone, d.CFRecordIDAAAA); err != nil && !errors.Is(err, service.ErrDNSNotFound) {
			d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
			return false
		}
		d.CFRecordIDAAAA = ""
		return true
	}
	ip6, proxied := p.PublicIPv6, d.Proxied
	if d.CFRecordIDAAAA != "" {
		_, err := o.dns.UpdateRecord(ctx, tok, zone, d.CFRecordIDAAAA, service.RecordChange{Content: &ip6, Proxied: &proxied})
		switch {
		case err == nil:
			return true
		case !errors.Is(err, service.ErrDNSNotFound):
			d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
			return false
		}
		d.CFRecordIDAAAA = "" // ya no está: se crea de nuevo
	}
	recs, err := o.dns.Records(ctx, tok, zone, d.Hostname)
	if err != nil {
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return false
	}
	if slices.ContainsFunc(recs, func(r service.Record) bool { return r.Type == typeAAAA }) {
		d.Fail(entity.CodeCloudflare, msgForeignAAAA(d.Hostname), now)
		return false
	}
	aaaa, err := o.dns.CreateRecord(ctx, tok, zone, service.Record{Type: typeAAAA, Name: d.Hostname, Content: ip6, Proxied: proxied})
	if err != nil {
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return false
	}
	d.CFRecordIDAAAA = aaaa.ID
	return true
}

// dropManaged es D29: el A propio ya no está, así que muelle tampoco maneja
// el AAAA. Borra el AAAA propio (un 404 no es error) y olvida los dos ids,
// también si el borrado falla. aaaaKept: el AAAA no se pudo borrar y quedó
// en Cloudflare (ahora es ajeno).
func (o dnsOps) dropManaged(ctx context.Context, d *entity.Domain, p shared.ProxySettings) (aaaaKept bool) {
	if d.CFRecordIDAAAA != "" {
		err := o.dns.DeleteRecord(ctx, p.CloudflareToken, d.CFZoneID, d.CFRecordIDAAAA)
		aaaaKept = err != nil && !errors.Is(err, service.ErrDNSNotFound)
	}
	d.CFRecordID, d.CFRecordIDAAAA = "", ""
	return aaaaKept
}

// setProxied cambia el proxied de los registros propios (PATCH, contrato
// §3.2). El valor pedido se guarda siempre; si Cloudflare falla, o no hay
// token, d queda en error + cloudflare. Si funciona y d estaba en error +
// cloudflare, vuelve a pending. Si el A ya no está, D29 y error + cloudflare.
func (o dnsOps) setProxied(ctx context.Context, d *entity.Domain, p shared.ProxySettings, proxied bool) {
	now := o.clock.Now()
	d.Proxied = proxied
	if p.CloudflareToken == "" {
		d.Fail(entity.CodeCloudflare, msgNoToken, now)
		return
	}
	change := service.RecordChange{Proxied: &proxied}
	_, err := o.dns.UpdateRecord(ctx, p.CloudflareToken, d.CFZoneID, d.CFRecordID, change)
	if errors.Is(err, service.ErrDNSNotFound) {
		kept := o.dropManaged(ctx, d, p)
		d.Fail(entity.CodeCloudflare, msgRecordGone(typeA, d.Hostname, kept), now)
		return
	}
	if err != nil {
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return
	}
	if d.CFRecordIDAAAA != "" {
		_, err := o.dns.UpdateRecord(ctx, p.CloudflareToken, d.CFZoneID, d.CFRecordIDAAAA, change)
		if errors.Is(err, service.ErrDNSNotFound) {
			d.CFRecordIDAAAA = ""
			d.Fail(entity.CodeCloudflare, msgRecordGone(typeAAAA, d.Hostname, false), now)
			return
		}
		if err != nil {
			d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
			return
		}
	}
	if d.Status == entity.StatusError && d.ErrorCode == entity.CodeCloudflare {
		d.MarkPending(now)
		return
	}
	d.Touch(now)
}

// remove borra los registros propios de d en Cloudflare, el AAAA primero;
// uno que ya no está (404) no es un error. kept: había registros propios y
// no hay token, así que quedan en Cloudflare (contrato §3.2). Cualquier otro
// fallo, también un rechazo (un token sin permiso), es un error que envuelve
// entity.ErrCloudflareUnavailable (502), y no se borra nada en muelle.
func (o dnsOps) remove(ctx context.Context, d entity.Domain, p shared.ProxySettings) (kept bool, err error) {
	if !d.Managed() {
		return false, nil
	}
	if p.CloudflareToken == "" {
		return true, nil
	}
	for _, id := range []string{d.CFRecordIDAAAA, d.CFRecordID} {
		if id == "" {
			continue
		}
		err := o.dns.DeleteRecord(ctx, p.CloudflareToken, d.CFZoneID, id)
		if err == nil || errors.Is(err, service.ErrDNSNotFound) {
			continue
		}
		if !errors.Is(err, shared.ErrCloudflareUnavailable) {
			err = fmt.Errorf("%w: %w", shared.ErrCloudflareUnavailable, err)
		}
		return false, fmt.Errorf("ingress: borrando los registros de %s: %w", d.Hostname, err)
	}
	return false, nil
}

// adopt pasa a manejar el registro ajeno de d (contrato §3.2, D17 y D32). Usa
// d.CFZoneID, que record_exists siempre trae (D7): no vuelve a buscar la
// zona, y deja TLSCloudflare. Necesita el token y public_ip:
//   - un A o un CNAME: lo lleva a un A con public_ip y conserva su proxied,
//     que se copia al dominio. Con public_ipv6, adopta el AAAA que haya (con
//     su proxied) o lo crea (con el del A);
//   - solo AAAA (D32): crea su A con el proxied de ese AAAA y, con
//     public_ipv6, después adopta el AAAA (si el A falla, el AAAA no se toca);
//   - ninguno: crea los suyos, como al crear;
//   - más de un A o CNAME, o con public_ipv6 más de un AAAA: no toca nada.
//
// Sin public_ipv6, un AAAA ajeno nunca se toca. Si todo sale bien, d queda
// pending; si no, en error + cloudflare.
func (o dnsOps) adopt(ctx context.Context, d *entity.Domain, p shared.ProxySettings) {
	now := o.clock.Now()
	switch {
	case p.CloudflareToken == "":
		d.Fail(entity.CodeCloudflare, msgNoToken, now)
		return
	case p.PublicIP == "":
		d.Fail(entity.CodeCloudflare, msgNoPublicIP, now)
		return
	}
	tok, zone := p.CloudflareToken, d.CFZoneID
	d.TLSMode = entity.TLSCloudflare
	recs, err := o.dns.Records(ctx, tok, zone, d.Hostname)
	if err != nil {
		d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
		return
	}
	var addr, aaaas []service.Record
	for _, r := range recs {
		switch r.Type {
		case typeA, typeCNAME:
			addr = append(addr, r)
		case typeAAAA:
			aaaas = append(aaaas, r)
		}
	}
	switch {
	case len(addr) > 1:
		d.Fail(entity.CodeCloudflare, msgMultiple("A o CNAME", d.Hostname), now)
		return
	case p.PublicIPv6 != "" && len(aaaas) > 1:
		d.Fail(entity.CodeCloudflare, msgMultiple(typeAAAA, d.Hostname), now)
		return
	case len(addr) == 0 && len(aaaas) == 0:
		o.createRecords(ctx, d, p, d.Proxied, now)
		return
	}

	ip := p.PublicIP
	if len(addr) == 1 {
		r := addr[0]
		change := service.RecordChange{Content: &ip}
		if r.Type != typeA {
			a := typeA
			change.Type = &a
		}
		if _, err := o.dns.UpdateRecord(ctx, tok, zone, r.ID, change); err != nil {
			d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
			return
		}
		d.CFRecordID, d.Proxied = r.ID, r.Proxied
	} else {
		proxied := aaaaProxied(aaaas, d.Proxied)
		a, err := o.dns.CreateRecord(ctx, tok, zone, service.Record{Type: typeA, Name: d.Hostname, Content: ip, Proxied: proxied})
		if err != nil {
			d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
			return
		}
		d.CFRecordID, d.Proxied = a.ID, proxied
	}

	if p.PublicIPv6 != "" {
		ip6 := p.PublicIPv6
		if len(aaaas) == 1 {
			if _, err := o.dns.UpdateRecord(ctx, tok, zone, aaaas[0].ID, service.RecordChange{Content: &ip6}); err != nil {
				d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
				return
			}
			d.CFRecordIDAAAA = aaaas[0].ID
		} else {
			aaaa, err := o.dns.CreateRecord(ctx, tok, zone, service.Record{Type: typeAAAA, Name: d.Hostname, Content: ip6, Proxied: d.Proxied})
			if err != nil {
				d.Fail(entity.CodeCloudflare, cloudflareMsg(err), now)
				return
			}
			d.CFRecordIDAAAA = aaaa.ID
		}
	}
	d.MarkPending(now)
}

// aaaaProxied es el proxied de los AAAA ajenos si todos tienen el mismo (con
// uno solo, el suyo: D32); si no, def.
func aaaaProxied(aaaas []service.Record, def bool) bool {
	for _, r := range aaaas[1:] {
		if r.Proxied != aaaas[0].Proxied {
			return def
		}
	}
	return aaaas[0].Proxied
}
