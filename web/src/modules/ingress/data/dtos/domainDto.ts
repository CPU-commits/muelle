import { z } from 'zod'
import { DNS_RECORD_TYPES, DOMAIN_ERROR_KINDS, DOMAIN_STATUSES, TLS_MODES } from '../../domain'

// La Fase 3 todavía no existe en el backend: esta forma sale del spec (§5.1 tabla
// `domains`, §8, §9 y §10) y de las suposiciones anotadas en ../../README.md, que es lo
// que la Fase 3 tiene que respetar (o cambiar acá y en el README). snake_case como el
// resto de la API; los campos desconocidos se descartan.

const timestamp = z.iso.datetime({ offset: true })

export const dnsRecordDtoSchema = z.object({
	type: z.enum(DNS_RECORD_TYPES),
	name: z.string().min(1),
	content: z.string().min(1),
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
	// Un código que el panel no conoce se trata como "sin código" (Reintentar).
	last_error_code: z.enum(DOMAIN_ERROR_KINDS).nullish().catch(null),
	cert_issuer: z.string().nullish(),
	cert_expires_at: timestamp.nullish(),
	records: z.array(dnsRecordDtoSchema).default([]),
	created_at: timestamp.nullish(),
})

/** `GET …/apps/{a}/domains`. */
export const domainListDtoSchema = z.array(domainDtoSchema)

export type DnsRecordDto = z.infer<typeof dnsRecordDtoSchema>
export type DomainDto = z.infer<typeof domainDtoSchema>

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
