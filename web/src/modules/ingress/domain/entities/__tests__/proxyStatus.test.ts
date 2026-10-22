import { describe, expect, it } from 'vitest'
import { makeProxyStatus } from '../../repositories/__fakes__/inMemoryProxyRepository'
import { panelCertificateIssuing, proxyNotice, proxySettling } from '../proxyStatus'

describe('proxyNotice', () => {
	it('sincronizado y corriendo no avisa nada; sin dato tampoco', () => {
		expect(proxyNotice(makeProxyStatus())).toBeNull()
		expect(proxyNotice(null)).toBeNull()
	})

	it('cada estado con su aviso', () => {
		expect(proxyNotice(makeProxyStatus({ status: 'out_of_sync', error: 'x' }))).toBe(
			'out_of_sync',
		)
		expect(proxyNotice(makeProxyStatus({ status: 'recreating' }))).toBe('recreating')
		expect(proxyNotice(makeProxyStatus({ status: 'disabled' }))).toBe('disabled')
	})

	it('sincronizado pero con muelle-caddy parado avisa que no corre', () => {
		const stopped = makeProxyStatus({
			caddy: { image: null, running: false, httpPort: 80, httpsPort: 443 },
		})
		expect(proxyNotice(stopped)).toBe('stopped')
	})

	it('proxySettling: se pregunta de nuevo mientras se arregla solo', () => {
		expect(proxySettling(makeProxyStatus({ status: 'out_of_sync' }))).toBe(true)
		expect(proxySettling(makeProxyStatus({ status: 'recreating' }))).toBe(true)
		expect(proxySettling(makeProxyStatus())).toBe(false)
		expect(proxySettling(makeProxyStatus({ status: 'disabled' }))).toBe(false)
	})
})

describe('panelCertificateIssuing', () => {
	it('solo mientras muelle emite el certificado de Cloudflare del panel', () => {
		const panel = (status: 'issuing' | 'active' | null) =>
			makeProxyStatus({
				panelCertificate: {
					hostname: 'panel.tudominio.cl',
					certSource: status === 'active' ? 'cloudflare_origin_ca' : 'letsencrypt',
					originCa:
						status === null
							? null
							: {
									status,
									errorCode: null,
									error: null,
									retryAt: null,
									expiresAt:
										status === 'active'
											? new Date('2027-01-19T12:00:00Z')
											: null,
								},
				},
			})

		expect(panelCertificateIssuing(panel('issuing'))).toBe(true)
		expect(panelCertificateIssuing(panel('active'))).toBe(false)
		expect(panelCertificateIssuing(panel(null))).toBe(false)
		expect(panelCertificateIssuing(makeProxyStatus({ panelCertificate: null }))).toBe(false)
		expect(panelCertificateIssuing(undefined)).toBe(false)
	})
})
