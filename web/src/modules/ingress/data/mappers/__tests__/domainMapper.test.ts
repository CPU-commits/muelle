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
	cert_source: 'letsencrypt',
	origin_ca: null,
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
			certSource: 'letsencrypt',
			originCa: null,
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

// Los certificados de Cloudflare (Origin CA, contrato §3.1 y §3.7).
describe('cert_source y origin_ca', () => {
	it('uno que se sirve con Origin CA trae su vencimiento', () => {
		const dto = domainDtoSchema.parse({
			...DTO,
			cert_issuer: 'CloudFlare, Inc.',
			cert_source: 'cloudflare_origin_ca',
			origin_ca: {
				status: 'active',
				error_code: null,
				error: null,
				retry_at: null,
				expires_at: '2027-01-19T12:00:00Z',
			},
		})
		expect(domainFromDto(dto)).toMatchObject({
			certSource: 'cloudflare_origin_ca',
			originCa: {
				status: 'active',
				errorCode: null,
				error: null,
				retryAt: null,
				expiresAt: new Date('2027-01-19T12:00:00Z'),
			},
		})
	})

	it('una emisión que falló trae el código, el mensaje y cuándo se reintenta', () => {
		const error =
			"muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso Zone → SSL and Certificates → Edit en la zona de api.tudominio.cl (Cloudflare: Authentication error); agrégalo al token y pulsa Reintentar; mientras tanto usa Let's Encrypt"
		const dto = domainDtoSchema.parse({
			...DTO,
			origin_ca: {
				status: 'failed',
				error_code: 'permission',
				error,
				retry_at: '2026-10-21T21:04:10.5Z',
				expires_at: null,
			},
		})
		expect(domainFromDto(dto).originCa).toEqual({
			status: 'failed',
			errorCode: 'permission',
			error,
			retryAt: new Date('2026-10-21T21:04:10.5Z'),
			expiresAt: null,
		})
	})

	it('un error_code que el panel no conoce es una falla cualquiera, con su mensaje', () => {
		const dto = domainDtoSchema.parse({
			...DTO,
			origin_ca: {
				status: 'failed',
				error_code: 'otro',
				error: 'muelle no pudo emitir el certificado de Cloudflare: algo nuevo',
				retry_at: '2026-10-21T21:04:10Z',
				expires_at: null,
			},
		})
		expect(domainFromDto(dto).originCa).toMatchObject({
			errorCode: null,
			error: 'muelle no pudo emitir el certificado de Cloudflare: algo nuevo',
		})
	})

	it('son obligatorios: un dominio sin cert_source o sin origin_ca no es del contrato', () => {
		const { cert_source: _source, ...withoutSource } = DTO
		const { origin_ca: _originCa, ...withoutOriginCa } = DTO
		expect([_source, _originCa]).toEqual(['letsencrypt', null])
		expect(domainDtoSchema.safeParse(withoutSource).success).toBe(false)
		expect(domainDtoSchema.safeParse(withoutOriginCa).success).toBe(false)
		expect(domainDtoSchema.safeParse({ ...DTO, cert_source: 'otro' }).success).toBe(false)
		const originCa = { error_code: null, error: null, retry_at: null, expires_at: null }
		expect(
			domainDtoSchema.safeParse({ ...DTO, origin_ca: { ...originCa, status: 'otro' } })
				.success,
		).toBe(false)
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
