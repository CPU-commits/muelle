import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError, formatDate, type StatusTone } from '@/shared/ui'
import { DomainError, UnavailableError, ValidationError } from '@/shared/domain'
import {
	type AppDomain,
	certificateExpiresSoon,
	CloudflareUnavailable,
	dnsMode,
	DockerUnavailable,
	DomainNotFound,
	type DomainState,
	type HostnameProblem,
	HostnameTaken,
	NothingToAdopt,
	originCaExpiresSoon,
	type OriginCaNotice,
	type TargetProblem,
} from '../../../domain'

// Cómo se nombra y se colorea cada estado de un dominio (App-Dominios.dc.html).

export function stateLabel(state: DomainState): string {
	switch (state) {
		case 'active':
			return m.ingress_state_active()
		case 'waiting_dns':
			return m.ingress_state_waiting_dns()
		case 'provisioning':
			return m.ingress_state_provisioning()
		case 'record_exists':
			return m.ingress_state_record_exists()
		case 'error':
			return m.ingress_state_error()
	}
}

export function stateTone(state: DomainState): StatusTone {
	switch (state) {
		case 'active':
			return 'ok'
		case 'waiting_dns':
		case 'provisioning':
			return 'warn'
		case 'record_exists':
		case 'error':
			return 'bad'
	}
}

/** "Registro A en Cloudflare", "Fuera de Cloudflare · certificado por HTTP-01"… */
export function dnsText(domain: AppDomain): string {
	switch (dnsMode(domain)) {
		case 'cloudflare':
			return domain.records.some((record) => record.type === 'AAAA')
				? m.ingress_dns_cloudflare_dual()
				: m.ingress_dns_cloudflare()
		case 'manual':
			return m.ingress_dns_manual()
		case 'unmanaged':
			return m.ingress_dns_unmanaged()
	}
}

/**
 * "Let's Encrypt · vence 8 ene 2027", o "Certificado pendiente". Con el de Cloudflare Origin
 * CA, "Cloudflare Origin CA · vence 19 ene 2027", con el vencimiento que guardó muelle: el de
 * la verificación (`cert_expires_at`) puede ir hasta una hora atrasado después del cambio, y
 * su emisor no se sabe cómo se llama (contrato §3.7).
 */
export function certificateText(domain: AppDomain): string | null {
	const originExpiresAt = domain.originCa?.expiresAt
	if (domain.certSource === 'cloudflare_origin_ca' && originExpiresAt) {
		const values = { date: formatDate(originExpiresAt) }
		return originCaExpiresSoon(domain)
			? m.ingress_cert_origin_ca_expiring(values)
			: m.ingress_cert_origin_ca(values)
	}
	const certificate = domain.certificate
	if (!certificate) {
		// Fuera de Cloudflare, "certificado por HTTP-01" ya lo dice la parte del DNS.
		return dnsMode(domain) === 'manual' ? null : m.ingress_cert_pending()
	}
	const values = { issuer: certificate.issuer, date: formatDate(certificate.expiresAt) }
	return certificateExpiresSoon(domain)
		? m.ingress_cert_expiring(values)
		: m.ingress_cert_valid(values)
}

/**
 * La pista del certificado de Cloudflare debajo del dominio (emitiendo, o por qué no lo usa),
 * o null: las fallas van en el pie, con Reintentar (`originCaFailureText`).
 */
export function originCaHint(notice: OriginCaNotice | null): string | null {
	switch (notice) {
		case 'issuing':
			return m.ingress_origin_ca_issuing()
		case 'dns_only':
			return m.ingress_origin_ca_dns_only()
		case 'dns_only_in_cloudflare':
			return m.ingress_origin_ca_dns_only_cloudflare()
		default:
			return null
	}
}

/** Por qué falló el certificado de Cloudflare: el mensaje de muelle, o el del panel si no hay. */
export function originCaFailureText(domain: AppDomain): string {
	const originCa = domain.originCa
	if (originCa?.error) return sentence(originCa.error)
	return originCa?.status === 'active'
		? m.ingress_origin_ca_renew_failed()
		: m.ingress_origin_ca_failed()
}

export function hostnameProblemMessage(problem: HostnameProblem): string {
	switch (problem) {
		case 'empty':
			return m.ingress_hostname_empty()
		case 'too_long':
			return m.ingress_hostname_too_long()
		case 'not_bare':
			return m.ingress_hostname_not_bare()
		case 'no_domain':
			return m.ingress_hostname_no_domain()
		case 'bad_tld':
			return m.ingress_hostname_bad_tld()
		case 'bad_label':
			return m.ingress_hostname_bad_label()
	}
}

export function targetProblemMessage(problem: TargetProblem): string {
	switch (problem) {
		case 'format':
			return m.ingress_target_format()
		case 'service':
			return m.ingress_target_service()
		case 'port':
			return m.ingress_target_port()
	}
}

/** Un mensaje del backend (en minúscula, estilo Go) para mostrar como oración. */
export function sentence(text: string): string {
	return text.charAt(0).toUpperCase() + text.slice(1)
}

/** El mensaje de un error del módulo para un toast o un banner. */
export function ingressErrorMessage(error: unknown): string {
	if (error instanceof HostnameTaken) return m.ingress_error_taken()
	if (error instanceof DomainNotFound) return m.ingress_error_not_found()
	if (error instanceof NothingToAdopt) return m.ingress_error_nothing_to_adopt()
	if (error instanceof CloudflareUnavailable) return m.ingress_error_cloudflare_unavailable()
	if (error instanceof DockerUnavailable) return m.ingress_error_docker()
	if (error instanceof UnavailableError) return describeError(error)
	if (error instanceof ValidationError) return sentence(error.message)
	if (error instanceof DomainError && error.code === 'rate_limited') return error.message
	return m.common_error_generic()
}
