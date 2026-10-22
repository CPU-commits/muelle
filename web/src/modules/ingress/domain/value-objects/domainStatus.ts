// Los valores de la tabla `domains` (spec §5.1, §8 y §9).

/** `pending` mientras se crea el registro o se emite el certificado; `error` con `last_error`. */
export const DOMAIN_STATUSES = ['pending', 'active', 'error'] as const
export type DomainStatus = (typeof DOMAIN_STATUSES)[number]

/**
 * `cloudflare`: certificado por el desafío DNS de Cloudflare (el hostname está en una
 * zona del token). `http`: certificado por HTTP-01 (fuera de Cloudflare: el registro se
 * crea a mano).
 */
export const TLS_MODES = ['cloudflare', 'http'] as const
export type TlsMode = (typeof TLS_MODES)[number]

/**
 * El motivo de un `error`, para elegir la acción (`last_error_code`, migración 0003: con
 * `status: error` nunca falta):
 * - `record_exists`: el registro ya existe y no lo creó muelle → Adoptar (§9);
 * - `cloudflare`: la API de Cloudflare respondió con error → Reintentar (§9);
 * - `certificate`: certificado inválido o por vencer en menos de 14 días (§8) → Reintentar.
 */
export const DOMAIN_ERROR_KINDS = ['record_exists', 'cloudflare', 'certificate'] as const
export type DomainErrorKind = (typeof DOMAIN_ERROR_KINDS)[number]

export const DNS_RECORD_TYPES = ['A', 'AAAA'] as const
export type DnsRecordType = (typeof DNS_RECORD_TYPES)[number]

/**
 * Qué certificado le pide muelle a Caddy para el dominio (contrato §3.1, calculado): el de
 * ACME (`letsencrypt`: DNS-01 con `tls_mode: cloudflare`, HTTP-01 con `http`) o uno de
 * Cloudflare Origin CA que emitió muelle (§3.7). Es `cloudflare_origin_ca` si y solo si
 * `origin_ca.status` es `active`.
 */
export const CERT_SOURCES = ['letsencrypt', 'cloudflare_origin_ca'] as const
export type CertSource = (typeof CERT_SOURCES)[number]

/**
 * El estado del certificado de Cloudflare Origin CA (contrato §3.7, en el orden en que se
 * decide):
 * - `not_eligible`: no califica (fuera de una zona, sin el registro de muelle, o en
 *   Cloudflare no hay registros de ese nombre): Let's Encrypt;
 * - `dns_only`: sin el proxy, en muelle o en Cloudflare (la nube gris): Let's Encrypt;
 * - `active`: Caddy sirve el de Origin CA (con `error` si falló una renovación);
 * - `failed`: califica, pero la última emisión falló: Let's Encrypt mientras tanto;
 * - `issuing`: muelle lo está emitiendo: Let's Encrypt mientras tanto.
 */
export const ORIGIN_CA_STATUSES = [
	'active',
	'issuing',
	'failed',
	'dns_only',
	'not_eligible',
] as const
export type OriginCaStatus = (typeof ORIGIN_CA_STATUSES)[number]

/**
 * Por qué falló la última emisión o renovación: al token le falta el permiso Zone → SSL and
 * Certificates → Edit (`permission`), Cloudflare no respondió (`cloudflare`) o lo rechazó
 * por otro motivo (`rejected`).
 */
export const ORIGIN_CA_ERROR_CODES = ['permission', 'cloudflare', 'rejected'] as const
export type OriginCaErrorCode = (typeof ORIGIN_CA_ERROR_CODES)[number]
