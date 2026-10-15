package service

import (
	"strconv"
	"time"

	"github.com/CPU-commits/muelle/src/internal/ingress/domain/entity"
)

const (
	// RenewMargin: un certificado que vence antes no cuenta como bueno
	// (contrato §3.1): Caddy lo renueva 30 días antes, así que a los 14 algo
	// anda mal.
	RenewMargin = 14 * 24 * time.Hour
	// PendingGrace es lo que tiene un pending para conseguir un certificado
	// válido, desde su último cambio (UpdatedAt).
	PendingGrace = 10 * time.Minute
)

// CertObservation es lo que vio la verificación en el certificado que sirve
// Caddy para un hostname.
type CertObservation struct {
	Valid bool
	// Issuer es la primera Organization del emisor, o su CommonName si no
	// tiene (D9 del plan de la Fase 3). Solo con Valid.
	Issuer string
	// NotAfter es el vencimiento de la hoja. Solo con Valid.
	NotAfter time.Time
	// Problem dice por qué no es válido, en español. Solo sin Valid.
	Problem string
}

// ApplyCert aplica a d lo que vio la verificación, con las reglas del
// contrato §3.1, y dice si cambió algo (también si solo guardó el emisor y
// el vencimiento, que no mueven UpdatedAt):
//   - un error con cloudflare o record_exists no se mira: primero el DNS;
//   - uno válido guarda su emisor y su vencimiento, y es "bueno" si vence en
//     más de 14 días (RenewMargin);
//   - pending: con uno bueno pasa a active; si no, recién PendingGrace
//     después de UpdatedAt pasa a error + certificate. retry no reinicia el
//     plazo;
//   - active: con uno que no es bueno pasa a error + certificate;
//   - error + certificate: con uno bueno vuelve a active; si no, con retry
//     vuelve a pending con otros 10 min, y sin retry queda igual.
//
// No lo llama nadie con un proxy que no está synced ni si no se llegó a
// Caddy: ese caso es "no se sabe" y no cambia nada.
func ApplyCert(d *entity.Domain, o CertObservation, now time.Time, retry bool) (changed bool) {
	if d.Status == entity.StatusError && d.ErrorCode != entity.CodeCertificate {
		return false
	}
	if o.Valid {
		changed = d.SetCertificate(o.Issuer, o.NotAfter)
	}
	good := o.Valid && o.NotAfter.Sub(now) > RenewMargin
	problem := o.Problem
	switch {
	case o.Valid:
		problem = "el certificado vence el " + shortDate(o.NotAfter) + " y Caddy no lo renovó"
	case problem == "":
		problem = "el certificado que sirve Caddy no es válido"
	}

	switch d.Status {
	case entity.StatusPending:
		if good {
			d.MarkActive(now)
			return true
		}
		if now.Sub(d.UpdatedAt) < PendingGrace {
			return changed
		}
		d.Fail(entity.CodeCertificate, deadlineMessage(problem, d.TLSMode), now)
		return true
	case entity.StatusActive:
		if good {
			return changed
		}
		d.Fail(entity.CodeCertificate, problem, now)
		return true
	case entity.StatusError:
		switch {
		case good:
			d.MarkActive(now)
			return true
		case retry:
			d.MarkPending(now)
			return true
		}
	}
	return changed
}

// deadlineMessage es el mensaje de un pending que no consiguió su
// certificado a tiempo, con qué revisar según el modo de TLS.
func deadlineMessage(problem string, mode entity.TLSMode) string {
	msg := "Caddy no consiguió un certificado válido en 10 minutos: " + problem
	if mode == entity.TLSCloudflare {
		return msg + "; revisa que el token de Cloudflare pueda editar el DNS de la zona"
	}
	return msg + "; revisa que el registro DNS apunte a la IP pública y que los puertos 80 y 443 lleguen a muelle-caddy"
}

var monthsES = [...]string{"ene", "feb", "mar", "abr", "may", "jun", "jul", "ago", "sep", "oct", "nov", "dic"}

// shortDate es la fecha en UTC como "2 ene 2027", sin depender del locale.
func shortDate(t time.Time) string {
	t = t.UTC()
	return strconv.Itoa(t.Day()) + " " + monthsES[t.Month()-1] + " " + strconv.Itoa(t.Year())
}
