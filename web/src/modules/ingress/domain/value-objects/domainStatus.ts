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
