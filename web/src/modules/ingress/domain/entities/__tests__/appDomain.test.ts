import { describe, expect, it } from 'vitest'
import {
	makeDomain,
	manualPending,
	recordExists,
} from '../../repositories/__fakes__/domainFixtures'
import {
	canToggleProxied,
	certificateExpiresSoon,
	dnsMode,
	domainState,
	isReachable,
	manualRecords,
	publicAddress,
	replaceDomain,
	sortDomains,
	suggestedTargets,
} from '../appDomain'

describe('dnsMode', () => {
	it('con registro de muelle es cloudflare', () => {
		expect(dnsMode(makeDomain('api.tudominio.cl'))).toBe('cloudflare')
	})

	it('fuera de toda zona (tls http) es manual', () => {
		expect(dnsMode(manualPending('api.otro.com'))).toBe('manual')
	})

	it('en una zona pero sin registro de muelle es unmanaged', () => {
		expect(dnsMode(recordExists('webhooks.tudominio.cl'))).toBe('unmanaged')
	})
})

describe('domainState (estado y acción, spec §8 y §9)', () => {
	it('active es válido', () => {
		expect(domainState(makeDomain('api.tudominio.cl'))).toBe('active')
	})

	it('pending manual espera el DNS; pending en Cloudflare se está emitiendo', () => {
		expect(domainState(manualPending('api.otro.com'))).toBe('waiting_dns')
		expect(domainState(makeDomain('nuevo.tudominio.cl', { status: 'pending' }))).toBe(
			'provisioning',
		)
	})

	it('el registro existente se adopta', () => {
		expect(domainState(recordExists('webhooks.tudominio.cl'))).toBe('record_exists')
	})

	it('sin código, se reconoce por el mensaje del spec', () => {
		const domain = makeDomain('webhooks.tudominio.cl', {
			recordId: null,
			status: 'error',
			errorKind: null,
			lastError: 'el registro ya existe',
		})
		expect(domainState(domain)).toBe('record_exists')
	})

	it('cualquier otro error se reintenta', () => {
		const domain = makeDomain('api.tudominio.cl', {
			recordId: null,
			status: 'error',
			errorKind: 'cloudflare',
			lastError: 'Cloudflare respondió 403',
		})
		expect(domainState(domain)).toBe('error')
		expect(
			domainState(
				makeDomain('api.tudominio.cl', { status: 'error', errorKind: 'certificate' }),
			),
		).toBe('error')
	})
})

describe('reglas de la vista', () => {
	it('Proxied solo si muelle maneja el registro', () => {
		expect(canToggleProxied(makeDomain('api.tudominio.cl'))).toBe(true)
		expect(canToggleProxied(manualPending('api.otro.com'))).toBe(false)
		expect(canToggleProxied(recordExists('webhooks.tudominio.cl'))).toBe(false)
	})

	it('el certificado vence pronto a menos de 14 días', () => {
		const domain = makeDomain('api.tudominio.cl', {
			certificate: { issuer: 'R11', expiresAt: new Date('2026-10-20T00:00:00Z') },
		})
		expect(certificateExpiresSoon(domain, Date.parse('2026-10-10T00:00:00Z'))).toBe(true)
		expect(certificateExpiresSoon(domain, Date.parse('2026-09-01T00:00:00Z'))).toBe(false)
		expect(certificateExpiresSoon(manualPending('a.b.com'))).toBe(false)
	})

	it('solo un dominio activo se abre como link', () => {
		expect(isReachable(makeDomain('api.tudominio.cl'))).toBe(true)
		expect(isReachable(manualPending('api.otro.com'))).toBe(false)
	})

	it('los registros a mano solo se piden fuera de Cloudflare', () => {
		expect(manualRecords(manualPending('api.otro.com'))).toHaveLength(1)
		expect(manualRecords(makeDomain('api.tudominio.cl'))).toEqual([])
		expect(publicAddress(recordExists('webhooks.tudominio.cl'))).toBe('203.0.113.24')
	})

	it('suggestedTargets junta los del catálogo con los ya usados, sin repetir', () => {
		const domains = [
			makeDomain('a.tudominio.cl'),
			makeDomain('b.tudominio.cl', { target: { service: 'web', port: 80 } }),
		]
		expect(suggestedTargets(domains, ['api:3000', 'worker:9000'])).toEqual([
			'api:3000',
			'worker:9000',
			'web:80',
		])
	})

	it('ordena por hostname y reemplaza por hostname', () => {
		const domains = sortDomains([makeDomain('z.tudominio.cl'), makeDomain('a.tudominio.cl')])
		expect(domains.map((domain) => domain.hostname)).toEqual([
			'a.tudominio.cl',
			'z.tudominio.cl',
		])
		const updated = replaceDomain(domains, makeDomain('z.tudominio.cl', { proxied: false }))
		expect(updated[1]?.proxied).toBe(false)
	})
})
