import { describe, expect, it } from 'vitest'
import { movedPanelOrigin, panelOrigin, parsePanelOrigin, type ProxySettings } from '../..'

const PROXY: ProxySettings = {
	cloudflare: { tokenSet: false, zones: null, originCa: false },
	publicIpv4: '203.0.113.24',
	publicIpv6: '',
	acmeEmail: 'ops@tudominio.cl',
	panelDomain: 'panel.tudominio.cl',
	httpPort: 80,
	httpsPort: 443,
}

// Dónde sirve muelle-caddy el panel: `https://<panel_domain>[:<proxy_https_port>]`, sin el
// puerto si es 443 (contrato §0.1 y §3.5).
describe('settings', () => {
	it('el origen del panel lleva el puerto HTTPS solo si no es 443', () => {
		expect(panelOrigin(PROXY)).toBe('https://panel.tudominio.cl')
		expect(panelOrigin({ ...PROXY, httpsPort: 8443 })).toBe('https://panel.tudominio.cl:8443')
	})

	it('sin dominio del panel (una IP o localhost, en desarrollo) no hay origen de Caddy', () => {
		expect(panelOrigin({ ...PROXY, panelDomain: '' })).toBeNull()
	})

	it('el panel se muda solo si cambió el puerto HTTPS y hay dominio del panel', () => {
		const https = { ...PROXY, httpsPort: 8443 }
		expect(movedPanelOrigin(PROXY, https)).toBe('https://panel.tudominio.cl:8443')
		expect(movedPanelOrigin(https, PROXY)).toBe('https://panel.tudominio.cl')
		// Solo el HTTP: el panel sigue donde está.
		expect(movedPanelOrigin(PROXY, { ...PROXY, httpPort: 8080 })).toBeNull()
		expect(movedPanelOrigin(PROXY, PROXY)).toBeNull()
		expect(
			movedPanelOrigin({ ...PROXY, panelDomain: '' }, { ...https, panelDomain: '' }),
		).toBeNull()
	})

	it('un origen del panel guardado es https, con un dominio o una IP y el puerto, y nada más', () => {
		for (const origin of [
			'https://panel.tudominio.cl',
			'https://panel.tudominio.cl:8443',
			'https://203.0.113.24:8443',
			'https://[2001:db8::24]:8443',
		]) {
			expect(parsePanelOrigin(origin)).toBe(origin)
		}
	})

	it('un host con lo que una consola interpreta no es un origen del panel (el URL de WHATWG lo acepta)', () => {
		for (const origin of [
			'https://a$(id).com',
			'https://a;id.com',
			'https://a`id`.com',
			"https://a'b.com",
			'https://a&b.com',
			'https://a..b.com',
			'https://-a.com',
		]) {
			expect(new URL(origin).origin).toBe(origin)
			expect(parsePanelOrigin(origin)).toBeNull()
		}
	})

	it('otro esquema, una ruta, un usuario o algo que no es una URL tampoco', () => {
		for (const text of [
			'javascript:alert(1)',
			'http://panel.tudominio.cl',
			'https://panel.tudominio.cl/x',
			'https://felipe@panel.tudominio.cl',
			'https://PANEL.tudominio.cl',
			'panel.tudominio.cl',
			'',
		]) {
			expect(parsePanelOrigin(text)).toBeNull()
		}
	})
})
