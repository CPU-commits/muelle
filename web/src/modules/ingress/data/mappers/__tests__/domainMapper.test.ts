import { describe, expect, it } from 'vitest'
import { asHostname } from '../../../domain'
import { domainDtoSchema, proxyDtoSchema } from '../../dtos/domainDto'
import {
	createDomainPayload,
	domainFromDto,
	proxyFromDto,
	updateDomainPayload,
} from '../domainMapper'

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
	records: [
		{ type: 'A', name: 'api-staging.tudominio.cl', content: '203.0.113.24' },
		{ type: 'AAAA', name: 'api-staging.tudominio.cl', content: '2001:db8::24' },
	],
	created_at: '2026-10-01T12:00:00Z',
	updated_at: '2026-10-02T08:30:00Z',
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
			records: [
				{ type: 'A', name: 'api-staging.tudominio.cl', content: '203.0.113.24' },
				{ type: 'AAAA', name: 'api-staging.tudominio.cl', content: '2001:db8::24' },
			],
			createdAt: new Date('2026-10-01T12:00:00Z'),
			updatedAt: new Date('2026-10-02T08:30:00Z'),
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

describe('proxyFromDto', () => {
	it('traduce el estado de muelle-caddy', () => {
		const dto = proxyDtoSchema.parse({
			status: 'out_of_sync',
			error: 'Caddy rechazó la configuración',
			synced_at: '2026-10-10T14:20:41Z',
			caddy: {
				image: 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0',
				running: true,
				http_port: 80,
				https_port: 443,
			},
			ports_rollback: null,
		})
		expect(proxyFromDto(dto)).toEqual({
			status: 'out_of_sync',
			error: 'Caddy rechazó la configuración',
			syncedAt: new Date('2026-10-10T14:20:41Z'),
			caddy: {
				image: 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0',
				running: true,
				httpPort: 80,
				httpsPort: 443,
			},
			portsRollback: null,
		})
	})

	it('trae la última vuelta a los puertos anteriores (ports_rollback, contrato §3.3)', () => {
		const dto = proxyDtoSchema.parse({
			status: 'synced',
			error: null,
			synced_at: '2026-10-16T20:41:20Z',
			caddy: {
				image: 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0',
				running: true,
				http_port: 80,
				https_port: 443,
			},
			ports_rollback: {
				attempted: { http_port: 8080, https_port: 8443 },
				restored: { http_port: 80, https_port: 443 },
				error: 'los puertos 8080 o 8443 del host ya están en uso, y muelle-caddy no pudo arrancar con ellos',
				at: '2026-10-16T20:41:07.12Z',
			},
		})
		expect(proxyFromDto(dto).portsRollback).toEqual({
			attempted: { httpPort: 8080, httpsPort: 8443 },
			restored: { httpPort: 80, httpsPort: 443 },
			error: 'los puertos 8080 o 8443 del host ya están en uso, y muelle-caddy no pudo arrancar con ellos',
			at: new Date('2026-10-16T20:41:07.12Z'),
		})
	})

	it('ports_rollback viene siempre (contrato §3.3): null si muelle no volvió atrás, pero no falta', () => {
		const dto = {
			status: 'synced',
			error: null,
			synced_at: null,
			caddy: { image: null, running: false, http_port: 80, https_port: 443 },
			ports_rollback: null,
		}
		expect(proxyFromDto(proxyDtoSchema.parse(dto)).portsRollback).toBeNull()
		const { ports_rollback: _rollback, ...missing } = dto
		expect(_rollback).toBeNull()
		expect(proxyDtoSchema.safeParse(missing).success).toBe(false)
	})

	it('el schema rechaza un estado que no es del contrato', () => {
		expect(proxyDtoSchema.safeParse({ status: 'roto', caddy: { running: true } }).success).toBe(
			false,
		)
	})
})
