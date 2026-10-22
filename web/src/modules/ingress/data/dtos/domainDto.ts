import { z } from 'zod'
import {
	CERT_SOURCES,
	DNS_RECORD_TYPES,
	DOMAIN_ERROR_KINDS,
	DOMAIN_STATUSES,
	ORIGIN_CA_ERROR_CODES,
	ORIGIN_CA_STATUSES,
	PROXY_SYNC_STATUSES,
	TLS_MODES,
} from '../../domain'

// La forma del contrato HTTP de la Fase 3 (`docs/superpowers/specs/2026-10-10-contrato-
// http-fases-2-3.md`, §3, en este árbol), resumido en ../../README.md. snake_case como
// el resto de la API; los campos desconocidos se descartan.

const timestamp = z.iso.datetime({ offset: true })

export const dnsRecordDtoSchema = z.object({
	type: z.enum(DNS_RECORD_TYPES),
	name: z.string().min(1),
	content: z.string().min(1),
})

/**
 * El certificado de Cloudflare Origin CA de un hostname (contrato §3.7): las cinco claves van
 * siempre. Un `error_code` que el panel no conoce se trata como una falla cualquiera, con su
 * `error` (como `last_error_code`).
 */
export const originCaDtoSchema = z.object({
	status: z.enum(ORIGIN_CA_STATUSES),
	error_code: z.enum(ORIGIN_CA_ERROR_CODES).nullable().catch(null),
	error: z.string().nullable(),
	retry_at: timestamp.nullable(),
	expires_at: timestamp.nullable(),
})

export const domainDtoSchema = z.object({
	hostname: z.string().min(1),
	target_service: z.string().min(1),
	target_port: z.number().int().min(1).max(65535),
	proxied: z.boolean(),
	tls_mode: z.enum(TLS_MODES),
	cf_zone_id: z.string().nullish(),
	cf_record_id: z.string().nullish(),
	status: z.enum(DOMAIN_STATUSES),
	last_error: z.string().nullish(),
	// Con `status: error` siempre viene (la migración 0003 lo garantiza). Un código que el
	// panel no conoce se trata como un error cualquiera (Reintentar).
	last_error_code: z.enum(DOMAIN_ERROR_KINDS).nullish().catch(null),
	cert_issuer: z.string().nullish(),
	cert_expires_at: timestamp.nullish(),
	// Calculados, obligatorios (los certificados de Cloudflare, §3.1 y §3.7): `origin_ca` es
	// null con la opción apagada, sin token o con el proxy deshabilitado.
	cert_source: z.enum(CERT_SOURCES),
	origin_ca: originCaDtoSchema.nullable(),
	// Calculados con los ajustes actuales: `[]` sin IP pública.
	records: z.array(dnsRecordDtoSchema).default([]),
	created_at: timestamp.nullish(),
	updated_at: timestamp.nullish(),
})

/** `GET …/apps/{a}/domains`. */
export const domainListDtoSchema = z.array(domainDtoSchema)

const proxyPortsDtoSchema = z.object({
	http_port: z.number().int(),
	https_port: z.number().int(),
})

/** `GET /proxy` (contrato §3.3): el estado de muelle-caddy. */
export const proxyDtoSchema = z.object({
	status: z.enum(PROXY_SYNC_STATUSES),
	error: z.string().nullish(),
	synced_at: timestamp.nullish(),
	caddy: z.object({
		image: z.string().nullish(),
		running: z.boolean(),
		http_port: z.number().int().nullish(),
		https_port: z.number().int().nullish(),
	}),
	// La última vez que muelle volvió a los puertos anteriores (§3.5), o null si no volvió.
	ports_rollback: z
		.object({
			attempted: proxyPortsDtoSchema,
			restored: proxyPortsDtoSchema,
			error: z.string(),
			at: timestamp,
		})
		.nullable(),
})

export type OriginCaDto = z.infer<typeof originCaDtoSchema>
export type DnsRecordDto = z.infer<typeof dnsRecordDtoSchema>
export type DomainDto = z.infer<typeof domainDtoSchema>
export type ProxyDto = z.infer<typeof proxyDtoSchema>

/** Cuerpo de `POST …/apps/{a}/domains`. */
export interface CreateDomainPayload {
	hostname: string
	target_service: string
	target_port: number
	proxied: boolean
}

/** Cuerpo de `PATCH …/apps/{a}/domains/{hostname}`: solo lo que cambia. */
export interface UpdateDomainPayload {
	target_service?: string
	target_port?: number
	proxied?: boolean
}
