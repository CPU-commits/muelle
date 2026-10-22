import { describe, expect, it } from 'vitest'
import { formatDate } from '@/shared/ui'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { OriginCaState } from '../../../../domain'
import { makeDomain } from '../../../../domain/repositories/__fakes__/domainFixtures'
import { certificateText, originCaFailureText, originCaHint } from '../domainLabels'

const DAY_MS = 24 * 60 * 60 * 1000

function originCa(overrides: Partial<OriginCaState> = {}): OriginCaState {
	return {
		status: 'active',
		errorCode: null,
		error: null,
		retryAt: null,
		expiresAt: new Date(Date.now() + 80 * DAY_MS),
		...overrides,
	}
}

// Los certificados de Cloudflare (Origin CA, contrato §3.7) en la tarjeta de un dominio.
describe('certificateText', () => {
	it("con Let's Encrypt, el emisor que vio la verificación", () => {
		const domain = makeDomain('api.tudominio.cl')
		expect(certificateText(domain)).toBe(
			m.ingress_cert_valid({
				issuer: "Let's Encrypt",
				date: formatDate(new Date('2027-01-08T00:00:00Z')),
			}),
		)
	})

	it('con Origin CA, el origen y el vencimiento que guardó muelle (no el de la verificación)', () => {
		const state = originCa()
		const domain = makeDomain('api.tudominio.cl', {
			certSource: 'cloudflare_origin_ca',
			originCa: state,
		})

		const text = certificateText(domain)

		expect(text).toBe(m.ingress_cert_origin_ca({ date: formatDate(state.expiresAt as Date) }))
		expect(text).toMatch(/^Cloudflare Origin CA · vence /)
	})

	it('dentro de los 30 días de la renovación dice "pronto"', () => {
		const state = originCa({ expiresAt: new Date(Date.now() + 20 * DAY_MS) })
		const domain = makeDomain('api.tudominio.cl', {
			certSource: 'cloudflare_origin_ca',
			originCa: state,
		})

		expect(certificateText(domain)).toBe(
			m.ingress_cert_origin_ca_expiring({ date: formatDate(state.expiresAt as Date) }),
		)
	})
})

describe('originCaHint y originCaFailureText', () => {
	it('emitiendo y las dos variantes de DNS only tienen su pista; lo demás, ninguna', () => {
		expect(originCaHint('issuing')).toBe(m.ingress_origin_ca_issuing())
		expect(originCaHint('dns_only')).toBe(m.ingress_origin_ca_dns_only())
		expect(originCaHint('dns_only_in_cloudflare')).toBe(
			m.ingress_origin_ca_dns_only_cloudflare(),
		)
		expect(originCaHint('failed')).toBeNull()
		expect(originCaHint('renew_failed')).toBeNull()
		expect(originCaHint(null)).toBeNull()
	})

	it('la falla muestra el mensaje del backend tal cual, como oración', () => {
		const domain = makeDomain('api.tudominio.cl', {
			originCa: originCa({
				status: 'failed',
				errorCode: 'permission',
				error: 'muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso',
				expiresAt: null,
			}),
		})

		expect(originCaFailureText(domain)).toBe(
			'Muelle no pudo emitir el certificado de Cloudflare: el token no tiene el permiso',
		)
	})

	it('sin mensaje, el texto del panel: emisión o renovación', () => {
		const failed = makeDomain('api.tudominio.cl', {
			originCa: originCa({ status: 'failed', expiresAt: null, errorCode: 'rejected' }),
		})
		const renewal = makeDomain('api.tudominio.cl', {
			originCa: originCa({ errorCode: 'cloudflare' }),
		})

		expect(originCaFailureText(failed)).toBe(m.ingress_origin_ca_failed())
		expect(originCaFailureText(renewal)).toBe(m.ingress_origin_ca_renew_failed())
	})
})
