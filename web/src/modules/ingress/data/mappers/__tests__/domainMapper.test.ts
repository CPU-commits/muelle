import { describe, expect, it } from 'vitest'
import { asHostname } from '../../../domain'
import { domainDtoSchema } from '../../dtos/domainDto'
import { createDomainPayload, domainFromDto, updateDomainPayload } from '../domainMapper'

const DTO = {
	hostname: 'api-staging.tudominio.cl',
	target_service: 'api',
	target_port: 3000,
	proxied: true,
	tls_mode: 'cloudflare',
	cf_zone_id: '023e105f',
	cf_record_id: '372e6795',
	status: 'active',
	last_error: null,
	last_error_code: null,
	cert_issuer: "Let's Encrypt",
	cert_expires_at: '2027-01-08T00:00:00Z',
	records: [{ type: 'A', name: 'api-staging.tudominio.cl', content: '203.0.113.24' }],
	created_at: '2026-10-01T12:00:00Z',
}

describe('domainFromDto', () => {
	it('traduce snake_case y arma el certificado', () => {
		expect(domainFromDto(domainDtoSchema.parse(DTO))).toEqual({
			hostname: 'api-staging.tudominio.cl',
			target: { service: 'api', port: 3000 },
			proxied: true,
			tlsMode: 'cloudflare',
			zoneId: '023e105f',
			recordId: '372e6795',
			status: 'active',
			lastError: null,
			errorKind: null,
			certificate: { issuer: "Let's Encrypt", expiresAt: new Date('2027-01-08T00:00:00Z') },
			records: [{ type: 'A', name: 'api-staging.tudominio.cl', content: '203.0.113.24' }],
			createdAt: new Date('2026-10-01T12:00:00Z'),
		})
	})

	it('sin emisor o sin vencimiento no hay certificado; los opcionales faltantes son null', () => {
		const dto = domainDtoSchema.parse({
			...DTO,
			cert_issuer: null,
			cf_zone_id: undefined,
			cf_record_id: undefined,
			records: undefined,
			created_at: undefined,
		})
		expect(domainFromDto(dto)).toMatchObject({
			certificate: null,
			zoneId: null,
			recordId: null,
			records: [],
			createdAt: null,
		})
	})

	it('un last_error_code desconocido se lee como sin código', () => {
		const dto = domainDtoSchema.parse({ ...DTO, status: 'error', last_error_code: 'otro' })
		expect(domainFromDto(dto).errorKind).toBeNull()
	})

	it('el schema rechaza un status que no es del spec', () => {
		expect(domainDtoSchema.safeParse({ ...DTO, status: 'ok' }).success).toBe(false)
	})
})

describe('payloads', () => {
	it('crear manda hostname, servicio, puerto y proxied', () => {
		expect(
			createDomainPayload({
				hostname: asHostname('api.tudominio.cl'),
				target: { service: 'api', port: 3000 },
				proxied: false,
			}),
		).toEqual({
			hostname: 'api.tudominio.cl',
			target_service: 'api',
			target_port: 3000,
			proxied: false,
		})
	})

	it('editar manda solo lo que cambia', () => {
		expect(updateDomainPayload({ proxied: true })).toEqual({ proxied: true })
		expect(updateDomainPayload({ target: { service: 'web', port: 80 } })).toEqual({
			target_service: 'web',
			target_port: 80,
		})
	})
})
