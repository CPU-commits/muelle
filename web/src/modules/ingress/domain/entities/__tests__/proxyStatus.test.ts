import { describe, expect, it } from 'vitest'
import { makeProxyStatus } from '../../repositories/__fakes__/inMemoryProxyRepository'
import { proxyNotice, proxySettling } from '../proxyStatus'

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
